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

export interface Me {
  id: string;
  email: string;
  permissions: string[];
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
}
