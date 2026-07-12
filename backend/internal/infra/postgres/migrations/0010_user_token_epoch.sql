-- Session invalidation support: a per-user counter embedded in issued JWTs.
-- Bumping it (on password change / reset) invalidates every outstanding session
-- token for that user without needing server-side session storage.
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_epoch integer NOT NULL DEFAULT 0;
