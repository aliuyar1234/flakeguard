BEGIN;

-- ORG MEMBERSHIPS: add updated_at for role changes
ALTER TABLE org_memberships
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

DROP TRIGGER IF EXISTS trg_org_memberships_updated_at ON org_memberships;
CREATE TRIGGER trg_org_memberships_updated_at
BEFORE UPDATE ON org_memberships
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ORG INVITES
CREATE TABLE IF NOT EXISTS org_invites (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  email CITEXT NOT NULL,
  role org_role NOT NULL,
  token_hash BYTEA NOT NULL,
  created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ NULL,
  accepted_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
  revoked_at TIMESTAMPTZ NULL,
  revoked_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT org_invites_expires_after_created CHECK (expires_at > created_at),
  CONSTRAINT org_invites_accepted_consistency CHECK (
    (accepted_at IS NULL AND accepted_by_user_id IS NULL) OR
    (accepted_at IS NOT NULL AND accepted_by_user_id IS NOT NULL)
  ),
  CONSTRAINT org_invites_revoked_consistency CHECK (
    (revoked_at IS NULL AND revoked_by_user_id IS NULL) OR
    (revoked_at IS NOT NULL AND revoked_by_user_id IS NOT NULL)
  ),
  CONSTRAINT org_invites_not_both_accepted_revoked CHECK (
    NOT (accepted_at IS NOT NULL AND revoked_at IS NOT NULL)
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_org_invites_token_hash_unique
  ON org_invites(token_hash);

CREATE INDEX IF NOT EXISTS idx_org_invites_org_created
  ON org_invites(org_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_org_invites_org_email_active
  ON org_invites(org_id, email)
  WHERE accepted_at IS NULL AND revoked_at IS NULL;

COMMIT;

