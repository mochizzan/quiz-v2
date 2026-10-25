-- 0009_grading_version.up.sql
-- Optimistic concurrency for essay grading (grading-history rework). Every
-- grade and re-grade increments the counter; an edit form submits the value it
-- rendered and a mismatch is a conflict, never a silent overwrite.
-- Forward-only and idempotent: re-running this file is a no-op.

ALTER TABLE attempt_answers
  ADD COLUMN IF NOT EXISTS grading_version SMALLINT UNSIGNED NOT NULL DEFAULT 0;
