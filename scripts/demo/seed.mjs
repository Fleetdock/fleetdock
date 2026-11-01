#!/usr/bin/env node
// Seeds a fresh Fleetdock demo install (deploy/demo/docker-compose.yml) with
// realistic state for product screenshots: database servers, backups,
// agent-connected servers, schedules, data-browser edits, schema history,
// saved queries, a fleet query, database users, access control and
// notifications.
//
//   node scripts/demo/seed.mjs
//
// Idempotent: everything is looked up by name (or by existing history) first,
// so re-running does not create duplicates. Node 20+, no dependencies.
//
// Env: FLEETDOCK_DEMO_URL, FLEETDOCK_DEMO_EMAIL, FLEETDOCK_DEMO_PASSWORD.

import { mkdirSync, existsSync, readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const BASE = (process.env.FLEETDOCK_DEMO_URL || 'http://127.0.0.1:18080').replace(/\/+$/, '');
const EMAIL = process.env.FLEETDOCK_DEMO_EMAIL || 'admin@demo.fleetdock.dev';
const PASSWORD = process.env.FLEETDOCK_DEMO_PASSWORD || 'fleetdock-demo-password';

// Demo-only credentials of the sample servers in deploy/demo.
const SERVERS = [
  {
    name: 'prod-postgres', engine: 'postgres', engine_version: '16', host: 'demo-postgres', port: 5432,
    username: 'fleetdock_admin', password: 'demo-postgres-password',
    tags: ['production', 'eu-west'], labels: { team: 'commerce', env: 'production' },
    databases: ['shop', 'analytics', 'billing'],
  },
  {
    name: 'cms-mariadb', engine: 'mariadb', engine_version: '11', host: 'demo-mariadb', port: 3306,
    username: 'fleetdock_admin', password: 'demo-mariadb-password',
    tags: ['production'], labels: { team: 'content', env: 'production' },
    databases: ['cms', 'inventory', 'crm'],
  },
  {
    name: 'support-mysql', engine: 'mysql', engine_version: '8.4', host: 'demo-mysql', port: 3306,
    username: 'fleetdock_admin', password: 'demo-mysql-password',
    tags: ['staging'], labels: { team: 'support', env: 'staging' },
    databases: ['support', 'payments'],
  },
];

// Agent containers in deploy/demo/docker-compose.yml; each reads its
// registration token from deploy/demo/.agent-tokens/<hostname>.
const AGENTS = [
  { hostname: 'db-eu-1', tags: ['production', 'eu-west'], labels: { region: 'eu-west-1', role: 'database-host' } },
  { hostname: 'db-us-1', tags: ['production', 'us-east'], labels: { region: 'us-east-1', role: 'database-host' } },
];
const TOKEN_DIR = fileURLToPath(new URL('../../deploy/demo/.agent-tokens/', import.meta.url));
const MIN_SAMPLES = Number(process.env.FLEETDOCK_DEMO_MIN_SAMPLES || 3);

const DESTINATION = {
  name: 'Demo S3 (SeaweedFS)', provider: 's3_compatible', bucket: 'fleetdock-backups', region: 'us-east-1',
  endpoint: 'http://s3:8333', prefix: 'demo', access_key_id: 'fleetdock-demo', secret_access_key: 'demo-s3-secret-key',
};

// ---------------------------------------------------------------- helpers

const t0 = Date.now();
const summary = { done: [], skipped: [], failed: [], warnings: [] };
const warn = (msg) => { console.log(`  ~ WARNING: ${msg}`); summary.warnings.push(msg); };
let token = '';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const log = (msg) => console.log(`  ${msg}`);
const elapsed = () => `${((Date.now() - t0) / 1000).toFixed(1)}s`;

class ApiError extends Error {
  constructor(method, path, status, body) {
    const msg = body?.error?.message || body?.error || (typeof body === 'string' ? body : JSON.stringify(body));
    super(`${method} ${path} -> ${status}: ${typeof msg === 'string' ? msg : JSON.stringify(msg)}`);
    this.status = status;
    this.body = body;
  }
}

async function api(method, path, body) {
  const headers = { Accept: 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  const res = await fetch(BASE + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  const text = await res.text();
  let data = text;
  try { data = text ? JSON.parse(text) : null; } catch { /* keep text */ }
  if (!res.ok) throw new ApiError(method, path, res.status, data);
  return data;
}
const get = (p) => api('GET', p);
const post = (p, b = {}) => api('POST', p, b);

// Lists every item of a (possibly paginated) collection.
async function listAll(path) {
  const sep = path.includes('?') ? '&' : '?';
  const out = [];
  for (let offset = 0; ; ) {
    const page = await get(`${path}${sep}limit=100&offset=${offset}`);
    const items = page?.items ?? [];
    out.push(...items);
    if (!page?.pagination || items.length === 0 || out.length >= page.pagination.total) break;
    offset = out.length;
  }
  return out;
}

async function waitOperation(id, label, timeoutMs = 180_000) {
  const end = Date.now() + timeoutMs;
  for (;;) {
    const op = await get(`/v1/operations/${id}`);
    if (['succeeded', 'failed', 'canceled'].includes(op.status)) {
      if (op.status !== 'succeeded') throw new Error(`${label}: operation ${id} ${op.status}${op.error ? `: ${op.error}` : ''}`);
      return op;
    }
    if (Date.now() > end) throw new Error(`${label}: operation ${id} still ${op.status} after ${timeoutMs / 1000}s`);
    await sleep(1000);
  }
}

async function step(title, fn) {
  console.log(`\n[${elapsed()}] ${title}`);
  try {
    const r = await fn();
    if (r === 'skip') summary.skipped.push(title);
    else summary.done.push(title);
  } catch (e) {
    console.log(`  ! FAILED: ${e.message}`);
    summary.failed.push(`${title}: ${e.message}`);
  }
}

// State shared between steps.
const S = { instances: {}, dbs: {}, destinationId: null, backups: {}, userId: null };
const db = (name) => {
  const d = S.dbs[name];
  if (!d) throw new Error(`database "${name}" is not registered`);
  return d;
};
const sql = (dbName, text, extra = {}) => post(`/v1/databases/${db(dbName).id}/query`, { sql: text, ...extra });

// ------------------------------------------------------------------ steps

await step('1. Log in', async () => {
  const r = await post('/v1/auth/login', { email: EMAIL, password: PASSWORD });
  token = r.token;
  if (!token) throw new Error('login returned no token');
  log(`logged in as ${EMAIL} at ${BASE}`);
});
if (!token) {
  console.error('\nCannot continue without a session.');
  process.exit(1);
}

await step('2a. Agent servers: registration tokens for the demo agents', async () => {
  mkdirSync(TOKEN_DIR, { recursive: true });
  const servers = await listAll('/v1/servers');
  const tokens = (await get('/v1/agent-tokens')).items ?? [];
  for (const a of AGENTS) {
    const file = TOKEN_DIR + a.hostname;
    const srv = servers.find((x) => x.hostname === a.hostname || x.name === a.hostname);
    if (srv) { log(`server ${a.hostname} already enrolled (${srv.id}, ${srv.status})`); continue; }
    // A pending token is reusable only if its raw value is still on disk.
    const name = `demo agent ${a.hostname}`;
    const open = tokens.find((t) => t.name === name && !t.used_at && new Date(t.expires_at) > new Date());
    if (open && existsSync(file) && readFileSync(file, 'utf8').trim()) {
      log(`registration token for ${a.hostname} pending (${open.id}); agent should enroll shortly`);
      continue;
    }
    const t = await post('/v1/agent-tokens', { name, ttl_hours: 24 });
    writeFileSync(file, t.token + '\n', { mode: 0o600 });
    log(`registration token for ${a.hostname} -> ${t.id}, written to deploy/demo/.agent-tokens/${a.hostname}`);
  }
});

await step('2. Connect database servers and import their databases', async () => {
  const existing = await listAll('/v1/instances');
  for (const s of SERVERS) {
    let inst = existing.find((i) => i.name === s.name);
    if (inst) {
      log(`server ${s.name} exists (${inst.id})`);
    } else {
      inst = await post('/v1/instances', {
        kind: 'external', name: s.name, engine: s.engine, engine_version: s.engine_version,
        host: s.host, port: s.port, username: s.username, password: s.password,
        tls_mode: 'disable', tags: s.tags, labels: s.labels,
      });
      log(`registered server ${s.name} (${s.engine}) -> ${inst.id}`);
    }
    S.instances[s.name] = inst;
  }

  const haveDbs = async () => {
    const all = await listAll('/v1/databases');
    for (const d of all) {
      const owner = SERVERS.find((s) => S.instances[s.name]?.id === (d.instance_id || d.instance?.id));
      if (owner && owner.databases.includes(d.name)) S.dbs[d.name] = d;
    }
    return SERVERS.filter((s) => s.databases.some((n) => !S.dbs[n]));
  };

  let missing = await haveDbs();
  for (const s of missing) {
    const inst = S.instances[s.name];
    const r = await post(`/v1/instances/${inst.id}/import-databases`);
    log(`import-databases on ${s.name}: mode=${r.mode}${r.imported != null ? ` imported=${JSON.stringify(r.imported)}` : ''}`);
    if (r.operation_id) await waitOperation(r.operation_id, `import ${s.name}`);
  }
  if (missing.length) {
    const end = Date.now() + 60_000;
    while ((missing = await haveDbs()).length && Date.now() < end) await sleep(1000);
  }
  const want = SERVERS.flatMap((s) => s.databases);
  const absent = want.filter((n) => !S.dbs[n]);
  if (absent.length) throw new Error(`databases not discovered: ${absent.join(', ')}`);
  log(`all ${want.length} databases present: ${want.map((n) => `${n}=${S.dbs[n].id}`).join(' ')}`);
});

await step('3. Add S3 backup destination and test it', async () => {
  const list = (await get('/v1/backup-destinations')).items ?? [];
  let d = list.find((x) => x.name === DESTINATION.name);
  if (d) log(`destination exists (${d.id})`);
  else {
    d = await post('/v1/backup-destinations', DESTINATION);
    log(`created destination ${DESTINATION.name} -> ${d.id}`);
  }
  S.destinationId = d.id;
  const t = await post(`/v1/backup-destinations/${d.id}/test`);
  log(`test: ${JSON.stringify(t)}`);
});

await step('4. Backup schedules, immediate backups, verify', async () => {
  if (!S.destinationId) throw new Error('no destination');
  const schedules = (await get('/v1/backup-schedules')).items ?? [];
  for (const [name, cron, retention] of [['shop', '0 2 * * *', 14], ['crm', '0 */6 * * *', 7], ['billing', '30 3 * * *', 30]]) {
    const id = db(name).id;
    const s = schedules.find((x) => x.database_id === id);
    if (s) { log(`schedule for ${name} exists (${s.cron})`); continue; }
    const c = await post('/v1/backup-schedules', { database_id: id, destination_id: S.destinationId, cron, retention_days: retention, enabled: true });
    log(`schedule ${name} "${cron}" keep ${retention}d -> ${c.id}`);
  }

  // One backup per database: an existing completed (or in-flight) backup is
  // kept; a failed one is not retried, so re-runs never pile up attempts.
  const backups = await listAll('/v1/backups');
  const pending = [];
  // support/payments (MySQL 8.4) are left out: the control plane's
  // mariadb-dump cannot load the caching_sha2_password auth plugin.
  for (const name of ['shop', 'billing', 'cms', 'crm']) {
    const id = db(name).id;
    const mine = backups.filter((b) => b.database_id === id);
    const ok = mine.find((b) => b.status === 'completed');
    if (ok) { S.backups[name] = ok; log(`backup of ${name} exists (${ok.id})`); continue; }
    const other = mine.find((b) => b.status !== 'completed');
    if (other) {
      if (other.status === 'failed') warn(`backup of ${name} failed earlier, not retrying: ${other.error ?? 'unknown error'}`);
      else log(`backup of ${name} is ${other.status} (${other.id})`);
      continue;
    }
    const b = await post('/v1/backups', { database_id: id, destination_id: S.destinationId });
    log(`triggered backup of ${name} -> ${b.id}`);
    pending.push([name, b]);
  }
  await Promise.all(pending.map(async ([name, b]) => {
    try {
      if (b.operation_id) await waitOperation(b.operation_id, `backup ${name}`);
    } catch (e) {
      warn(e.message);
    }
    S.backups[name] = await get(`/v1/backups/${b.id}`);
    log(`backup of ${name} ${S.backups[name].status}${S.backups[name].size_bytes ? ` (${S.backups[name].size_bytes} bytes)` : ''}`);
  }));

  const shop = S.backups.shop;
  if (shop?.status === 'completed' && !shop.verify_status) {
    const v = await post(`/v1/backups/${shop.id}/verify`);
    await waitOperation(v.operation_id, 'verify shop backup', 300_000);
    const after = await get(`/v1/backups/${shop.id}`);
    log(`verified shop backup: ${after.verify_status}${after.verify_error ? ` (${after.verify_error})` : ''}`);
  } else if (shop) log(`shop backup already verified: ${shop.verify_status}`);
});

await step('5. Data edits through the table editor (row change journal)', async () => {
  const edits = [
    ['shop', 'public.products', [
      [{ id: '3' }, { price_cents: '2499', stock: '18' }],
      [{ id: '7' }, { stock: '0', active: 'false' }],
      [{ id: '12' }, { price_cents: '15900' }],
    ]],
    ['crm', 'deals', [
      [{ id: '5' }, { stage: 'won' }],
      [{ id: '11' }, { stage: 'proposal', value_eur: '48500.00' }],
    ]],
    ['support', 'tickets', [[{ id: '2' }, { status: 'closed' }]]],
  ];
  let any = false;
  for (const [name, table, rows] of edits) {
    const d = db(name);
    const ch = await get(`/v1/databases/${d.id}/changes?limit=5`);
    const n = (ch.items ?? []).length;
    if (n) { log(`${name}: change journal already has entries, skipping`); continue; }
    const batch = crypto.randomUUID();
    for (const [key, values] of rows) {
      try {
        const r = await api('PATCH', `/v1/databases/${d.id}/tables/${encodeURIComponent(table)}/rows`, { key, values, batch_id: batch });
        log(`${name}.${table} ${JSON.stringify(key)} <- ${JSON.stringify(values)}: ${r.status}${r.change_id ? '' : ' (no journal entry)'}`);
        any = true;
      } catch (e) {
        log(`${name}.${table} ${JSON.stringify(key)}: ${e.message}`);
      }
    }
  }
  return any ? undefined : 'skip';
});

await step('6. Schema history: watch shop, change schema, snapshot', async () => {
  const d = db('shop');
  await api('PUT', `/v1/databases/${d.id}/schema/watch`, { enabled: true });
  log('schema watch enabled on shop');
  const snaps = async () => (await get(`/v1/databases/${d.id}/schema/snapshots`)).items ?? [];
  let list = await snaps();
  if (!list.length) {
    // Baseline before the change so the timeline has a diff.
    const r = await post(`/v1/databases/${d.id}/schema/snapshots`);
    log(`baseline snapshot: created=${r.created}`);
  }
  const cols = await sql('shop', "SELECT 1 FROM information_schema.columns WHERE table_name = 'customers' AND column_name = 'loyalty_tier'");
  const has = (cols.results?.[0]?.rows ?? []).length > 0;
  if (has) log('customers.loyalty_tier already exists');
  else {
    await sql('shop', [
      "ALTER TABLE customers ADD COLUMN loyalty_tier text NOT NULL DEFAULT 'standard'",
      'CREATE INDEX IF NOT EXISTS customers_loyalty_tier_idx ON customers (loyalty_tier)',
      'CREATE INDEX IF NOT EXISTS products_category_idx ON products (category)',
    ].join(';\n'));
    log('added customers.loyalty_tier + 2 indexes');
  }
  const r = await post(`/v1/databases/${d.id}/schema/snapshots`);
  log(r.created ? `new snapshot: changes=${JSON.stringify(r.snapshot?.summary?.changes ?? {})}` : "schema unchanged since the latest snapshot");
  list = await snaps();
  log(`shop has ${list.length} schema snapshot(s)`);
});

await step('7. Saved queries and query history', async () => {
  const saved = (await get('/v1/saved-queries')).items ?? [];
  const wanted = [
    ['Top customers by revenue', 'shop',
      'SELECT c.full_name, c.country, sum(o.total_cents) / 100.0 AS revenue\nFROM orders o JOIN customers c ON c.id = o.customer_id\nGROUP BY c.id\nORDER BY revenue DESC\nLIMIT 20;'],
    ['Low stock products', 'shop',
      'SELECT sku, name, category, stock\nFROM products\nWHERE active AND stock < 10\nORDER BY stock;'],
    ['Pipeline by stage', 'crm',
      'SELECT stage, count(*) AS deals, sum(value_eur) AS value_eur\nFROM deals\nGROUP BY stage\nORDER BY value_eur DESC;'],
  ];
  for (const [name, dbName, text] of wanted) {
    const s = saved.find((x) => x.name === name);
    if (s) { log(`saved query "${name}" exists`); continue; }
    const c = await post('/v1/saved-queries', { name, sql: text, database_id: db(dbName).id });
    log(`saved query "${name}" -> ${c.id}`);
  }
  const runs = {
    shop: [wanted[0][2], wanted[1][2], "SELECT status, count(*) FROM orders GROUP BY status ORDER BY 2 DESC;"],
    crm: [wanted[2][2], 'SELECT industry, count(*) FROM companies GROUP BY industry;'],
    analytics: ['SELECT * FROM daily_active_users ORDER BY 1 DESC LIMIT 14;'],
  };
  for (const [dbName, queries] of Object.entries(runs)) {
    const h = await get(`/v1/databases/${db(dbName).id}/query-history?limit=200`);
    const seen = new Set((h.items ?? []).map((x) => x.sql.trim().replace(/;$/, '')));
    for (const q of queries) {
      if (seen.has(q.trim().replace(/;$/, ''))) { log(`${dbName}: already in history: "${q.split('\n')[0].slice(0, 50)}"`); continue; }
      const r = await sql(dbName, q, { limit: 200 });
      const res = r.results?.[0];
      log(`${dbName}: ran "${q.split('\n')[0].slice(0, 50)}" -> ${res?.rows?.length ?? res?.rows_affected ?? '?'} rows`);
    }
  }
});

await step('8. Fleet query across MariaDB + MySQL databases', async () => {
  const text = "SELECT count(*) AS tables FROM information_schema.tables WHERE table_schema = DATABASE();";
  const runs = (await get('/v1/fleet-runs')).items ?? [];
  let run = runs.find((r) => r.sql === text && r.status === 'succeeded');
  if (run) { log(`fleet run exists (${run.id})`); S.fleetRunId = run.id; return; }
  const ids = ['cms', 'inventory', 'crm', 'support', 'payments'].map((n) => db(n).id);
  run = await post('/v1/fleet-runs', { sql: text, database_ids: ids, concurrency: 3, stop_on_error: false });
  log(`fleet run ${run.id} on ${ids.length} databases`);
  const end = Date.now() + 120_000;
  while (['running', 'canceling'].includes(run.status) && Date.now() < end) {
    await sleep(1000);
    run = await get(`/v1/fleet-runs/${run.id}`);
  }
  S.fleetRunId = run.id;
  log(`fleet run ${run.status}: ${run.succeeded}/${run.total} succeeded, ${run.failed} failed`);
  if (run.status !== 'succeeded') throw new Error(`fleet run ended ${run.status}`);
});

await step('9. Database users and grants on prod-postgres', async () => {
  const inst = S.instances['prod-postgres'];
  const users = (await get(`/v1/instances/${inst.id}/db-users`)).items ?? [];
  const exists = users.some((u) => (u.username ?? u.name ?? u.user) === 'reporting');
  if (exists) log('db user reporting exists');
  else {
    await post(`/v1/instances/${inst.id}/db-users`, { username: 'reporting', host: '', password: 'demo-reporting-password' });
    log('created db user reporting');
  }
  await post(`/v1/databases/${db('shop').id}/grants`, { username: 'reporting', host: '', privileges: ['SELECT'] });
  await post(`/v1/databases/${db('analytics').id}/grants`, { username: 'reporting', host: '', privileges: ['SELECT'] });
  log('granted SELECT on shop and analytics to reporting');
});

await step('10. Access: custom role, second user, API token', async () => {
  const roles = (await get('/v1/roles')).items ?? [];
  const perms = ['database:read', 'backup:read', 'instance:read', 'operation:read', 'schedule:read', 'server:read'];
  // Role names are stored lower-cased.
  let role = roles.find((r) => r.name.toLowerCase() === 'analyst');
  if (role) log(`role Analyst exists (${role.id})`);
  else {
    role = await post('/v1/roles', { name: 'Analyst', description: 'Read-only access to data and backups', permissions: perms });
    log(`created role Analyst -> ${role.id}`);
  }
  const users = (await get('/v1/users')).items ?? [];
  const people = [
    ['Maria Lopez', 'maria@demo.fleetdock.dev', role.name],
    ['Daniel Okafor', 'daniel@demo.fleetdock.dev', 'operator'],
  ];
  for (const [name, email, r] of people) {
    const u = users.find((x) => x.email === email);
    if (u) { log(`user ${email} exists (${u.id}, roles ${u.roles?.join(',')})`); continue; }
    const c = await post('/v1/users', { name, email, password: 'demo-user-password-123', role: r });
    log(`created user ${email} (${r}) -> ${c.id}`);
  }
  const tokens = (await get('/v1/tokens')).items ?? [];
  const tk = tokens.find((x) => x.name === 'CI backups');
  if (tk) log(`API token "CI backups" exists (${tk.prefix})`);
  else {
    const c = await post('/v1/tokens', { name: 'CI backups', scopes: ['backup:read', 'backup:write', 'database:read'], ttl_hours: 24 * 90 });
    log(`created API token "CI backups" (${c.prefix ?? c.id})`);
  }
});

await step('11. Notifications: webhook channel and alert rule', async () => {
  const chans = (await get('/v1/notification-channels')).items ?? [];
  let ch = chans.find((c) => c.name === 'Ops webhook');
  if (ch) log(`channel exists (${ch.id})`);
  else {
    ch = await post('/v1/notification-channels', { name: 'Ops webhook', type: 'webhook', config: { url: 'https://hooks.example.com/fleetdock' }, enabled: true });
    log(`created webhook channel -> ${ch.id}`);
  }
  const rules = (await get('/v1/alert-rules')).items ?? [];
  const want = [
    { name: 'Disk almost full', metric: 'disk_used_pct', comparator: 'gt', threshold: 85, for_seconds: 300, severity: 'critical' },
    { name: 'High CPU', metric: 'cpu_pct', comparator: 'gt', threshold: 90, for_seconds: 600, severity: 'warning' },
  ];
  for (const w of want) {
    if (rules.some((r) => r.name === w.name)) { log(`alert rule "${w.name}" exists`); continue; }
    const c = await post('/v1/alert-rules', { ...w, target_type: 'global', channel_ids: [ch.id], enabled: true });
    log(`created alert rule "${w.name}" -> ${c.id}`);
  }
  log('note: alert rules only support server metrics (cpu/mem/disk/connections); no backup-failure rule type exists');
});

await step('12. Agent servers online with metrics', async () => {
  const end = Date.now() + Number(process.env.FLEETDOCK_DEMO_AGENT_TIMEOUT || 180) * 1000;
  const pending = new Set(AGENTS.map((a) => a.hostname));
  let lastNote = 0;
  for (;;) {
    const servers = await listAll('/v1/servers');
    for (const a of AGENTS) {
      if (!pending.has(a.hostname)) continue;
      const srv = servers.find((x) => x.hostname === a.hostname || x.name === a.hostname);
      if (!srv || srv.status !== 'online') continue;
      const samples = (await get(`/v1/servers/${srv.id}/metrics?hours=168`)).items ?? [];
      if (samples.length < MIN_SAMPLES) continue;
      pending.delete(a.hostname);
      S.servers = { ...(S.servers ?? {}), [a.hostname]: srv.id };
      const sameTags = JSON.stringify([...(srv.tags ?? [])].sort()) === JSON.stringify([...a.tags].sort());
      const sameLabels = Object.entries(a.labels).every(([k, v]) => srv.labels?.[k] === v);
      if (!sameTags || !sameLabels) {
        await api('PATCH', `/v1/servers/${srv.id}`, { tags: a.tags, labels: { ...(srv.labels ?? {}), ...a.labels } });
        log(`${a.hostname}: set tags/labels`);
      }
      log(`${a.hostname} online (${srv.id}), ${samples.length} metric samples`);
    }
    if (!pending.size) break;
    if (Date.now() > end) throw new Error(`not online with ${MIN_SAMPLES}+ samples: ${[...pending].join(', ')} (agents heartbeat every 30s)`);
    if (Date.now() - lastNote > 30_000) { log(`waiting for ${[...pending].join(', ')} (heartbeats every 30s)...`); lastNote = Date.now(); }
    await sleep(3000);
  }
});

// ---------------------------------------------------------------- summary

console.log(`\nDone in ${elapsed()}.`);
console.log(`  ok: ${summary.done.length}  skipped (nothing to do): ${summary.skipped.length}  failed: ${summary.failed.length}  warnings: ${summary.warnings.length}`);
for (const f of summary.failed) console.log(`  FAILED ${f}`);
for (const w of summary.warnings) console.log(`  WARNING ${w}`);
if (S.dbs.shop) console.log(`  shop db:  ${S.dbs.shop.id}`);
if (S.dbs.crm) console.log(`  crm db:   ${S.dbs.crm.id}`);
for (const [n, i] of Object.entries(S.instances)) console.log(`  server ${n}: ${i.id}`);
for (const [n, id] of Object.entries(S.servers ?? {})) console.log(`  agent server ${n}: ${id}`);
if (S.fleetRunId) console.log(`  fleet run: ${S.fleetRunId}`);
process.exit(summary.failed.length ? 1 : 0);
