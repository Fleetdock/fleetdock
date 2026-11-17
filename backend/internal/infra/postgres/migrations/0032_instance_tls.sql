-- =============================================================================
-- Migration 0032 — TLS on managed instances.
--
-- tls_enabled is set once the engine serves TLS with a certificate from the
-- install's certificate authority; tls_expires_at is when that certificate
-- expires (configure_tls renews it). External instances keep false.
-- =============================================================================

ALTER TABLE instances ADD COLUMN IF NOT EXISTS tls_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS tls_expires_at timestamptz;

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN (
  'create_database','delete_database','clone_database','rename_database',
  'lock_database','unlock_database','backup','restore','migrate',
  'provision_instance','start_instance','stop_instance','restart_instance','remove_instance',
  'enroll_server','test_connection','import_databases','reconcile_gateway',
  'copy_schema','restore_schema','configure_tls'
));
