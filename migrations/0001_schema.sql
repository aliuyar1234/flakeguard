BEGIN;

-- Extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- Enums
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'org_role') THEN
    CREATE TYPE org_role AS ENUM ('OWNER','ADMIN','MEMBER','VIEWER');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'api_key_scope') THEN
    CREATE TYPE api_key_scope AS ENUM ('ingest:write','read:project');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'test_status') THEN
    CREATE TYPE test_status AS ENUM ('passed','failed','skipped','error');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ci_event') THEN
    CREATE TYPE ci_event AS ENUM ('push','pull_request','workflow_dispatch','schedule','other');
  END IF;
END $$;

-- Helper function for updated_at
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- USERS
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email CITEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ORGANIZATIONS
CREATE TABLE IF NOT EXISTS orgs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT orgs_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$')
);

DROP TRIGGER IF EXISTS trg_orgs_updated_at ON orgs;
CREATE TRIGGER trg_orgs_updated_at
BEFORE UPDATE ON orgs
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- MEMBERSHIPS
CREATE TABLE IF NOT EXISTS org_memberships (
  org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role org_role NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (org_id, user_id)
);

-- PROJECTS
CREATE TABLE IF NOT EXISTS projects (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  default_branch TEXT NOT NULL DEFAULT 'main',
  slack_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  slack_webhook_url TEXT NULL,
  created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (org_id, slug),
  CONSTRAINT projects_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$')
);

DROP TRIGGER IF EXISTS trg_projects_updated_at ON projects;
CREATE TRIGGER trg_projects_updated_at
BEFORE UPDATE ON projects
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- API KEYS (store only hash)
CREATE TABLE IF NOT EXISTS api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  token_hash BYTEA NOT NULL,
  scopes api_key_scope[] NOT NULL,
  revoked_at TIMESTAMPTZ NULL,
  last_used_at TIMESTAMPTZ NULL,
  created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, name)
);

DROP TRIGGER IF EXISTS trg_api_keys_updated_at ON api_keys;
CREATE TRIGGER trg_api_keys_updated_at
BEFORE UPDATE ON api_keys
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS idx_api_keys_project_id ON api_keys(project_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_token_hash ON api_keys(token_hash);

-- INGESTIONS
CREATE TABLE IF NOT EXISTS ingestions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
  received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  meta JSONB NOT NULL,
  junit_files_count INT NOT NULL DEFAULT 0,
  test_results_count INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_ingestions_project_received ON ingestions(project_id, received_at DESC);

-- CI RUNS (per github_run_id)
CREATE TABLE IF NOT EXISTS ci_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  repo_full_name TEXT NOT NULL,
  workflow_name TEXT NOT NULL,
  workflow_ref TEXT NOT NULL,
  github_run_id BIGINT NOT NULL,
  github_run_number BIGINT NOT NULL,
  run_url TEXT NOT NULL,
  sha TEXT NOT NULL,
  branch TEXT NOT NULL,
  event ci_event NOT NULL,
  pr_number BIGINT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, repo_full_name, github_run_id)
);

CREATE INDEX IF NOT EXISTS idx_ci_runs_project_repo ON ci_runs(project_id, repo_full_name);
CREATE INDEX IF NOT EXISTS idx_ci_runs_project_last_seen ON ci_runs(project_id, last_seen_at DESC);

-- CI ATTEMPTS
CREATE TABLE IF NOT EXISTS ci_run_attempts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ci_run_id UUID NOT NULL REFERENCES ci_runs(id) ON DELETE CASCADE,
  attempt_number INT NOT NULL,
  started_at TIMESTAMPTZ NULL,
  completed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (ci_run_id, attempt_number),
  CONSTRAINT attempt_number_positive CHECK (attempt_number >= 1)
);

CREATE INDEX IF NOT EXISTS idx_ci_attempts_run ON ci_run_attempts(ci_run_id, attempt_number);

-- CI JOBS (one per attempt + job_name + job_variant)
CREATE TABLE IF NOT EXISTS ci_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ci_run_attempt_id UUID NOT NULL REFERENCES ci_run_attempts(id) ON DELETE CASCADE,
  job_name TEXT NOT NULL,
  job_variant TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (ci_run_attempt_id, job_name, job_variant)
);

CREATE INDEX IF NOT EXISTS idx_ci_jobs_attempt ON ci_jobs(ci_run_attempt_id);

-- JUNIT FILES (optional raw content, truncated)
CREATE TABLE IF NOT EXISTS junit_files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ingestion_id UUID NOT NULL REFERENCES ingestions(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  size_bytes INT NOT NULL,
  content_truncated BOOLEAN NOT NULL DEFAULT FALSE,
  content BYTEA NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_junit_files_ingestion ON junit_files(ingestion_id);

-- TEST CASES (canonical identity per project+repo+job+variant+identifier)
CREATE TABLE IF NOT EXISTS test_cases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  repo_full_name TEXT NOT NULL,
  job_name TEXT NOT NULL,
  job_variant TEXT NOT NULL DEFAULT '',
  test_identifier TEXT NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, repo_full_name, job_name, job_variant, test_identifier)
);

CREATE INDEX IF NOT EXISTS idx_test_cases_project_last_seen ON test_cases(project_id, last_seen_at DESC);

-- TEST RESULTS (per job execution)
CREATE TABLE IF NOT EXISTS test_results (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  test_case_id UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
  ci_job_id UUID NOT NULL REFERENCES ci_jobs(id) ON DELETE CASCADE,
  status test_status NOT NULL,
  duration_ms INT NULL,
  failure_message TEXT NULL,
  failure_output TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (test_case_id, ci_job_id)
);

CREATE INDEX IF NOT EXISTS idx_test_results_test_case ON test_results(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_results_ci_job ON test_results(ci_job_id);

-- FLAKE EVENTS (evidence of flake: fail->pass across attempts for same run_id)
CREATE TABLE IF NOT EXISTS flake_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  test_case_id UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
  ci_run_id UUID NOT NULL REFERENCES ci_runs(id) ON DELETE CASCADE,
  failed_attempt_number INT NOT NULL,
  passed_attempt_number INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT flake_attempt_order CHECK (passed_attempt_number > failed_attempt_number)
);

CREATE INDEX IF NOT EXISTS idx_flake_events_test_case_created ON flake_events(test_case_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_flake_events_run ON flake_events(ci_run_id);

-- FLAKE STATS (aggregated)
CREATE TABLE IF NOT EXISTS flake_stats (
  test_case_id UUID PRIMARY KEY REFERENCES test_cases(id) ON DELETE CASCADE,
  mixed_outcome_runs INT NOT NULL DEFAULT 0,
  total_runs_seen INT NOT NULL DEFAULT 0,
  flake_score DOUBLE PRECISION NOT NULL DEFAULT 0,
  last_failure_message TEXT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT flake_counts_nonnegative CHECK (mixed_outcome_runs >= 0 AND total_runs_seen >= 0),
  CONSTRAINT flake_score_range CHECK (flake_score >= 0 AND flake_score <= 1)
);

DROP TRIGGER IF EXISTS trg_flake_stats_updated_at ON flake_stats;
CREATE TRIGGER trg_flake_stats_updated_at
BEFORE UPDATE ON flake_stats
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- AUDIT LOG
CREATE TABLE IF NOT EXISTS audit_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NULL REFERENCES orgs(id) ON DELETE CASCADE,
  project_id UUID NULL REFERENCES projects(id) ON DELETE CASCADE,
  actor_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_org_created ON audit_log(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_project_created ON audit_log(project_id, created_at DESC);

COMMIT;
