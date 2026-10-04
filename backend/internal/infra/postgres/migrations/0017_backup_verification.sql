-- =============================================================================
-- Migration 0017 — backup verification.
--
-- A backup can be test-restored into a throwaway database on its instance;
-- the outcome is recorded here so the dashboard can show which backups are
-- known to restore.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE backups ADD COLUMN IF NOT EXISTS verified_at timestamptz;
ALTER TABLE backups ADD COLUMN IF NOT EXISTS verify_status text
  CHECK (verify_status IN ('running','passed','failed'));
ALTER TABLE backups ADD COLUMN IF NOT EXISTS verify_error text;
