-- =============================================================================
-- Migration 0025 — backup copies ("data time machine").
--
-- A backup copy is a backup restored into a temporary database (fds_<hex>)
-- on the same instance as the database it came from, kept for a while so a
-- table can be compared with it and rows put back. The worker drops it when
-- it expires.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

CREATE TABLE IF NOT EXISTS backup_copies (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  database_id   uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  backup_id     uuid NOT NULL REFERENCES backups(id) ON DELETE CASCADE,
  instance_id   uuid NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
  scratch_name  text NOT NULL,
  status        text NOT NULL CHECK (status IN ('restoring', 'ready', 'failed', 'dropping', 'dropped')),
  job_id        uuid,
  drop_job_id   uuid,
  drop_attempts integer NOT NULL DEFAULT 0,
  error         text,
  ttl_seconds   integer NOT NULL,
  ready_at      timestamptz,
  expires_at    timestamptz,
  created_by    uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
-- One live copy per database.
CREATE UNIQUE INDEX IF NOT EXISTS uq_backup_copies_live ON backup_copies (database_id)
  WHERE status IN ('restoring', 'ready');
CREATE INDEX IF NOT EXISTS ix_backup_copies_due ON backup_copies (status, expires_at);

-- Rows put back from a backup copy are journaled too (undoable).
ALTER TABLE row_changes DROP CONSTRAINT IF EXISTS row_changes_source_check;
ALTER TABLE row_changes ADD CONSTRAINT row_changes_source_check CHECK (source IN ('editor', 'undo', 'restore'));
