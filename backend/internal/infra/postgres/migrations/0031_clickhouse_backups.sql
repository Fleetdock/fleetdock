-- =============================================================================
-- Migration 0031 — ClickHouse backups.
--
-- ClickHouse backups are written by Fleetdock itself (the "clickhouse-native"
-- dump: definitions plus Native-format rows), recorded under that tool name.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE backups DROP CONSTRAINT IF EXISTS backups_engine_check;
ALTER TABLE backups ADD CONSTRAINT backups_engine_check
  CHECK (engine IN ('mydumper','mariabackup','mariadb-dump','mysqldump','pg_dump','clickhouse-native'));
