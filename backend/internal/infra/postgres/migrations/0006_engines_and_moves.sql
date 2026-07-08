-- =============================================================================
-- Migration 0006 — multi-engine support (MySQL, PostgreSQL) and the
-- move-database saga (backup -> restore -> verify -> optional drop of source).
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

-- ---- More engines -----------------------------------------------------------
ALTER TABLE instances DROP CONSTRAINT instances_engine_check;
ALTER TABLE instances ADD CONSTRAINT instances_engine_check
  CHECK (engine IN ('mariadb','mysql','postgres'));

-- ---- Move-database saga -----------------------------------------------------
-- One row tracks a move as it advances through its sub-operations. The backup
-- and restore jobs are linked so the operation-completion hook can advance it.
CREATE TABLE db_moves (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_database_id uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  target_instance_id uuid NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
  target_database    text NOT NULL,
  destination_id     uuid NOT NULL REFERENCES backup_destinations(id) ON DELETE RESTRICT,
  drop_source        boolean NOT NULL DEFAULT false,
  backup_id          uuid REFERENCES backups(id) ON DELETE SET NULL,
  restore_job_id     uuid REFERENCES jobs(id) ON DELETE SET NULL,
  status             text NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','backing_up','restoring','completed','failed')),
  table_count        integer,
  error              text,
  created_by         uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  version            integer NOT NULL DEFAULT 0
);
CREATE INDEX ix_db_moves_backup  ON db_moves (backup_id);
CREATE INDEX ix_db_moves_restore ON db_moves (restore_job_id);
CREATE TRIGGER trg_db_moves_updated BEFORE UPDATE ON db_moves
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
