-- =============================================================================
-- Migration 0018 — SQL console history and saved queries (per user).
-- History keeps the most recent 200 runs per user and database; older rows are
-- pruned on insert.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

CREATE TABLE query_history (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  database_id uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  sql         text NOT NULL,
  statements  integer NOT NULL DEFAULT 1,
  duration_ms bigint NOT NULL DEFAULT 0,
  row_count   bigint NOT NULL DEFAULT 0,
  error       text,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_query_history_user_db ON query_history (user_id, database_id, created_at DESC);

CREATE TABLE saved_queries (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  database_id uuid REFERENCES databases(id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
  sql         text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_saved_queries_user ON saved_queries (user_id, name);
