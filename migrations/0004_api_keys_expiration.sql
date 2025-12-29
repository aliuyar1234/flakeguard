BEGIN;

ALTER TABLE api_keys
  ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL;

ALTER TABLE api_keys
  ADD CONSTRAINT api_keys_expires_after_created CHECK (expires_at IS NULL OR expires_at > created_at);

CREATE INDEX IF NOT EXISTS idx_api_keys_project_expires_at ON api_keys(project_id, expires_at);

COMMIT;

