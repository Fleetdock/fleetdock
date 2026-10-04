-- =============================================================================
-- Migration 0019 — automatic database discovery.
--
-- The per-minute instance probe now keeps each instance's database list in
-- sync on its own, replacing the manual "Import DBs" step:
--   * databases that appear on the server are added;
--   * a database Fleetdock can no longer see is marked 'missing' (never
--     deleted) and becomes 'active' again if it reappears.
-- last_seen_at records the last probe that saw the database; missing_since
-- when it went missing.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE databases DROP CONSTRAINT IF EXISTS databases_status_check;
ALTER TABLE databases ADD CONSTRAINT databases_status_check
  CHECK (status IN ('creating','active','locked','migrating','deleting','error','missing'));

ALTER TABLE databases ADD COLUMN IF NOT EXISTS last_seen_at timestamptz;
ALTER TABLE databases ADD COLUMN IF NOT EXISTS missing_since timestamptz;
