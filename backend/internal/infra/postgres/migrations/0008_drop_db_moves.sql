-- =============================================================================
-- Migration 0008 — drop the db_moves table. A database move is no longer a
-- grouped saga with its own record: it now just creates a backup and a restore
-- operation, carrying the move target/cutover in the operations' own params, so
-- moves are tracked through the Operations list like any other job.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

DROP TABLE IF EXISTS db_moves;
