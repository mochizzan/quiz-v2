-- 0005_telemetry_schema.up.sql
-- Attempts, per-question answers, selected options, and focus-loss events.
-- Forward-only and idempotent: re-running this file is a no-op.

CREATE TABLE IF NOT EXISTS quiz_attempts (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  registration_id INT UNSIGNED NOT NULL,
  attempt_no TINYINT UNSIGNED NOT NULL,
  submitted_at DATETIME NULL,
  final_score DECIMAL(5,4) NULL,
  shuffle_seed INT UNSIGNED NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_attempt_number (registration_id, attempt_no, deleted_at),
  KEY ix_attempts_registration (registration_id, submitted_at),
  KEY ix_attempts_submitted (submitted_at),
  CONSTRAINT fk_attempt_registration FOREIGN KEY (registration_id) REFERENCES registrations (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT chk_attempt_number CHECK (attempt_no BETWEEN 1 AND 255),
  CONSTRAINT chk_attempt_score CHECK (final_score IS NULL OR final_score BETWEEN 0 AND 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS attempt_answers (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  attempt_id INT UNSIGNED NOT NULL,
  question_id INT UNSIGNED NOT NULL,
  answer_text TEXT NULL,
  score DECIMAL(5,4) NULL,
  feedback VARCHAR(1000) NULL,
  graded_by_account_id INT UNSIGNED NULL,
  graded_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_answers_slot (attempt_id, question_id, deleted_at),
  KEY ix_answers_attempt (attempt_id),
  KEY ix_answers_queue (graded_at, question_id),
  CONSTRAINT fk_answers_attempt FOREIGN KEY (attempt_id) REFERENCES quiz_attempts (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_answers_question FOREIGN KEY (question_id) REFERENCES questions (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_answers_grader FOREIGN KEY (graded_by_account_id) REFERENCES accounts (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT chk_answers_score CHECK (score IS NULL OR score BETWEEN 0 AND 1),
  CONSTRAINT chk_answers_grading CHECK ((score IS NULL) = (graded_at IS NULL))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS attempt_answer_selections (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  attempt_answer_id INT UNSIGNED NOT NULL,
  option_id INT UNSIGNED NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_selection (attempt_answer_id, option_id, deleted_at),
  KEY ix_selection_option (option_id),
  CONSTRAINT fk_selection_answer FOREIGN KEY (attempt_answer_id) REFERENCES attempt_answers (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_selection_option FOREIGN KEY (option_id) REFERENCES answer_options (id) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS focus_loss_events (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  attempt_id INT UNSIGNED NOT NULL,
  kind VARCHAR(20) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  KEY ix_focus_attempt (attempt_id, occurred_at),
  CONSTRAINT fk_focus_attempt FOREIGN KEY (attempt_id) REFERENCES quiz_attempts (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT chk_focus_kind CHECK (kind IN ('blur','hidden','fullscreen_exit'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
