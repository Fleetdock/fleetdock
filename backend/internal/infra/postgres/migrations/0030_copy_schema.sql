-- =============================================================================
-- Migration 0030 — copy a PostgreSQL schema.
--
-- copy_schema copies one schema of a database to a new schema, in the same
-- database or another one on the instance. It runs where backups run (the
-- agent or the control plane), through a scratch database.
-- =============================================================================

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN (
  'create_database','delete_database','clone_database','rename_database',
  'lock_database','unlock_database','backup','restore','migrate',
  'provision_instance','start_instance','stop_instance','restart_instance','remove_instance',
  'enroll_server','test_connection','import_databases','reconcile_gateway',
  'copy_schema'
));
