-- =============================================================================
-- Migration 0011 — external database access: endpoints, credentials, audit.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

-- Extend secret kinds for application credentials.
ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_kind_check;
ALTER TABLE secrets ADD CONSTRAINT secrets_kind_check CHECK (kind IN (
  'mariadb_root','mariadb_user','postgres_user','ssh_key',
  's3_credential','tls_cert','agent_enrollment','other'
));

-- Public/private connectivity endpoints for managed databases.
CREATE TABLE database_endpoints (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  database_id     uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  access_type     text NOT NULL CHECK (access_type IN ('private','public')),
  status          text NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','active','disabling','disabled','error')),
  protocol        text NOT NULL CHECK (protocol IN ('postgresql','mysql','mariadb')),
  external_host   text NOT NULL,
  external_port   integer,
  internal_host   text NOT NULL,
  internal_port   integer NOT NULL,
  tls_mode        text NOT NULL DEFAULT 'required'
                       CHECK (tls_mode IN ('required','preferred','disabled')),
  tls_status      text NOT NULL DEFAULT 'unknown'
                       CHECK (tls_status IN ('required','preferred','disabled','unsupported','misconfigured','unknown')),
  allowed_cidrs   text[] NOT NULL,
  max_connections integer,
  last_error      text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  disabled_at     timestamptz,
  version         integer NOT NULL DEFAULT 0,
  CONSTRAINT database_endpoints_public_needs_cidrs
    CHECK (access_type <> 'public' OR cardinality(allowed_cidrs) > 0)
);
CREATE INDEX ix_database_endpoints_database ON database_endpoints (database_id);
CREATE INDEX ix_database_endpoints_access_status ON database_endpoints (access_type, status);
CREATE UNIQUE INDEX uq_database_endpoints_public ON database_endpoints (database_id)
  WHERE access_type = 'public' AND status NOT IN ('disabled');
CREATE UNIQUE INDEX uq_database_endpoints_port ON database_endpoints (external_port)
  WHERE access_type = 'public' AND status IN ('pending','active','disabling','error') AND external_port IS NOT NULL;

CREATE TRIGGER database_endpoints_set_updated_at
  BEFORE UPDATE ON database_endpoints
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Application credentials scoped to a logical database.
CREATE TABLE database_credentials (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  database_id   uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  name          text NOT NULL,
  username      text NOT NULL,
  secret_ref    text NOT NULL REFERENCES secrets(ref) ON DELETE RESTRICT,
  access_level  text NOT NULL CHECK (access_level IN ('readonly','readwrite','admin','custom')),
  account_host  text NOT NULL DEFAULT '%',
  expires_at    timestamptz,
  revoked_at    timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  version       integer NOT NULL DEFAULT 0
);
CREATE INDEX ix_database_credentials_database ON database_credentials (database_id);
CREATE UNIQUE INDEX uq_database_credentials_name ON database_credentials (database_id, name)
  WHERE revoked_at IS NULL;
CREATE UNIQUE INDEX uq_database_credentials_username ON database_credentials (database_id, username)
  WHERE revoked_at IS NULL;

CREATE TRIGGER database_credentials_set_updated_at
  BEFORE UPDATE ON database_credentials
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Minimal audit trail (reintroduced after 0009_drop_audit).
CREATE TABLE audit_events (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  action        text NOT NULL,
  resource_type text NOT NULL,
  resource_id   uuid,
  metadata      jsonb NOT NULL DEFAULT '{}',
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_audit_events_resource ON audit_events (resource_type, resource_id);
CREATE INDEX ix_audit_events_created ON audit_events (created_at DESC);

CREATE TRIGGER audit_events_forbid_mutation
  BEFORE UPDATE OR DELETE ON audit_events
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- reconcile_gateway operation type.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN (
  'create_database','delete_database','clone_database','rename_database',
  'lock_database','unlock_database','backup','restore','migrate',
  'provision_instance','start_instance','stop_instance','restart_instance','remove_instance',
  'enroll_server','test_connection','import_databases','reconcile_gateway'
));

-- audit:read permission for owner/admin.
INSERT INTO role_permissions (role_id, permission)
SELECT r.id, 'audit:read'
FROM roles r
WHERE r.name IN ('owner', 'admin')
ON CONFLICT (role_id, permission) DO NOTHING;
