-- Ensure flake_events is idempotent per (test_case_id, ci_run_id)
-- and add missing query indexes for flake_stats sorting.

-- Deduplicate any existing rows before enforcing uniqueness.
WITH ranked AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY test_case_id, ci_run_id
      ORDER BY created_at ASC, id ASC
    ) AS rn
  FROM flake_events
)
DELETE FROM flake_events fe
USING ranked r
WHERE fe.id = r.id
  AND r.rn > 1;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    WHERE n.nspname = 'public'
      AND t.relname = 'flake_events'
      AND c.conname = 'flake_events_unique_test_case_run'
  ) THEN
    ALTER TABLE flake_events
      ADD CONSTRAINT flake_events_unique_test_case_run
      UNIQUE (test_case_id, ci_run_id);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_flake_stats_score_last_seen
  ON flake_stats (flake_score DESC, last_seen_at DESC);

