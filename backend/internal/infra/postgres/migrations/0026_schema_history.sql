-- =============================================================================
-- Migration 0026 — schema history and drift.
--
-- A watched database has its structure read periodically; each distinct
-- structure (by hash) becomes a snapshot, so the history shows what changed
-- and when. The structure itself is stored gzip-compressed JSON (NULL when it
-- is too large to keep: only the hash is then recorded).
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

CREATE TABLE IF NOT EXISTS schema_watches (
  database_id     uuid PRIMARY KEY REFERENCES databases(id) ON DELETE CASCADE,
  enabled         boolean NOT NULL DEFAULT true,
  last_checked_at timestamptz,
  last_error      text,
  updated_by      uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_schema_watches_due ON schema_watches (last_checked_at NULLS FIRST) WHERE enabled;

CREATE TABLE IF NOT EXISTS schema_snapshots (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  database_id uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  hash        text NOT NULL,
  schema_gz   bytea,
  summary     jsonb NOT NULL DEFAULT '{}'::jsonb,
  previous_id uuid REFERENCES schema_snapshots(id) ON DELETE SET NULL,
  trigger     text NOT NULL CHECK (trigger IN ('scheduled', 'manual', 'ddl')),
  taken_by    uuid REFERENCES users(id) ON DELETE SET NULL,
  taken_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_schema_snapshots_db ON schema_snapshots (database_id, taken_at DESC);
