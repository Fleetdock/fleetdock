-- =============================================================================
-- Migration 0004 — automation & observability: scheduled backups + retention,
-- audit log write path, notification channels + alert rules, metrics history.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

-- ---- Scheduled backups ------------------------------------------------------
-- The scheduler reuses the manual backup path (engine 'mariadb-dump'), so the
-- schedule engine CHECK is widened to match, and schedules gain a concrete
-- destination + owner. `mydumper`/`mariabackup` are kept for future engines.
ALTER TABLE backup_schedules DROP CONSTRAINT backup_schedules_engine_check;
ALTER TABLE backup_schedules ADD CONSTRAINT backup_schedules_engine_check
  CHECK (engine IN ('mydumper','mariabackup','mariadb-dump'));
ALTER TABLE backup_schedules ALTER COLUMN engine SET DEFAULT 'mariadb-dump';
ALTER TABLE backup_schedules
  ADD COLUMN destination_id uuid REFERENCES backup_destinations(id) ON DELETE RESTRICT;
ALTER TABLE backup_schedules
  ADD COLUMN created_by uuid REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX ix_backup_schedules_due ON backup_schedules (next_run_at)
  WHERE enabled;

-- ---- Metrics history --------------------------------------------------------
-- server_health keeps only the latest snapshot (PK on server_id). This table
-- appends one row per heartbeat so the dashboard can chart trends over time.
CREATE TABLE server_health_history (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  server_id          uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  cpu_pct            numeric(5,2),
  mem_used_bytes     bigint,
  mem_total_bytes    bigint,
  disk_used_bytes    bigint,
  disk_total_bytes   bigint,
  active_connections integer,
  collected_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_server_health_history ON server_health_history (server_id, collected_at DESC);

-- ---- New permissions --------------------------------------------------------
-- schedule:*  — backup schedules
-- audit:read  — audit log
-- notification:* — channels + alert rules
INSERT INTO role_permissions (role_id, permission)
SELECT r.id, p.perm
FROM roles r
JOIN (VALUES
  ('owner', 'schedule:read'),     ('owner', 'schedule:write'),
  ('owner', 'audit:read'),
  ('owner', 'notification:read'), ('owner', 'notification:write'),

  ('admin', 'schedule:read'),     ('admin', 'schedule:write'),
  ('admin', 'audit:read'),
  ('admin', 'notification:read'), ('admin', 'notification:write'),

  ('operator', 'schedule:read'),  ('operator', 'schedule:write'),
  ('operator', 'notification:read'),

  ('viewer', 'schedule:read'),
  ('viewer', 'notification:read')
) AS p(role_name, perm) ON p.role_name = r.name
ON CONFLICT (role_id, permission) DO NOTHING;
