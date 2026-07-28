-- =============================================================================
-- Migration 0012 — protected system databases.
--
-- Database discovery now imports the engine-owned databases (PostgreSQL's
-- "postgres" maintenance database, MySQL/MariaDB's "mysql" and "sys" schemas)
-- so they can be browsed and backed up. They must never be dropped: the
-- control plane connects to the maintenance database for every admin
-- operation, and the MySQL catalogue holds the account definitions. This flag
-- marks those rows so the delete path can refuse them.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE databases ADD COLUMN IF NOT EXISTS system boolean NOT NULL DEFAULT false;

-- Backfill: any row that was registered manually before discovery flagged them.
UPDATE databases d
SET system = true
FROM instances i
WHERE d.instance_id = i.id
  AND d.system = false
  AND (
    (i.engine = 'postgres' AND d.name = 'postgres')
    OR (i.engine IN ('mysql', 'mariadb') AND d.name IN ('mysql', 'sys'))
  );
