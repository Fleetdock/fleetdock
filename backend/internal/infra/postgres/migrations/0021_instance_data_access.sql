-- =============================================================================
-- Migration 0021 — choose the login used for data access per instance.
--
-- The table browser, SQL console, exports and imports always connected as
-- Fleetdock-created roles (database_access_roles), made silently on first
-- use. That is now an explicit choice:
--   admin   — the instance's admin login (only instance administrators);
--   login   — a dedicated login the operator configures (data_username +
--             a secret under instance/<id>/data);
--   managed — the per-database roles, as before.
-- New instances default to 'admin'. Instances that already have roles keep
-- 'managed', so existing users do not lose access.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE instances
  ADD COLUMN IF NOT EXISTS data_access     text NOT NULL DEFAULT 'admin'
    CHECK (data_access IN ('admin','login','managed')),
  ADD COLUMN IF NOT EXISTS data_username   text,
  ADD COLUMN IF NOT EXISTS data_secret_ref text REFERENCES secrets(ref) ON DELETE SET NULL;

ALTER TABLE instances DROP CONSTRAINT IF EXISTS instances_data_login;
ALTER TABLE instances ADD CONSTRAINT instances_data_login
  CHECK (data_access <> 'login' OR data_username IS NOT NULL);

UPDATE instances SET data_access = 'managed'
WHERE id IN (SELECT d.instance_id FROM databases d JOIN database_access_roles r ON r.database_id = d.id);
