-- FlakeGuard Database Schema
-- Complete schema with 14 tables, 4 enums, indexes, and triggers

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- UUID generation via gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;     -- Case-insensitive email storage

-- Enum Types
CREATE TYPE org_role AS ENUM ('OWNER', 'ADMIN', 'MEMBER', 'VIEWER');
CREATE TYPE api_key_scope AS ENUM ('ingest:write', 'read:project');
CREATE TYPE test_status AS ENUM ('passed', 'failed', 'skipped', 'error');
CREATE TYPE ci_event AS ENUM ('push', 'pull_request', 'workflow_dispatch', 'schedule', 'other');

-- Trigger function for auto-updating updated_at columns
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Table: users
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email CITEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Table: orgs
CREATE TABLE orgs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  created_by_user_id UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT orgs_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$')
);

CREATE TRIGGER orgs_updated_at BEFORE UPDATE ON orgs
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Table: org_memberships
CREATE TABLE org_memberships (
  org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role org_role NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (org_id, user_id)
);

CREATE TRIGGER org_memberships_updated_at BEFORE UPDATE ON org_memberships
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Table: projects
CREATE TABLE projects (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  default_branch TEXT NOT NULL DEFAULT 'main',
  slack_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  slack_webhook_url TEXT NULL,
  created_by_user_id UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (org_id, slug),
  CONSTRAINT projects_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$')
);

CREATE TRIGGER projects_updated_at BEFORE UPDATE ON projects
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Table: api_keys
CREATE TABLE api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  token_hash BYTEA NOT NULL,
  scopes api_key_scope[] NOT NULL,
  revoked_at TIMESTAMPTZ NULL,
  last_used_at TIMESTAMPTZ NULL,
  created_by_user_id UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, name)
);

CREATE TRIGGER api_keys_updated_at BEFORE UPDATE ON api_keys
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX idx_api_keys_token_hash ON api_keys(token_hash);

-- Table: ingestions
CREATE TABLE ingestions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  api_key_id UUID NOT NULL REFERENCES api_keys(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Table: ci_runs
CREATE TABLE ci_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  repo_full_name TEXT NOT NULL,
  github_run_id BIGINT NOT NULL,
  github_run_number BIGINT NOT NULL,
  workflow_ref TEXT NOT NULL,
  run_url TEXT NOT NULL,
  pr_number BIGINT NULL,
  event ci_event NOT NULL,
  branch TEXT NOT NULL,
  commit_sha TEXT NOT NULL,
  workflow_name TEXT NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, repo_full_name, github_run_id)
);

CREATE INDEX idx_ci_runs_project_repo ON ci_runs(project_id, repo_full_name);
CREATE INDEX idx_ci_runs_project_last_seen ON ci_runs(project_id, last_seen_at DESC);

-- Table: ci_run_attempts
CREATE TABLE ci_run_attempts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ci_run_id UUID NOT NULL REFERENCES ci_runs(id) ON DELETE CASCADE,
  attempt_number INT NOT NULL,
  started_at TIMESTAMPTZ NULL,
  completed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (ci_run_id, attempt_number),
  CONSTRAINT ci_run_attempts_attempt_positive CHECK (attempt_number >= 1)
);

-- Table: ci_jobs
CREATE TABLE ci_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ci_run_attempt_id UUID NOT NULL REFERENCES ci_run_attempts(id) ON DELETE CASCADE,
  job_name TEXT NOT NULL,
  job_variant TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (ci_run_attempt_id, job_name, job_variant)
);

-- Table: junit_files
CREATE TABLE junit_files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ingestion_id UUID NOT NULL REFERENCES ingestions(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  size_bytes INT NOT NULL,
  content_truncated BOOLEAN NOT NULL DEFAULT FALSE,
  content BYTEA NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Table: test_cases
CREATE TABLE test_cases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  repo_full_name TEXT NOT NULL,
  job_name TEXT NOT NULL,
  job_variant TEXT NOT NULL DEFAULT '',
  test_identifier TEXT NOT NULL,
  classname TEXT NOT NULL,
  name TEXT NOT NULL,
  file TEXT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, repo_full_name, job_name, job_variant, test_identifier)
);

CREATE INDEX idx_test_cases_project_last_seen ON test_cases(project_id, last_seen_at DESC);

-- Table: test_results
CREATE TABLE test_results (
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

-- Table: flake_events
CREATE TABLE flake_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  test_case_id UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
  ci_run_id UUID NOT NULL REFERENCES ci_runs(id) ON DELETE CASCADE,
  failed_attempt_number INT NOT NULL,
  passed_attempt_number INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT flake_attempt_order CHECK (passed_attempt_number > failed_attempt_number)
);

CREATE INDEX idx_flake_events_test_case_created ON flake_events(test_case_id, created_at DESC);

-- Table: flake_stats
CREATE TABLE flake_stats (
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

CREATE TRIGGER flake_stats_updated_at BEFORE UPDATE ON flake_stats
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Table: audit_log
CREATE TABLE audit_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NULL REFERENCES orgs(id) ON DELETE CASCADE,
  project_id UUID NULL REFERENCES projects(id) ON DELETE CASCADE,
  actor_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_log_org_created ON audit_log(org_id, created_at DESC);
CREATE INDEX idx_audit_log_project_created ON audit_log(project_id, created_at DESC);
