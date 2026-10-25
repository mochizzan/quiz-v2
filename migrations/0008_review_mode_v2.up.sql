-- 0008 — review mode v2: replace the four old disclosure values with the four
-- the result screen actually implements.
-- Spec: docs/superpowers/specs/2026-10-25-rework-result-visibility-and-actions.md §1.2.
--
-- Mapping (key_only and questions_only both disclosed more than their label
-- promised, so both fall back to the least-revealing value):
--   questions_and_key -> questions_with_key   (unchanged behaviour)
--   answers_only      -> questions_no_verdict (question + the student's answer)
--   questions_only    -> questions_no_answers (question text and image only)
--   key_only          -> questions_no_answers (question text, but no key)
--   anything else     -> questions_no_answers (fail closed)
--
-- Replay safety. A CHECK constraint cannot be changed in place, so DROP and
-- ADD each rebuild `quizzes`. On the 9p-mounted dev datadir a second rebuild of
-- the same table can fail at InnoDB's internal #sql-backup rename with errno
-- 194, which would make a replay of this file unappliable. The guard below
-- reads information_schema: when the new constraint is already installed the
-- file performs no table rebuild at all, while a crash after DROP (constraint
-- absent) still runs exactly one ADD. DDL is not transactional, so this is what
-- turns a partial apply into a fix-forward rather than a dead end.
--
-- MariaDB 12.3 has no DROP CHECK (that is MySQL 8 syntax); DROP CONSTRAINT is
-- the portable spelling and resolves the CHECK by its constraint name.

SET @review_mode_check_current := (
    SELECT COUNT(*) FROM information_schema.CHECK_CONSTRAINTS
    WHERE CONSTRAINT_SCHEMA = DATABASE()
      AND TABLE_NAME = 'quizzes'
      AND CONSTRAINT_NAME = 'chk_quizzes_review_mode'
      AND CHECK_CLAUSE LIKE '%questions_no_verdict%'
      AND CHECK_CLAUSE LIKE '%questions_no_answers%'
      AND CHECK_CLAUSE LIKE '%correct_only%'
      AND CHECK_CLAUSE LIKE '%questions_with_key%'
      AND CHECK_CLAUSE NOT LIKE '%questions_and_key%'
      AND CHECK_CLAUSE NOT LIKE '%answers_only%'
      AND CHECK_CLAUSE NOT LIKE '%questions_only%'
      AND CHECK_CLAUSE NOT LIKE '%key_only%'
);

-- Drop the old constraint unless the new one is already installed. IF EXISTS
-- covers a half-applied state where it is already gone; the guard above keeps
-- a replay of a completed run from rebuilding the table for nothing.
SET @review_mode_ddl := IF(@review_mode_check_current = 0,
    'ALTER TABLE quizzes DROP CONSTRAINT IF EXISTS chk_quizzes_review_mode',
    'DO 0');
PREPARE review_mode_stmt FROM @review_mode_ddl;
EXECUTE review_mode_stmt;
DEALLOCATE PREPARE review_mode_stmt;

-- The UPDATE only touches values the new set does not already contain, so a
-- replay after a partial apply cannot remap a row that already migrated (the
-- CASE's ELSE would otherwise send correct_only and questions_with_key to
-- questions_no_answers). Unknown legacy values still fail closed.
UPDATE quizzes SET review_mode = CASE review_mode
    WHEN 'questions_and_key' THEN 'questions_with_key'
    WHEN 'answers_only'     THEN 'questions_no_verdict'
    WHEN 'questions_only'   THEN 'questions_no_answers'
    WHEN 'key_only'         THEN 'questions_no_answers'
    ELSE 'questions_no_answers' END
WHERE review_mode NOT IN ('questions_no_verdict','questions_no_answers','correct_only','questions_with_key');

-- Changing a default is metadata-only in MariaDB; ALGORITHM=INPLACE pins that
-- so a replay can never turn this line into a rebuild.
ALTER TABLE quizzes ALTER COLUMN review_mode SET DEFAULT 'questions_no_answers', ALGORITHM=INPLACE;

-- Install the new constraint unless it is already there (same guard as above).
SET @review_mode_ddl := IF(@review_mode_check_current = 0,
    'ALTER TABLE quizzes ADD CONSTRAINT chk_quizzes_review_mode CHECK (review_mode IN (''questions_no_verdict'',''questions_no_answers'',''correct_only'',''questions_with_key''))',
    'DO 0');
PREPARE review_mode_stmt FROM @review_mode_ddl;
EXECUTE review_mode_stmt;
DEALLOCATE PREPARE review_mode_stmt;
