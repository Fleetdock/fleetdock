-- =============================================================================
-- Migration 0031 — backups of one PostgreSQL schema.
--
-- schema_name is set on a backup of one schema of a database (pg_dump -n);
-- NULL is a whole-database backup. restore_schema restores such a backup into
-- a database, under its own name or another one.
-- =============================================================================

ALTER TABLE backups ADD COLUMN IF NOT EXISTS schema_name text;

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN (
  'create_database','delete_database','clone_database','rename_database',
  'lock_database','unlock_database','backup','restore','migrate',
  'provision_instance','start_instance','stop_instance','restart_instance','remove_instance',
  'enroll_server','test_connection','import_databases','reconcile_gateway',
  'copy_schema','restore_schema'
));
