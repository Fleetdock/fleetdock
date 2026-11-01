-- =============================================================================
-- Migration 0027 — fleet queries.
--
-- A fleet run is one SQL script run on many databases, a few at a time, by
-- a background runner. Each target keeps its own status and (bounded)
-- results; the run keeps the totals. A run is visible only to its creator.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

CREATE TABLE IF NOT EXISTS fleet_runs (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  created_by      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  sql             text NOT NULL,
  writes          boolean NOT NULL,
  status          text NOT NULL CHECK (status IN ('running', 'paused', 'canceling', 'succeeded', 'failed', 'canceled')),
  concurrency     integer NOT NULL CHECK (concurrency BETWEEN 1 AND 16),
  stop_on_error   boolean NOT NULL,
  canary          boolean NOT NULL,
  timeout_seconds integer NOT NULL CHECK (timeout_seconds BETWEEN 1 AND 600),
  row_limit       integer NOT NULL CHECK (row_limit BETWEEN 1 AND 1000),
  token_scopes    text[],
  total           integer NOT NULL,
  succeeded       integer NOT NULL DEFAULT 0,
  failed          integer NOT NULL DEFAULT 0,
  skipped         integer NOT NULL DEFAULT 0,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  finished_at     timestamptz
);
CREATE INDEX IF NOT EXISTS ix_fleet_runs_owner ON fleet_runs (created_by, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_fleet_runs_active ON fleet_runs (created_at) WHERE status IN ('running', 'canceling');

CREATE TABLE IF NOT EXISTS fleet_run_targets (
  run_id        uuid NOT NULL REFERENCES fleet_runs(id) ON DELETE CASCADE,
  position      integer NOT NULL,
  database_id   uuid REFERENCES databases(id) ON DELETE SET NULL,
  database_name text NOT NULL,
  instance_name text NOT NULL,
  engine        text NOT NULL,
  status        text NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'skipped', 'canceled', 'unknown')),
  results       jsonb,
  rows_affected bigint,
  error         text,
  claimed_by    text,
  started_at    timestamptz,
  finished_at   timestamptz,
  PRIMARY KEY (run_id, position)
);
CREATE INDEX IF NOT EXISTS ix_fleet_run_targets_running ON fleet_run_targets (started_at) WHERE status = 'running';
