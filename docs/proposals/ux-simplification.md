# Proposal — Simplifying database management UX

Status: **draft, for review**
Scope: frontend information architecture, instance sync/health mechanics, and a
prioritized list of refactors found while reading the code.

---

## 1. Summary

Three problems, one root cause.

| Symptom | Root cause |
|---|---|
| Two nav items (Instances, Databases) for one mental model | The UI mirrors the *data model* (`instances` table, `databases` table) instead of the *user's job* ("manage my databases") |
| "Import DBs" is a manual button | The control plane treats engine state as something the user pushes into it, rather than something it observes |
| "Test connection" is a manual button | Same — liveness is polled by a human instead of by the system |

The fix for #2 and #3 is the same mechanism: **a periodic probe per instance
that returns both liveness and inventory in one round-trip.** Listing databases
already proves the connection works. Once that exists, both buttons are deleted,
along with the `test_connection` and `import_databases` job types.

---

## 2. Information architecture

### 2.1 Merge Instances into Databases

**Nav:** one item, `Databases`. Delete the `Instances` sidebar entry.

**Routes:**

| Route | Purpose | Change |
|---|---|---|
| `/databases` | Fleet view, grouped by instance | rewritten |
| `/instances/[id]` | Instance detail (databases, DB users, settings) | kept, reachable only from `/databases` |
| `/databases/[id]` | Database detail | unchanged |
| `/instances` | — | **permanent redirect → `/databases`** |

Keeping `/instances/[id]` as a route avoids a Next.js dynamic-segment collision
(`/databases/[id]` vs a would-be `/databases/instance/[id]`) and avoids churn in
every `Link` and breadcrumb. The URL is not user-facing vocabulary; the nav label
and breadcrumb are. Breadcrumb becomes `Databases / <instance name>`.

**`/databases` layout — grouped by default, flat on demand:**

```
Databases                                    [+ Create database] [Add instance]
[search…]                            view: (•) By instance  ( ) All databases

▾  primary          mariadb 11.4   web-01:3306   ● healthy · 12ms · 20s ago   4 dbs   1.2 GB
     app_production      utf8mb4   active     420 MB    [Backup] [⋯]
     app_staging         utf8mb4   active      88 MB    [Backup] [⋯]
     mysql        system utf8mb4   active      12 MB    [Backup] [⋯]
▸  analytics        postgres 16    ext db.acme.com:5432  ● unreachable · 4m ago  — 
▸  legacy           mysql 8.4      web-02:3306   ○ no credentials             2 dbs
```

- The instance row **is** the old instances-table row: engine, location, health,
  db count, total size. Clicking the name opens `/instances/[id]`.
- The `⋯` menu on an instance holds the rare actions: Start/Stop/Restart
  (provisioned only), Edit credentials, Remove.
- `All databases` view is today's flat `/databases` table with an `Instance`
  column — keep it, it is the right tool for "where is `orders_db`?".
- Search filters both levels; an instance stays visible if it or any of its
  databases matches.
- Empty state when there are zero instances points at "Add instance", not
  "Create database" — today's copy ("Create your first database") is a dead end.

**Why grouping beats two pages:** the instance is a *container*, not a peer
concept. A flat list of instances with no databases visible answers no question
a user actually has. Neon, PlanetScale and Supabase all present exactly this
shape — the server/branch is a group header, never its own destination.

### 2.2 Consolidate the rest of the sidebar

12 items → 7. Sections use **real nested routes**, not client-side tab state, so
they deep-link and can carry per-tab RBAC.

| New item | Route | Absorbs |
|---|---|---|
| Overview | `/dashboard` | — |
| Servers | `/servers` | — |
| Databases | `/databases` | Instances |
| Backups | `/backups` · `/backups/schedules` · `/backups/destinations` | Schedules, Destinations |
| Activity | `/activity` | Operations |
| Access | `/access/users` · `/access/roles` · `/access/tokens` | Users, Roles, API Tokens |
| Settings | `/settings/notifications` · `/settings/profile` | Notifications |

Notes:

- **Notifications** today is channel + alert-rule *configuration*, not an inbox.
  It belongs in Settings. If a real notification inbox is ever built, it goes in
  the topbar as a bell, not the sidebar.
- **Operations → Activity**: "Operations" reads as a noun the product owns;
  "Activity" tells a user what they'll find. Consider surfacing running
  operations as a topbar indicator too — `useOperationToasts` already exists.
- **RBAC:** section is visible if the user has *any* child permission. The
  section index route (`/access`) redirects to the first tab the user can read.
  Today `sidebar.tsx` filters per item with `can`/`canAny`; that logic moves into
  a `NAV` tree with a `children` array and an `anyOf` permission list.

---

## 3. Replacing "Import DBs" with continuous sync

### 3.1 Design

Add one recurring operation, **instance probe**, that does:

1. `Ping` → engine version, latency
2. `ListDatabases` → name, charset, collation, system flag, size, connections

Both already exist in `internal/platform/engine` (`eng.Ping`, `eng.ListDatabases`)
and are exactly what `TestConnection` and `ImportDatabases` call today
(`internal/app/instance/service.go:283` and `:329`). No new engine code.

**Who runs it:**

| Instance kind | Runner | Transport |
|---|---|---|
| External | Control-plane worker | Direct connection, new ticker in `worker.Run` |
| Managed | Agent | Piggyback on the existing heartbeat loop (`cmd/agent/main.go:166`) |

For managed instances the agent already wakes on a timer and already posts to
`/agent/v1/heartbeat`. Extend the heartbeat payload with an `instances[]` array,
or — cleaner, because the payloads have different cadences and sizes — add
`POST /agent/v1/instances/report`. The agent learns which instances to probe from
the heartbeat *response* (control plane returns the instance list + credentials
refs for that server). This keeps the agent stateless and means adding an
instance takes effect within one heartbeat with no job dispatch.

**Cadence:** 60s baseline, exponential backoff to 10m after consecutive
failures, reset on success. Plus an immediate probe when a user opens
`/databases` or `/instances/[id]` (stale-while-revalidate: serve cached, kick a
refresh, let TanStack Query pick up the new data).

### 3.2 Reconciliation semantics — never auto-delete

This is the part that must be conservative. Diff engine inventory against the
`databases` table:

| Case | Action |
|---|---|
| In engine, not in metadata | Insert with `origin = 'discovered'` |
| In both | Update `charset`, `collation`, `size_bytes`, `active_connections`, `system` |
| In metadata, not in engine | Increment `missing_probes`. At 1 → status `missing` (shown with a warning badge). Never hard-delete. |
| Was `missing`, reappears | Reset counter, status `active` |

A database dropped outside Fleetdock should surface as *"`orders_db` no longer
exists on this instance — [Remove from Fleetdock]"*, because the alternative
(silently deleting the row) also destroys its backup history, schedules and
credential records. This is the single most important rule in the design.

**Schema additions** (one migration):

```sql
ALTER TABLE databases
  ADD COLUMN origin           text NOT NULL DEFAULT 'created',   -- created | discovered
  ADD COLUMN missing_probes   int  NOT NULL DEFAULT 0,
  ADD COLUMN last_seen_at     timestamptz;

ALTER TABLE instances
  ADD COLUMN health           text,        -- healthy | unreachable | unauthorized | unknown
  ADD COLUMN health_error     text,
  ADD COLUMN latency_ms       int,
  ADD COLUMN detected_version text,
  ADD COLUMN last_probed_at   timestamptz,
  ADD COLUMN probe_failures   int NOT NULL DEFAULT 0;
```

### 3.3 What the user sees

- Instance row: `● healthy · 12ms · synced 20s ago`, with a small `↻` that
  forces an immediate probe. A manual refresh affordance is good practice — it
  is *not* the same as making sync manual. It should be an icon, not a labelled
  button, and it must never be required.
- `Import DBs` button: **deleted.** `useImportDatabases` deleted.
- On adding an instance with credentials, the first probe fires immediately, so
  discovered databases appear within seconds of creation — the import step
  simply never surfaces.
- New instances with no credentials show `○ no credentials` and a
  `Add credentials` action, which is the real blocker today (`has_credentials`
  gates half the UI but there is no way to add them after registration — see
  §5, item 12).

---

## 4. Replacing "Test connection" with passive health

`Test connection` is the probe's liveness half. Once probes run continuously, a
button that asks "is it connected?" is answering a question the UI should
already be displaying.

- **Instance status** becomes a live dot with a tooltip:
  `Connected · MariaDB 11.4 · 12 ms · checked 20s ago`. On failure:
  `Unreachable · connection refused · failing for 4m` with the last error text
  and a `Retry now` link (which is the same forced probe as `↻`).
- **Delete** `jobdom.TypeTestConnection` and `jobdom.TypeImportDatabases`, their
  `executor.Execute` cases (`internal/platform/executor/executor.go:85,110`),
  the two service methods, both HTTP routes, and the two React hooks.
- **Do not** write a job row per probe. Operations is a log of user-intended
  mutating work; flooding it with 1440 probe rows per instance per day would
  destroy its usefulness. Probes are telemetry — they update instance columns
  and nothing else. (Today every "Test" click creates an operation row, which is
  already noise.)
- **Alerting for free:** emit `instance.unreachable` through the existing
  `notificationapp.Emit`, mirroring how `server.offline` works in
  `worker.housekeeping` (`internal/worker/worker.go:96`). Users get paged for a
  dead database instead of discovering it by clicking a button.

**Net code change:** two job types, two executor branches, two service methods,
two routes and two hooks deleted; one probe loop and one reconciler added. The
codebase gets smaller.

---

## 5. Other improvements found

Ordered by (impact ÷ effort). Items 1–3 are correctness bugs, not polish.

### Correctness

1. **Instance names break past 100 instances.** — **done**
   `databases/page.tsx` called `useInstances()` with no page argument, which
   requests `limit=100`; the backend caps it there too (`maxLimit = 100`).
   Databases on the 101st+ instance rendered `d.instance_id.slice(0, 8)` — a raw
   UUID fragment — and `DeleteDatabaseModal` received `instance={undefined}`, so
   its physical-drop option was silently disabled even when credentials existed.

   *(Correction to an earlier draft of this document, which said 20. The
   pagination default is 20, but these call sites pass `limit=100` explicitly.
   The bug is real; the threshold was wrong.)*

   **Fixed by** embedding the instance in the database response
   (`{id, name, engine, kind, server_id, provisioned, has_credentials}` via a
   SQL join in `database_repository.go`) and deleting the client-side `Map`
   lookups. This also removed one HTTP request from the databases list *and* the
   database detail page, and closes the window where detail rendered before the
   instance had loaded.

2. **Instance detail silently truncated its database list.** — **done**
   `instances/[id]/page.tsx` rendered `useDatabases({instance_id: id})` in a
   `DataTable` with no `pagination` prop, capping at `limit=100` with no
   indication there was more. Now paginated.

3. **The volume-deletion double-confirm is dangerous.**
   `instances/page.tsx:179` and `instances/[id]/page.tsx:120` both do:
   `confirm("Also delete the data volume? … Cancel to keep the volume.")`.
   Cancel meaning "yes, but the other thing" is an anti-pattern — a user
   dismissing a dialog they didn't read gets a different destructive outcome
   than they expect. Replace with one modal containing an explicit unchecked
   `[ ] Also delete the data volume (permanently destroys data)` and a
   type-the-name confirmation, matching the existing `DeleteDatabaseModal`.

### Consistency

4. **16 `window.confirm()` calls across 10 pages.** Build one
   `<ConfirmDialog>` (or `useConfirm()`) in `components/ui.tsx`. Native
   `confirm` is unstyleable, blocks the event loop, can't show the resource
   name in bold, and is suppressible by the browser.

5. **30 ad-hoc `setNotice` banner states** while a full `ToastProvider`
   (`components/toast.tsx`) sits unused outside `useOperationToasts`. Route all
   mutation feedback through `useToast`; delete ~30 `useState<string|null>`
   declarations and their JSX.

6. **Duplicated instance-delete logic** in `instances/page.tsx` and
   `instances/[id]/page.tsx`. Collapses to one component in the §2 rewrite.

### Structure

7. **`lib/hooks.ts` is 1029 lines / ~95 hooks.** Split into
   `lib/hooks/{instances,databases,backups,access,admin,notifications}.ts` with a
   barrel re-export so imports don't churn. Add a `queryKeys` factory — keys are
   stringly-typed today, so an invalidation typo fails silently.

8. **`databases/[id]/page.tsx` is 1109 lines** holding 8+ components. Extract
   `MoveDatabaseModal`, `QueryConsole`, `TablesBrowser`, `TableViewer`,
   `GrantsSection` into `components/database/`.

9. **Inline styles everywhere** — 42 `style={{}}` in one file, ~200 across the
   app. The recurring shapes are: card padding, `repeat(auto-fit, minmax(Npx,1fr))`
   grids, and right-aligned action rows. Three CSS classes or two small
   primitives (`<Toolbar>`, `<DetailGrid>`) remove most of them.

10. **Engine metadata is duplicated and will drift.**
    `ENGINE_DEFAULTS` in `instances/page.tsx:269` restates
    `Engine.DefaultPort()` / `Engine.AdminUser()` from
    `internal/domain/instance/instance.go`. Expose `GET /v1/engines` returning
    `{engine, versions[], default_port, admin_user}` — one source of truth, and
    adding an engine stops requiring a frontend release.

11. **MariaDB naming leaks into engine-agnostic code.**
    `HeartbeatInfo.MariaDBVersion` (`domain/server/agent.go:14`) and
    `secretdom.KindMariaDBRoot` — the latter stores PostgreSQL passwords today.
    Rename to `EngineVersion` and `KindAdminPassword`.

12. **Credentials can be set at registration but never edited.** — **done**
    `has_credentials` gates DB users, grants, browsing, backups and the whole
    probe design — yet there was no `PATCH /v1/instances/{id}` to add or rotate
    them. A user who registered an instance without a password had to delete and
    re-create it. `PATCH` now supports renaming, host/port edits (host is
    external-only, port is refused on provisioned instances), and adding,
    rotating, removing or clearing credentials, with the secret-store side
    effects covered by unit tests.

13. **`kind` + `provisioned` express one thing.** The UI shows two badges
    (`managed` and `docker`) for what is really a single `source` enum:
    `provisioned` (we created the container) / `adopted` (managed server, DB we
    didn't create) / `external` (network-reachable). One field, one badge.

### Testing

14. **Zero frontend tests**, 32 backend test files (~9% coverage per ROADMAP).
    The §2 rewrite is the right moment to add a Playwright smoke path:
    login → `/databases` → open instance → open database → run a `SELECT 1`.

15. **The probe/reconciler needs tests before it ships.** Specifically the
    "database disappeared from the engine" path — that's the one that can lose
    user data references if it's wrong.

### Backend

16. **Worker tickers are hardcoded** (3s / 15s / 60s in `worker.Run`), and the
    probe loop makes a fourth. Consider a small registry of periodic tasks with
    configurable intervals, so operators can tune probe cadence for large fleets
    without a rebuild.

---

## 6. Suggested sequencing

Each step is independently shippable.

| # | Step | Why this order |
|---|---|---|
| 1 | ~~`PATCH /v1/instances/{id}` for credentials (§5.12)~~ **done** | Unblocks everything else |
| 2 | ~~Embed instance in database responses (§5.1, §5.2)~~ **done** | Fixes a live bug; simplifies the §3 UI |
| 3 | Probe + reconcile, backend only (§3, §4) | Ship behind a flag; verify against a real fleet |
| 4 | Delete `test_connection` / `import_databases` (§4) | Only after #3 is proven |
| 5 | Merged `/databases` page + `/instances` redirect (§2.1) | The visible win |
| 6 | Sidebar consolidation + nested section routes (§2.2) | Independent of #5 |
| 7 | `<ConfirmDialog>`, toasts, hooks split (§5.4, 5.5, 5.7) | Cleanup, can run in parallel |

Steps 3 and 4 together are a net *reduction* in lines of code, which is a good
sanity check that the design is the simplification it claims to be.

---

## 7. Open questions

1. Should probe cadence be per-instance configurable, or one global setting?
   (Recommendation: global default + per-instance override, defaulting to null.)
2. Should a `missing` database auto-delete after N days, or require an explicit
   dismissal forever? (Recommendation: explicit forever — see §3.2.)
3. Does the flat "All databases" view stay a toggle on `/databases`, or does the
   grouped view get good enough search that it can be dropped?
4. `Activity` vs `Operations` as the nav label — worth checking against how
   users actually refer to it in issues.
