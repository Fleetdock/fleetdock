-- =============================================================================
-- Migration 0003 — agents, engine abstraction, external instances,
-- operations routing, backup destinations, new permissions.
-- =============================================================================

-- ---- Engine abstraction + external instances -------------------------------
-- `engine` makes the schema multi-engine ready (mariadb first, postgres/mysql
-- later: extending the CHECK is a cheap migration).
ALTER TABLE instances ADD COLUMN engine text NOT NULL DEFAULT 'mariadb'
  CHECK (engine IN ('mariadb'));
ALTER TABLE instances ADD COLUMN kind text NOT NULL DEFAULT 'managed'
  CHECK (kind IN ('managed','external'));
ALTER TABLE instances ADD COLUMN host text;           -- external instances only
ALTER TABLE instances ADD COLUMN username text;       -- admin user for SQL ops
-- external instances have no server; managed ones require one
ALTER TABLE instances ALTER COLUMN server_id DROP NOT NULL;
ALTER TABLE instances ADD CONSTRAINT instances_kind_server CHECK (
  (kind = 'managed' AND server_id IS NOT NULL) OR
  (kind = 'external' AND server_id IS NULL AND host IS NOT NULL)
);

-- generic engine version (mariadb_version kept for backward compat reads)
ALTER TABLE instances ALTER COLUMN mariadb_version SET DEFAULT '';

-- ---- Agent enrollment -------------------------------------------------------
ALTER TABLE servers ADD COLUMN agent_token_hash text UNIQUE;

CREATE TABLE agent_registration_tokens (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name       text NOT NULL DEFAULT '',
  token_hash text NOT NULL UNIQUE,
  created_by uuid REFERENCES users(id) ON DELETE SET NULL,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  server_id  uuid REFERENCES servers(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- ---- Operations (jobs) routing ----------------------------------------------
-- server_id = executor agent; NULL = executed by the control plane itself
-- (external instances). claimed_at supports at-least-once claim semantics.
ALTER TABLE jobs ADD COLUMN server_id uuid REFERENCES servers(id) ON DELETE SET NULL;
ALTER TABLE jobs ADD COLUMN claimed_at timestamptz;
ALTER TABLE jobs DROP CONSTRAINT jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN (
  'create_database','delete_database','clone_database','rename_database',
  'lock_database','unlock_database','backup','restore','migrate',
  'provision_instance','enroll_server','test_connection','import_databases'
));
CREATE INDEX ix_jobs_claim ON jobs (server_id, created_at) WHERE status = 'pending';

-- ---- Backup destinations (S3 / Cloudflare R2 / any S3-compatible) -----------
CREATE TABLE backup_destinations (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name           text NOT NULL,
  provider       text NOT NULL CHECK (provider IN ('s3','r2','s3_compatible')),
  bucket         text NOT NULL,
  region         text NOT NULL DEFAULT '',
  endpoint       text NOT NULL DEFAULT '',              -- empty = AWS default
  prefix         text NOT NULL DEFAULT '',
  access_key_id  text NOT NULL,
  secret_ref     text NOT NULL REFERENCES secrets(ref) ON DELETE RESTRICT,
  created_by     uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  version        integer NOT NULL DEFAULT 0,
  deleted_at     timestamptz
);
CREATE UNIQUE INDEX uq_backup_destinations_name ON backup_destinations (name) WHERE deleted_at IS NULL;
CREATE TRIGGER trg_backup_destinations_updated BEFORE UPDATE ON backup_destinations
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE backups ADD COLUMN destination_id uuid REFERENCES backup_destinations(id) ON DELETE SET NULL;
ALTER TABLE backups DROP CONSTRAINT backups_engine_check;
ALTER TABLE backups ADD CONSTRAINT backups_engine_check
  CHECK (engine IN ('mydumper','mariabackup','mariadb-dump'));

-- ---- New permissions ---------------------------------------------------------
INSERT INTO role_permissions (role_id, permission)
SELECT r.id, p.perm
FROM roles r
JOIN (VALUES
  ('owner', 'operation:read'),  ('owner', 'operation:write'),
  ('owner', 'backup:read'),     ('owner', 'backup:write'),
  ('owner', 'destination:read'),('owner', 'destination:write'),

  ('admin', 'operation:read'),  ('admin', 'operation:write'),
  ('admin', 'backup:read'),     ('admin', 'backup:write'),
  ('admin', 'destination:read'),('admin', 'destination:write'),

  ('operator', 'operation:read'), ('operator', 'operation:write'),
  ('operator', 'backup:read'),    ('operator', 'backup:write'),
  ('operator', 'destination:read'),

  ('viewer', 'operation:read'),
  ('viewer', 'backup:read'),
  ('viewer', 'destination:read')
) AS p(role_name, perm) ON p.role_name = r.name
ON CONFLICT (role_id, permission) DO NOTHING;
