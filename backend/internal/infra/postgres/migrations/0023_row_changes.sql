-- =============================================================================
-- Migration 0023 — row change journal (data browser undo).
--
-- Every insert, update and delete made in the data browser records what the
-- row looked like before and after, so it can be listed and undone for
-- FLEETDOCK_ROW_CHANGE_RETENTION (7 days by default). Images are JSON
-- objects {column: null | "text" | {"b64": "..."}}; an update keeps only the
-- columns it set plus the row key. Entries are pruned by the worker.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

CREATE TABLE IF NOT EXISTS row_changes (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  database_id uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  table_name  text NOT NULL,
  op          text NOT NULL CHECK (op IN ('insert', 'update', 'delete')),
  row_key     jsonb,
  before      jsonb,
  after       jsonb,
  columns     text[] NOT NULL DEFAULT '{}',
  session     jsonb NOT NULL DEFAULT '{}',
  undoable    boolean NOT NULL,
  note        text,
  batch_id    uuid,
  source      text NOT NULL CHECK (source IN ('editor', 'undo')),
  user_id     uuid REFERENCES users(id) ON DELETE SET NULL,
  undo_of     uuid REFERENCES row_changes(id) ON DELETE SET NULL,
  undone_at   timestamptz,
  undone_by   uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_row_changes_table ON row_changes (database_id, table_name, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_row_changes_batch ON row_changes (batch_id) WHERE batch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_row_changes_created ON row_changes (created_at);
