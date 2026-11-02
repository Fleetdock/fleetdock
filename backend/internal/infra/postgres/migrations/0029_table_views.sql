-- =============================================================================
-- Migration 0029 — saved table views.
--
-- A named view of a table in the data browser: its filters, search, sort and
-- columns, kept as the dashboard's own JSON. Private to its author, or shared
-- with everyone who can read the database.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

CREATE TABLE IF NOT EXISTS table_views (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  database_id uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  table_name  text NOT NULL CHECK (length(table_name) BETWEEN 1 AND 300),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
  state       jsonb NOT NULL,
  shared      boolean NOT NULL DEFAULT false,
  created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_table_views_table ON table_views (database_id, table_name);
CREATE UNIQUE INDEX IF NOT EXISTS uq_table_views_name ON table_views (database_id, table_name, created_by, lower(name));
