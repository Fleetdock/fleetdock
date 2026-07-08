-- =============================================================================
-- Migration 0005 — instance provisioning: agent launches DB engines via Docker.
-- Adds the container lifecycle operation types. `provision_instance` was
-- already permitted by the 0003 jobs_type_check.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE jobs DROP CONSTRAINT jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN (
  'create_database','delete_database','clone_database','rename_database',
  'lock_database','unlock_database','backup','restore','migrate',
  'provision_instance','start_instance','stop_instance','restart_instance','remove_instance',
  'enroll_server','test_connection','import_databases'
));
