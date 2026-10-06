-- =============================================================================
-- Migration 0013 — record the real dump tool on each backup.
--
-- Backups were stamped "mariadb-dump" regardless of engine. PostgreSQL backups
-- are taken with pg_dump and MySQL ones with mysqldump; widen the CHECK so the
-- row says what actually produced the artifact, and fix up existing rows.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE backups DROP CONSTRAINT IF EXISTS backups_engine_check;
ALTER TABLE backups ADD CONSTRAINT backups_engine_check
  CHECK (engine IN ('mydumper','mariabackup','mariadb-dump','mysqldump','pg_dump'));

UPDATE backups b
SET engine = CASE i.engine WHEN 'postgres' THEN 'pg_dump' WHEN 'mysql' THEN 'mysqldump' ELSE b.engine END
FROM databases d JOIN instances i ON i.id = d.instance_id
WHERE b.database_id = d.id AND b.engine = 'mariadb-dump' AND i.engine IN ('postgres','mysql');
