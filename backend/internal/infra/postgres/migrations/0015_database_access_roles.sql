-- =============================================================================
-- Migration 0015 — least-privilege console roles.
--
-- The SQL console, table browser and CSV export used to run with the
-- instance's admin (root) credentials, so a user allowed into one database
-- could read every other database, the account tables, and server files. They
-- now connect as a Fleetdock-managed role scoped to exactly one database: one
-- read-only, one read-write. Roles are created lazily on first use; this table
-- remembers them. Internal only — never shown as an application credential.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

CREATE TABLE database_access_roles (
  database_id uuid NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
  mode        text NOT NULL CHECK (mode IN ('ro','rw')),
  username    text NOT NULL,
  secret_ref  text NOT NULL REFERENCES secrets(ref) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  applied_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (database_id, mode)
);
