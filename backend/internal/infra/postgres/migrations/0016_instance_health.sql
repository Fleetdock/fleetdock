-- =============================================================================
-- Migration 0016 — instance health from the periodic probe.
--
-- The worker now probes instances every minute (ping + per-database size and
-- connection counts) instead of relying on manual "Test connection" runs. The
-- latest result lives on the instance; sizes and connection counts land in
-- the existing databases.size_bytes / active_connections columns, which were
-- never populated before.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE instances ADD COLUMN IF NOT EXISTS health jsonb;
