-- =============================================================================
-- Migration 0024 — temporary restore targets are not databases.
--
-- Backup verification restores into fdv_<16 hex> (and backup copies into
-- fds_<16 hex>) on the source database's instance. Discovery used to register
-- such a database as "discovered" while it existed, then report it missing
-- once it was dropped. Those names are now reserved and never registered; any
-- registered before are removed from the inventory here (the databases
-- themselves are untouched).
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

UPDATE databases SET deleted_at = now()
WHERE deleted_at IS NULL AND name ~ '^fd[vs]_[0-9a-f]{16}$' AND labels ? 'discovered';
