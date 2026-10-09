export interface Paginated<T> {
  items: T[];
  pagination: { total: number; limit: number; offset: number };
}

export interface Server {
  id: string;
  name: string;
  hostname: string;
  address?: string | null;
  status: string;
  agent_version?: string | null;
  mariadb_version?: string | null;
  os?: string | null;
  labels: Record<string, string>;
  tags: string[];
  last_heartbeat_at?: string | null;
  created_at: string;
  updated_at: string;
  version: number;
}

export interface Instance {
  id: string;
  server_id?: string | null;
  name: string;
  engine: string;
  kind: "managed" | "external";
  host?: string | null;
  username?: string | null;
  has_credentials: boolean;
  tls_mode?: TLSMode;
  health?: InstanceHealth | null;
  provisioned: boolean;
  container_id?: string | null;
  engine_version: string;
  mariadb_version: string; // back-compat alias of engine_version
  port: number;
  status: string;
  labels: Record<string, string>;
  tags: string[];
  created_at: string;
  updated_at: string;
  version: number;
  /** SSH bastion this external instance is reached through; null when direct. */
  ssh_tunnel?: SSHTunnel | null;
  /** Login the table browser, SQL console, exports and imports connect as. */
  data_access?: DataAccess;
  data_username?: string | null;
}

/**
 * admin   — the instance's admin login (instance administrators only);
 * login   — a dedicated login (data_username);
 * managed — read-only / read-write roles Fleetdock creates per database.
 */
export type DataAccess = "admin" | "login" | "managed";

export type SSHAuthMethod = "password" | "key";

export interface SSHTunnel {
  host: string;
  port: number;
  username: string;
  auth_method: SSHAuthMethod;
  /** SHA256 fingerprint of the pinned bastion host key; null until the first connection. */
  host_key_fingerprint: string | null;
}

/** SSHTunnelInput sets a tunnel. Secrets are write-only; on update, omit them to keep the stored ones. */
export interface SSHTunnelInput {
  host: string;
  port?: number;
  username: string;
  auth_method: SSHAuthMethod;
  password?: string;
  private_key?: string;
  passphrase?: string;
}

/**
 * Summary of the instance a database lives on, embedded in database responses.
 * Read this instead of cross-referencing a separately fetched instance list —
 * that list is paginated, so the lookup silently failed for large fleets.
 */
export interface InstanceRef {
  id: string;
  name: string;
  engine: string;
  kind: "managed" | "external";
  server_id?: string | null;
  provisioned: boolean;
  has_credentials: boolean;
}

export interface Database {
  id: string;
  instance_id: string;
  /** Present on list/get responses; absent on write responses. */
  instance?: InstanceRef;
  name: string;
  charset: string;
  collation: string;
  status: string;
  /** Engine-owned database (postgres, mysql, sys): browsable and backup-able, never deletable. */
  system: boolean;
  size_bytes: number;
  active_connections: number;
  locked_at?: string | null;
  locked_by?: string | null;
  /** Set while the database is no longer found on its server. */
  missing_since?: string | null;
  labels: Record<string, string>;
  tags: string[];
  created_at: string;
  updated_at: string;
  version: number;
}

export interface EndpointView {
  id: string;
  status: string;
  host: string;
  port: number;
  protocol: string;
  tls_mode: string;
  tls_status: string;
  allowed_cidrs?: string[];
  last_error?: string | null;
  /** Connections HAProxy rejected because the source was not in allowed_cidrs. */
  denied_connections: number;
  sessions_total: number;
}

export interface GatewayInfo {
  enabled: boolean;
  public_host?: string;
  /** Port serving the source-IP diagnostic. Absent or 0 when disabled. */
  diag_port?: number;
  source_ip_mode?: string;
  /** Why this database cannot get a public endpoint (e.g. behind an SSH tunnel). */
  unavailable_reason?: string;
}

export interface Connectivity {
  private: EndpointView;
  public?: EndpointView | null;
  gateway: GatewayInfo;
}

export interface DatabaseCredential {
  id: string;
  name: string;
  username: string;
  access_level: string;
  account_host: string;
  expires_at?: string | null;
  revoked_at?: string | null;
  created_at: string;
}

export interface CredentialCreateResult {
  credential: DatabaseCredential;
  /** Shown once, on create and rotate. Never retrievable afterwards. */
  password: string;
  connection_url: string;
  /** Ready-to-paste command for the engine's own CLI. */
  cli_command?: string;
  fields: {
    host: string;
    port: number;
    user: string;
    database: string;
    ssl_mode?: string;
  };
}

export interface Operation {
  id: string;
  type: string;
  resource_type: string;
  resource_id?: string | null;
  /** Display name of the resource (a backup's database). */
  resource_name?: string;
  status: string;
  server_id?: string | null;
  params?: Record<string, unknown>;
  result?: Record<string, unknown>;
  error?: string | null;
  progress: number;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
}

export interface OperationLog {
  seq: number;
  level: string;
  message: string;
  created_at: string;
}

export interface Backup {
  id: string;
  database_id: string;
  database_name?: string;
  instance_name?: string;
  operation_id?: string | null;
  destination_id?: string | null;
  type: string;
  engine: string;
  status: string;
  storage_url?: string | null;
  size_bytes?: number | null;
  checksum?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  error?: string | null;
  created_at: string;
  /** Latest test restore: running, passed or failed; absent = never verified. */
  verify_status?: "running" | "passed" | "failed" | null;
  verified_at?: string | null;
  verify_error?: string | null;
}

export interface Destination {
  id: string;
  name: string;
  provider: "s3" | "r2" | "s3_compatible";
  bucket: string;
  region?: string;
  endpoint?: string;
  prefix?: string;
  access_key_id: string;
  created_at: string;
}

export interface AgentToken {
  id: string;
  name: string;
  expires_at: string;
  used_at?: string | null;
  server_id?: string | null;
  created_at: string;
}

export interface CreatedAgentToken extends AgentToken {
  token: string;
  install_command: string;
}

export interface ApiToken {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  last_used_at?: string | null;
  expires_at?: string | null;
  revoked_at?: string | null;
  created_at: string;
}

export type ScopeType = "global" | "server" | "database";

export interface Grant {
  permission: string;
  scope_type: ScopeType;
  scope_id?: string;
}

export interface Me {
  id: string;
  email: string;
  permissions: string[];
  grants: Grant[];
}

export interface RoleGrant {
  id: string;
  role: string;
  scope_type: ScopeType;
  scope_id?: string;
}

export interface AddGrantInput {
  role: string;
  scope_type: ScopeType;
  scope_id?: string;
}

export interface CreateServerInput {
  name: string;
  hostname: string;
  address?: string;
  tags?: string[];
  labels?: Record<string, string>;
}

export interface UpdateServerInput {
  name?: string;
  tags?: string[];
  labels?: Record<string, string>;
}

export interface CreateInstanceInput {
  kind?: "managed" | "external";
  server_id?: string;
  host?: string;
  name: string;
  engine?: string;
  engine_version?: string;
  mariadb_version?: string; // back-compat
  port: number;
  username?: string;
  password?: string;
  tls_mode?: TLSMode;
  ssh_tunnel?: SSHTunnelInput;
  data_access?: DataAccess;
  data_username?: string;
  data_password?: string;
}

// UpdateInstanceInput is a partial update: omitted keys are left unchanged.
// The two empty-string cases are meaningful, not accidental —
// `username: ""` clears the admin credentials entirely, and `password: ""`
// removes the stored password while keeping the username.
export type TLSMode = "disable" | "prefer" | "require" | "verify-full";

export interface UpdateInstanceInput {
  name?: string;
  host?: string;
  port?: number;
  tls_mode?: TLSMode;
  username?: string;
  password?: string;
  ssh_tunnel?: SSHTunnelInput;
  remove_ssh_tunnel?: boolean;
  reset_ssh_host_key?: boolean;
  data_access?: DataAccess;
  data_username?: string;
  /** Omit to keep the stored data password. */
  data_password?: string;
}

export interface CreateDatabaseInput {
  instance_id: string;
  name: string;
  charset?: string;
  collation?: string;
}

export interface ProvisionInstanceInput {
  server_id: string;
  name: string;
  engine?: string;
  engine_version: string;
  port: number;
}

export interface CreateDestinationInput {
  name: string;
  provider: string;
  bucket: string;
  region?: string;
  endpoint?: string;
  prefix?: string;
  access_key_id: string;
  secret_access_key: string;
}

export interface UpdateDestinationInput {
  name: string;
  provider: string;
  bucket: string;
  region?: string;
  endpoint?: string;
  prefix?: string;
  access_key_id: string;
  secret_access_key?: string;
}

export interface TriggerBackupInput {
  database_id: string;
  destination_id: string;
}

export interface RestoreBackupInput {
  backup_id: string;
  target_instance_id?: string;
  target_database?: string;
}

export interface StartMoveInput {
  source_database_id: string;
  target_instance_id: string;
  target_database?: string;
  destination_id: string;
  drop_source: boolean;
}

export interface CreateTokenInput {
  name: string;
  scopes?: string[];
  ttl_hours?: number;
}

export interface User {
  id: string;
  email: string;
  name: string;
  status: "active" | "suspended" | "invited";
  roles: string[];
  created_at: string;
  updated_at: string;
}

export interface Profile extends User {
  permissions: string[];
}

export interface Role {
  id: string;
  name: string;
  description: string;
  is_system: boolean;
  permissions: string[];
}

export interface CreateUserInput {
  name: string;
  email: string;
  password: string;
  role: string;
}

export interface UpdateUserInput {
  name?: string;
  email?: string;
  status?: string;
  role?: string;
}

export interface UpdateProfileInput {
  name?: string;
  email?: string;
}

export interface ChangePasswordInput {
  current_password: string;
  new_password: string;
}

export interface RoleInput {
  name?: string;
  description?: string;
  permissions?: string[];
}

// ---- Live DB administration ----
export interface DBUser {
  user: string;
  host: string;
}

export interface SchemaGrant {
  user: string;
  host: string;
  privileges: string[];
}

export interface TableInfo {
  name: string;
  /** Namespace holding the table. PostgreSQL: the schema; MySQL/MariaDB: the database. */
  schema: string;
  engine: string;
  row_count: number;
  data_bytes: number;
  index_bytes: number;
  comment: string;
}

export interface ColumnInfo {
  name: string;
  type: string;
  nullable: boolean;
  key: string; // PRI, UNI, MUL, or ""
  default: string | null;
  extra: string;
  comment: string;
}

export interface IndexInfo {
  name: string;
  columns: string[];
  unique: boolean;
  type: string;
}

export interface TableSchema {
  table: string;
  columns: ColumnInfo[];
  indexes: IndexInfo[];
  ddl: string;
}

export interface QueryResult {
  columns: string[];
  rows: (string | null)[][];
  row_count: number;
  truncated: boolean;
  rows_affected: number;
  read_only: boolean;
  duration_ms: number;
}

/** QueryOutput is a console run: one result per statement that ran. */
export interface QueryOutput {
  results: QueryResult[];
  /** Set when a statement after the first failed (1-based). */
  error?: { statement: number; message: string };
}

export interface HistoryEntry {
  id: string;
  database_id: string;
  sql: string;
  statements: number;
  duration_ms: number;
  row_count: number;
  error?: string;
  created_at: string;
}

export interface SavedQuery {
  id: string;
  database_id: string | null;
  name: string;
  sql: string;
  created_at: string;
  updated_at: string;
}

export interface RunQueryInput {
  sql: string;
  limit?: number;
  /** Client-generated UUID; lets the caller cancel the run. */
  query_id?: string;
}

export interface CreateDBUserInput {
  instance_id: string;
  username: string;
  host: string;
  password: string;
}

export interface GrantInput {
  username: string;
  host: string;
  database?: string;
  privileges?: string[];
}

// ---- Overview dashboard ----
export interface Overview {
  servers: { total: number; online: number; offline: number };
  instances: { total: number; managed: number; external: number };
  databases: { total: number; active: number };
  backups: { completed_24h: number; failed_24h: number; last_backup_at?: string | null };
  operations: { running: number; failed_24h: number };
  automation: { schedules_enabled: number; channels_enabled: number; rules_enabled: number };
  setup: { servers: number; instances: number; destinations: number; schedules: number; channels: number };
  /** Open problems the current user may see, critical first (at most 20). */
  attention: AttentionItem[];
}

export interface AttentionItem {
  kind:
    | "server_offline"
    | "instance_unreachable"
    | "backup_failed"
    | "backup_check_failed"
    | "no_recent_backup"
    | "database_missing"
    | "operation_failed";
  severity: "critical" | "warning";
  resource_type: "server" | "instance" | "database" | "backup" | "operation";
  resource_id: string;
  name: string;
  message: string;
  since: string;
}

// ---- Backup schedules ----
export interface Schedule {
  id: string;
  database_id: string;
  database_name?: string;
  instance_name?: string;
  destination_id: string;
  cron: string;
  engine: string;
  retention_days: number;
  enabled: boolean;
  last_run_at?: string | null;
  next_run_at?: string | null;
  created_at: string;
}

export interface CreateScheduleInput {
  database_id: string;
  destination_id: string;
  cron: string;
  retention_days: number;
  enabled: boolean;
}

export interface UpdateScheduleInput {
  destination_id: string;
  cron: string;
  retention_days: number;
  enabled: boolean;
}

// ---- Notification channels + alert rules ----
export type ChannelType = "email" | "slack" | "webhook";

export interface NotificationChannel {
  id: string;
  name: string;
  type: ChannelType;
  config: Record<string, string>;
  enabled: boolean;
  created_at: string;
}

export interface ChannelInput {
  name: string;
  type: ChannelType;
  config: Record<string, string>;
  enabled: boolean;
}

export interface AlertRule {
  id: string;
  name: string;
  target_type: string;
  target_id?: string | null;
  metric: string;
  comparator: string;
  threshold: number;
  for_seconds: number;
  severity: string;
  channel_ids: string[];
  enabled: boolean;
  created_at: string;
}

export interface RuleInput {
  name: string;
  target_type: string;
  target_id?: string;
  metric: string;
  comparator: string;
  threshold: number;
  for_seconds: number;
  severity: string;
  channel_ids: string[];
  enabled: boolean;
}

// ---- Metrics history ----
export interface MetricSample {
  collected_at: string;
  cpu_pct?: number | null;
  mem_used_bytes?: number | null;
  mem_total_bytes?: number | null;
  disk_used_bytes?: number | null;
  disk_total_bytes?: number | null;
  active_connections?: number | null;
}

export interface InstanceHealth {
  status: "healthy" | "unreachable" | "unknown";
  version?: string;
  latency_ms: number;
  error?: string;
  checked_at: string;
}

// Process is one session on an instance (PROCESSLIST / pg_stat_activity).
export interface Process {
  id: number;
  user: string;
  host: string;
  database: string | null;
  state: string;
  seconds: number;
  query: string | null;
}

export interface Setting {
  name: string;
  value: string;
}

// ---- Data editing ----
export type FilterOp =
  | "eq" | "ne" | "lt" | "lte" | "gt" | "gte"
  | "contains" | "starts_with" | "is_null" | "not_null";

export interface RowFilter {
  column: string;
  op: FilterOp;
  value?: string | null;
}

export interface SortKey {
  column: string;
  desc: boolean;
}

export interface BrowseRequest {
  filters?: RowFilter[];
  sort?: SortKey[];
  search?: string;
  limit?: number;
  offset?: number;
}

export interface BrowseColumn {
  name: string;
  type: string;
  nullable: boolean;
  has_default: boolean;
}

export interface BrowseResult {
  columns: BrowseColumn[];
  rows: (string | null)[][];
  total: number;
  total_exact: boolean;
  total_capped: boolean;
  /** Columns identifying a row; empty = the table is read-only here. */
  key: string[];
}

export type RowValues = Record<string, string | null>;

// ---- Structure ----
export interface ColumnSpec {
  name: string;
  type: string;
  nullable: boolean;
  default?: string | null;
  default_is_expression?: boolean;
  auto_increment?: boolean;
  comment?: string;
  /** modify_column only: leave the current default as it is. */
  keep_default?: boolean;
}

export interface ForeignKey {
  name: string;
  columns: string[];
  ref_table: string;
  ref_columns: string[];
  on_delete: string;
  on_update: string;
}

export interface TableSpec {
  name: string;
  columns: ColumnSpec[];
  primary_key: string[];
  foreign_keys?: Partial<ForeignKey>[];
  comment?: string;
}

export type AlterOp =
  | { op: "add_column"; column: ColumnSpec }
  | { op: "drop_column"; name: string }
  | { op: "modify_column"; name: string; column: ColumnSpec }
  | { op: "rename_column"; name: string; new_name: string }
  | { op: "add_foreign_key"; foreign_key: Partial<ForeignKey> }
  | { op: "drop_foreign_key"; name: string };

export interface DBObject {
  kind: "view" | "materialized_view" | "function" | "procedure" | "trigger" | "sequence" | "event";
  schema: string;
  name: string;
  table?: string;
  definition: string | null;
}
