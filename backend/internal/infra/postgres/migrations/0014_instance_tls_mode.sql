-- =============================================================================
-- Migration 0014 — per-instance TLS mode for control-plane connections.
--
-- Connections to instances used sslmode=prefer (PostgreSQL) and no TLS at all
-- (MySQL/MariaDB). The mode is now explicit per instance, named after libpq's
-- sslmode; 'prefer' keeps existing instances working unchanged.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE instances ADD COLUMN IF NOT EXISTS tls_mode text NOT NULL DEFAULT 'prefer'
  CHECK (tls_mode IN ('disable','prefer','require','verify-full'));
