-- =============================================================================
-- Migration 0020 — SSH tunnels for external instances.
--
-- An external instance may be reached through an SSH bastion instead of
-- directly. ssh_host NULL means "no tunnel". The SSH password or private key
-- lives in secrets (kind 'ssh_key'); ssh_host_key pins the bastion's host key
-- (authorized_keys format) on first use and is cleared to re-pin.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE instances
  ADD COLUMN IF NOT EXISTS ssh_host       text,
  ADD COLUMN IF NOT EXISTS ssh_port       int  NOT NULL DEFAULT 22 CHECK (ssh_port BETWEEN 1 AND 65535),
  ADD COLUMN IF NOT EXISTS ssh_user       text,
  ADD COLUMN IF NOT EXISTS ssh_auth       text CHECK (ssh_auth IN ('password','key')),
  ADD COLUMN IF NOT EXISTS ssh_secret_ref text REFERENCES secrets(ref) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS ssh_host_key   text;

ALTER TABLE instances DROP CONSTRAINT IF EXISTS instances_ssh_external;
ALTER TABLE instances ADD CONSTRAINT instances_ssh_external
  CHECK (ssh_host IS NULL OR (kind = 'external' AND ssh_user IS NOT NULL AND ssh_auth IS NOT NULL));
