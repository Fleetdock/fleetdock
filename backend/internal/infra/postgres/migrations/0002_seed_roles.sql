-- =============================================================================
-- Seed system roles and their permissions. Idempotent.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

INSERT INTO roles (name, description, is_system) VALUES
  ('owner',    'Full access to everything',            true),
  ('admin',    'Administrative access',                true),
  ('operator', 'Operate servers, instances, databases', true),
  ('viewer',   'Read-only access',                     true)
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission)
SELECT r.id, p.perm
FROM roles r
JOIN (VALUES
  ('owner', 'server:read'),   ('owner', 'server:write'),
  ('owner', 'instance:read'), ('owner', 'instance:write'),
  ('owner', 'database:read'), ('owner', 'database:write'),
  ('owner', 'user:read'),     ('owner', 'user:write'),
  ('owner', 'token:read'),    ('owner', 'token:write'),

  ('admin', 'server:read'),   ('admin', 'server:write'),
  ('admin', 'instance:read'), ('admin', 'instance:write'),
  ('admin', 'database:read'), ('admin', 'database:write'),
  ('admin', 'user:read'),     ('admin', 'user:write'),
  ('admin', 'token:read'),    ('admin', 'token:write'),

  ('operator', 'server:read'),
  ('operator', 'instance:read'), ('operator', 'instance:write'),
  ('operator', 'database:read'), ('operator', 'database:write'),
  ('operator', 'token:read'),    ('operator', 'token:write'),

  ('viewer', 'server:read'),
  ('viewer', 'instance:read'),
  ('viewer', 'database:read'),
  ('viewer', 'user:read'),
  ('viewer', 'token:read')
) AS p(role_name, perm) ON p.role_name = r.name
ON CONFLICT (role_id, permission) DO NOTHING;
