-- =============================================================================
-- Migration 0022 — fleet-wide schema catalog.
--
-- A background loop records the tables, views and columns of every database
-- Fleetdock can reach, so that "where is customer_email?" is answered from
-- one table instead of by connecting to every server. One row per table
-- (column_name NULL) plus one per column. A database's rows are replaced
-- whole on each refresh; schema_catalog_state says when that last happened.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

CREATE TABLE IF NOT EXISTS schema_catalog (
  database_id uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  schema_name text NOT NULL DEFAULT '',
  table_name  text NOT NULL,
  kind        text NOT NULL CHECK (kind IN ('table', 'view')),
  column_name text,
  data_type   text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS ix_schema_catalog_db ON schema_catalog (database_id);
CREATE INDEX IF NOT EXISTS ix_schema_catalog_table ON schema_catalog (lower(table_name) text_pattern_ops);
CREATE INDEX IF NOT EXISTS ix_schema_catalog_column ON schema_catalog (lower(column_name) text_pattern_ops)
  WHERE column_name IS NOT NULL;

CREATE TABLE IF NOT EXISTS schema_catalog_state (
  database_id  uuid PRIMARY KEY REFERENCES databases(id) ON DELETE CASCADE,
  refreshed_at timestamptz,
  attempted_at timestamptz NOT NULL DEFAULT now(),
  truncated    boolean NOT NULL DEFAULT false,
  error        text
);
