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
  server_id: string;
  name: string;
  mariadb_version: string;
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
  server_id: string;
  name: string;
  mariadb_version: string;
  port: number;
}

export interface CreateDatabaseInput {
  instance_id: string;
  name: string;
  charset?: string;
  collation?: string;
}

export interface CreateTokenInput {
  name: string;
  scopes?: string[];
}
