-- =============================================================================
-- Migration 0030 — ClickHouse instances.
--
-- ClickHouse joins the engines an instance can run, its account passwords
-- (console roles, application credentials) get their own secret kind, and
-- its HTTP interface becomes a connectivity protocol.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE instances DROP CONSTRAINT IF EXISTS instances_engine_check;
ALTER TABLE instances ADD CONSTRAINT instances_engine_check
  CHECK (engine IN ('mariadb','mysql','postgres','clickhouse'));

ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_kind_check;
ALTER TABLE secrets ADD CONSTRAINT secrets_kind_check CHECK (kind IN (
  'mariadb_root','mariadb_user','postgres_user','clickhouse_user','ssh_key',
  's3_credential','tls_cert','agent_enrollment','other'
));

ALTER TABLE database_endpoints DROP CONSTRAINT IF EXISTS database_endpoints_protocol_check;
ALTER TABLE database_endpoints ADD CONSTRAINT database_endpoints_protocol_check
  CHECK (protocol IN ('postgresql','mysql','mariadb','clickhouse'));
