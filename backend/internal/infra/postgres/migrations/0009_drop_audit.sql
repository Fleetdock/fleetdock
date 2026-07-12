-- Remove the audit log feature (table, trigger, and permission grants).

DROP TRIGGER IF EXISTS trg_audit_immutable ON audit_log;
DROP TABLE IF EXISTS audit_log;

DELETE FROM role_permissions WHERE permission = 'audit:read';
