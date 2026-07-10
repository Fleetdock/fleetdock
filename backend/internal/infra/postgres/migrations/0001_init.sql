-- =============================================================================
-- Fleetdock — Control-plane metadata schema
-- Migration: 0001_init.sql   (PostgreSQL 15+)
--
-- Design rules baked into this schema:
--   * All ids are UUID v4 (gen_random_uuid, pgcrypto).
--   * Every mutable row carries created_at, updated_at, and an integer `version`
--     column for OPTIMISTIC LOCKING. The application increments version in the
--     UPDATE ... WHERE id = $1 AND version = $2 statement. The updated_at trigger
--     deliberately does NOT touch version (so it never double-bumps).
--   * Resources that must never be hard-deleted casually use `deleted_at`
--     (soft delete) plus, where a recovery window applies, `purge_after`.
--   * State machines are enforced with CHECK constraints, not native ENUMs, so
--     new states are cheap migrations rather than type rewrites.
--   * The schema is GENERIC / open-source: no tenant, billing, or customer
--     onboarding logic. It manages servers, MariaDB instances, and databases.
-- =============================================================================


CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;     -- case-insensitive email

-- -----------------------------------------------------------------------------
-- Shared helpers
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Blocks UPDATE/DELETE on append-only tables (audit chain integrity).
CREATE OR REPLACE FUNCTION forbid_mutation() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'table % is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- Identity & access
-- =============================================================================
CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email         citext NOT NULL UNIQUE,
  name          text   NOT NULL,
  password_hash text,                                  -- null for SSO-only users (future)
  status        text   NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active','suspended','invited')),
  last_login_at timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  version       integer     NOT NULL DEFAULT 0
);

CREATE TABLE roles (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        text NOT NULL UNIQUE,
  description text NOT NULL DEFAULT '',
  is_system   boolean NOT NULL DEFAULT false,          -- system roles cannot be deleted
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- Permission catalog stored as strings (e.g. 'database:create', 'server:read')
-- so the set is data-driven and auditable. Full catalog defined in Phase 9.
CREATE TABLE role_permissions (
  role_id    uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  permission text NOT NULL,
  PRIMARY KEY (role_id, permission)
);

-- A role grant can be global or scoped to a single server or database (RBAC tree).
CREATE TABLE user_roles (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id    uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  scope_type text NOT NULL DEFAULT 'global'
                  CHECK (scope_type IN ('global','server','database')),
  scope_id   uuid,                                     -- null when scope_type = 'global'
  granted_by uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((scope_type = 'global') = (scope_id IS NULL))
);
-- Dedupe grants (UNIQUE treats NULLs as distinct, so split into two partial indexes).
CREATE UNIQUE INDEX uq_user_roles_global ON user_roles (user_id, role_id)
  WHERE scope_type = 'global';
CREATE UNIQUE INDEX uq_user_roles_scoped ON user_roles (user_id, role_id, scope_type, scope_id)
  WHERE scope_id IS NOT NULL;

CREATE TABLE api_tokens (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         text NOT NULL,
  prefix       text NOT NULL,                          -- non-secret display prefix, e.g. 'fleetd_ab12'
  token_hash   text NOT NULL UNIQUE,                   -- sha256 of the full token; raw token never stored
  scopes       text[] NOT NULL DEFAULT '{}',           -- subset of the permission catalog
  last_used_at timestamptz,
  expires_at   timestamptz,
  revoked_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_api_tokens_user ON api_tokens (user_id);

-- =============================================================================
-- Secrets — envelope-encrypted. Postgres stores ciphertext + key references only.
-- =============================================================================
CREATE TABLE secrets (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  ref               text NOT NULL UNIQUE,              -- logical reference used by other rows
  kind              text NOT NULL
                         CHECK (kind IN ('mariadb_root','mariadb_user','ssh_key',
                                         's3_credential','tls_cert','agent_enrollment','other')),
  ciphertext        bytea NOT NULL,                    -- payload encrypted by the data key
  encrypted_data_key bytea NOT NULL,                   -- data key wrapped by the master/KMS key
  key_id            text  NOT NULL,                    -- which master/KMS key wrapped the data key
  nonce             bytea NOT NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  rotated_at        timestamptz
);

-- =============================================================================
-- Fleet: servers + latest health snapshot
-- =============================================================================
CREATE TABLE servers (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name              text NOT NULL,
  hostname          text NOT NULL,
  address           inet,
  status            text NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending','online','offline','draining','error')),
  agent_version     text,
  agent_enrolled_at timestamptz,
  agent_secret_ref  text REFERENCES secrets(ref) ON DELETE SET NULL,   -- mTLS / enrollment material
  mariadb_version   text,
  os                text,
  labels            jsonb  NOT NULL DEFAULT '{}',
  tags              text[] NOT NULL DEFAULT '{}',
  last_heartbeat_at timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  version           integer NOT NULL DEFAULT 0,
  deleted_at        timestamptz
);
CREATE UNIQUE INDEX uq_servers_name ON servers (name) WHERE deleted_at IS NULL;

-- Latest snapshot only; time-series history lives in Prometheus/VictoriaMetrics.
CREATE TABLE server_health (
  server_id         uuid PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
  cpu_pct           numeric(5,2),
  mem_used_bytes    bigint,
  mem_total_bytes   bigint,
  disk_used_bytes   bigint,
  disk_total_bytes  bigint,
  docker_ok         boolean,
  active_connections integer,
  collected_at      timestamptz NOT NULL DEFAULT now()
);

-- =============================================================================
-- Managed resources: MariaDB instances and databases
-- =============================================================================
CREATE TABLE instances (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  server_id       uuid NOT NULL REFERENCES servers(id) ON DELETE RESTRICT,
  name            text NOT NULL,
  container_id    text,
  mariadb_version text NOT NULL,
  port            integer NOT NULL,
  status          text NOT NULL DEFAULT 'provisioning'
                       CHECK (status IN ('provisioning','running','stopped','error','deleting')),
  root_secret_ref text REFERENCES secrets(ref) ON DELETE SET NULL,
  config          jsonb  NOT NULL DEFAULT '{}',
  labels          jsonb  NOT NULL DEFAULT '{}',
  tags            text[] NOT NULL DEFAULT '{}',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  version         integer NOT NULL DEFAULT 0,
  deleted_at      timestamptz
);
CREATE UNIQUE INDEX uq_instances_name ON instances (server_id, name) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_instances_port ON instances (server_id, port) WHERE deleted_at IS NULL;

CREATE TABLE databases (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  instance_id        uuid NOT NULL REFERENCES instances(id) ON DELETE RESTRICT,
  name               text NOT NULL,
  charset            text NOT NULL DEFAULT 'utf8mb4',
  "collation"        text NOT NULL DEFAULT 'utf8mb4_unicode_ci',   -- quoted: reserved word in PostgreSQL
  status             text NOT NULL DEFAULT 'creating'
                          CHECK (status IN ('creating','active','locked','migrating','deleting','error')),
  size_bytes         bigint  NOT NULL DEFAULT 0,
  active_connections integer NOT NULL DEFAULT 0,
  locked_at          timestamptz,
  locked_by          uuid REFERENCES users(id) ON DELETE SET NULL,
  labels             jsonb  NOT NULL DEFAULT '{}',
  tags               text[] NOT NULL DEFAULT '{}',
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  version            integer NOT NULL DEFAULT 0,
  deleted_at         timestamptz,                       -- soft delete: enters recovery window
  purge_after        timestamptz                        -- when the window ends and data is truly dropped
);
CREATE UNIQUE INDEX uq_databases_name ON databases (instance_id, name) WHERE deleted_at IS NULL;
CREATE INDEX ix_databases_status ON databases (status);
CREATE INDEX ix_databases_labels ON databases USING gin (labels);
CREATE INDEX ix_databases_tags   ON databases USING gin (tags);
CREATE INDEX ix_databases_purge  ON databases (purge_after) WHERE deleted_at IS NOT NULL;

-- =============================================================================
-- Orchestration: jobs (also serves as the operations log), migrations
-- =============================================================================
CREATE TABLE jobs (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  type            text NOT NULL
                       CHECK (type IN ('create_database','delete_database','clone_database',
                                       'rename_database','lock_database','unlock_database',
                                       'backup','restore','migrate',
                                       'provision_instance','enroll_server')),
  resource_type   text NOT NULL,                        -- 'database' | 'instance' | 'server' | 'backup'
  resource_id     uuid,
  status          text NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','running','succeeded','failed','compensating','canceled')),
  workflow_id     text,                                 -- Temporal workflow id
  run_id          text,                                 -- Temporal run id
  idempotency_key text,
  params          jsonb NOT NULL DEFAULT '{}',
  result          jsonb,
  error           text,
  progress        integer NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
  created_by      uuid REFERENCES users(id) ON DELETE SET NULL,
  started_at      timestamptz,
  completed_at    timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  version         integer NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX uq_jobs_idempotency ON jobs (idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX uq_jobs_workflow    ON jobs (workflow_id)     WHERE workflow_id IS NOT NULL;
CREATE INDEX ix_jobs_resource ON jobs (resource_type, resource_id);
CREATE INDEX ix_jobs_status   ON jobs (status);
CREATE INDEX ix_jobs_created  ON jobs (created_at DESC);

CREATE TABLE migrations (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  job_id             uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  database_id        uuid NOT NULL REFERENCES databases(id) ON DELETE RESTRICT,
  source_instance_id uuid NOT NULL REFERENCES instances(id) ON DELETE RESTRICT,
  dest_instance_id   uuid NOT NULL REFERENCES instances(id) ON DELETE RESTRICT,
  phase              text NOT NULL DEFAULT 'validate'
                          CHECK (phase IN ('validate','copy','verify','cutover','cleanup','rolled_back')),
  source_checksum    text,
  dest_checksum      text,
  bytes_copied       bigint NOT NULL DEFAULT 0,
  cutover_at         timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  version            integer NOT NULL DEFAULT 0
);

-- =============================================================================
-- Backups + schedules
-- =============================================================================
CREATE TABLE backup_schedules (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  database_id    uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  cron           text NOT NULL,
  engine         text NOT NULL CHECK (engine IN ('mydumper','mariabackup')),
  retention_days integer NOT NULL DEFAULT 30 CHECK (retention_days > 0),
  target         text NOT NULL DEFAULT 's3',
  enabled        boolean NOT NULL DEFAULT true,
  last_run_at    timestamptz,
  next_run_at    timestamptz,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  version        integer NOT NULL DEFAULT 0
);

CREATE TABLE backups (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  database_id    uuid NOT NULL REFERENCES databases(id) ON DELETE RESTRICT,
  schedule_id    uuid REFERENCES backup_schedules(id) ON DELETE SET NULL,
  job_id         uuid REFERENCES jobs(id) ON DELETE SET NULL,
  type           text NOT NULL CHECK (type IN ('manual','scheduled')),
  engine         text NOT NULL CHECK (engine IN ('mydumper','mariabackup')),
  status         text NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','running','completed','failed','expired','deleted')),
  storage_url    text,                                  -- s3://bucket/key
  size_bytes     bigint,
  checksum       text,                                  -- sha256, verified on restore
  binlog_position text,                                 -- anchor for point-in-time recovery
  started_at     timestamptz,
  completed_at   timestamptz,
  expires_at     timestamptz,                           -- retention boundary
  error          text,
  created_by     uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  version        integer NOT NULL DEFAULT 0
);
CREATE INDEX ix_backups_database ON backups (database_id, created_at DESC);
CREATE INDEX ix_backups_status   ON backups (status);
CREATE INDEX ix_backups_expiry   ON backups (expires_at) WHERE status = 'completed';

-- =============================================================================
-- Transactional outbox — intent + outbox commit atomically, dispatcher starts
-- the Temporal workflow / emits the event afterwards (exactly-once semantics).
-- =============================================================================
CREATE TABLE outbox (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  aggregate_type text NOT NULL,
  aggregate_id   uuid NOT NULL,
  event_type     text NOT NULL,
  payload        jsonb NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  published_at   timestamptz,
  attempts       integer NOT NULL DEFAULT 0
);
CREATE INDEX ix_outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;

-- =============================================================================
-- Audit log — append-only, hash-chained (tamper-evident)
-- =============================================================================
CREATE TABLE audit_log (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_type    text NOT NULL CHECK (actor_type IN ('user','token','system')),
  actor_id      uuid,
  action        text NOT NULL,                          -- e.g. 'database.create'
  resource_type text NOT NULL,
  resource_id   uuid,
  before        jsonb,
  after         jsonb,
  metadata      jsonb NOT NULL DEFAULT '{}',
  prev_hash     bytea,                                  -- hash of the previous row
  hash          bytea NOT NULL,                         -- sha256(prev_hash || canonical(this row))
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_audit_resource ON audit_log (resource_type, resource_id);
CREATE INDEX ix_audit_actor    ON audit_log (actor_id);
CREATE INDEX ix_audit_created  ON audit_log (created_at);
CREATE TRIGGER trg_audit_immutable
  BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- =============================================================================
-- Automation: notification channels + alert rules
-- =============================================================================
CREATE TABLE notification_channels (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name       text NOT NULL,
  type       text NOT NULL CHECK (type IN ('email','slack','webhook')),
  config     jsonb NOT NULL DEFAULT '{}',
  enabled    boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  version    integer NOT NULL DEFAULT 0
);

CREATE TABLE alert_rules (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        text NOT NULL,
  target_type text NOT NULL CHECK (target_type IN ('server','instance','database','global')),
  target_id   uuid,
  metric      text NOT NULL,                            -- 'cpu_pct','disk_used_pct','connections','replication_lag'
  comparator  text NOT NULL CHECK (comparator IN ('gt','gte','lt','lte')),
  threshold   numeric NOT NULL,
  for_seconds integer NOT NULL DEFAULT 60,
  severity    text NOT NULL DEFAULT 'warning' CHECK (severity IN ('info','warning','critical')),
  channel_ids uuid[] NOT NULL DEFAULT '{}',
  enabled     boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  version     integer NOT NULL DEFAULT 0
);

-- =============================================================================
-- updated_at triggers
-- =============================================================================
CREATE TRIGGER trg_users_updated                BEFORE UPDATE ON users                FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_servers_updated              BEFORE UPDATE ON servers              FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_instances_updated            BEFORE UPDATE ON instances            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_databases_updated            BEFORE UPDATE ON databases            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_jobs_updated                 BEFORE UPDATE ON jobs                 FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_migrations_updated           BEFORE UPDATE ON migrations           FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_backup_schedules_updated     BEFORE UPDATE ON backup_schedules     FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_notification_channels_updated BEFORE UPDATE ON notification_channels FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_alert_rules_updated          BEFORE UPDATE ON alert_rules          FOR EACH ROW EXECUTE FUNCTION set_updated_at();

