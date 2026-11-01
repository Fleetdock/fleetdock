-- =============================================================================
-- Migration 0028 — the audit log becomes readable.
--
-- audit_events (0011) gains who acted (email kept beside the user id, so the
-- trail survives the user), how (session, API token, CLI), from where (IP,
-- user agent), the outcome and the resource's name at the time.
--
-- The append-only trigger now allows exactly two changes:
--   * ON DELETE SET NULL from users: deleting a user who has events used to
--     fail with "table audit_events is append-only";
--   * deleting expired rows, only in a transaction that set
--     fleetdock.audit_prune = 'on' (the retention job).
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

ALTER TABLE audit_events
  ADD COLUMN IF NOT EXISTS actor_email   text,
  ADD COLUMN IF NOT EXISTS actor_kind    text NOT NULL DEFAULT 'user',
  ADD COLUMN IF NOT EXISTS token_id      uuid,
  ADD COLUMN IF NOT EXISTS ip            inet,
  ADD COLUMN IF NOT EXISTS user_agent    text,
  ADD COLUMN IF NOT EXISTS outcome       text NOT NULL DEFAULT 'success',
  ADD COLUMN IF NOT EXISTS resource_name text;

ALTER TABLE audit_events DROP CONSTRAINT IF EXISTS audit_events_actor_kind_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_actor_kind_check
  CHECK (actor_kind IN ('user', 'api_token', 'system', 'anonymous'));
ALTER TABLE audit_events DROP CONSTRAINT IF EXISTS audit_events_outcome_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_outcome_check
  CHECK (outcome IN ('success', 'denied', 'failure'));

-- Events written before this migration: fill in the email where the user
-- still exists. The trigger is not installed yet, so this UPDATE is allowed.
DROP TRIGGER IF EXISTS audit_events_forbid_mutation ON audit_events;
UPDATE audit_events a SET actor_email = u.email
FROM users u WHERE u.id = a.actor_user_id AND a.actor_email IS NULL;

CREATE INDEX IF NOT EXISTS ix_audit_events_page ON audit_events (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS ix_audit_events_actor ON audit_events (actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_audit_events_action ON audit_events (action, created_at DESC);
-- Superseded by ix_audit_events_page.
DROP INDEX IF EXISTS ix_audit_events_created;

CREATE OR REPLACE FUNCTION audit_events_guard() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF current_setting('fleetdock.audit_prune', true) = 'on' THEN
      RETURN OLD;
    END IF;
  ELSIF NEW.actor_user_id IS NULL AND OLD.actor_user_id IS NOT NULL
    AND (to_jsonb(NEW) - 'actor_user_id') = (to_jsonb(OLD) - 'actor_user_id') THEN
    -- The user was deleted (ON DELETE SET NULL); actor_email keeps who it was.
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'table % is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS audit_events_guard ON audit_events;
CREATE TRIGGER audit_events_guard
  BEFORE UPDATE OR DELETE ON audit_events
  FOR EACH ROW EXECUTE FUNCTION audit_events_guard();
