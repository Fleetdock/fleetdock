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
}

export interface Database {
  id: string;
  instance_id: string;
  name: string;
  charset: string;
  collation: string;
  status: string;
  size_bytes: number;
  active_connections: number;
  locked_at?: string | null;
  locked_by?: string | null;
  labels: Record<string, string>;
  tags: string[];
  created_at: string;
  updated_at: string;
  version: number;
}

export interface Operation {
  id: string;
  type: string;
  resource_type: string;
  resource_id?: string | null;
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

export interface TestConnectionResult {
  mode: "sync" | "async";
  ok: boolean;
  version?: string;
  error?: string;
  operation_id?: string;
}

export interface ImportDatabasesResult {
  mode: "sync" | "async";
  imported: number;
  operation_id?: string;
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
  engine: string;
  row_count: number;
  data_bytes: number;
  index_bytes: number;
  comment: string;
}

export interface RowsPage {
  columns: string[];
  rows: (string | null)[][];
  total: number;
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

export interface RunQueryInput {
  sql: string;
  limit?: number;
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
}

// ---- Backup schedules ----
export interface Schedule {
  id: string;
  database_id: string;
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

// ---- Audit log ----
export interface AuditEntry {
  id: number;
  actor_type: string;
  actor_id?: string | null;
  action: string;
  resource_type: string;
  resource_id?: string | null;
  metadata: Record<string, unknown>;
  created_at: string;
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
