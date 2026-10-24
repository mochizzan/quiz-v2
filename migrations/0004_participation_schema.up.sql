-- 0004 — participation schema: anonymous_participants, then registrations.
-- Spec: docs/superpowers/specs/2026-10-23-schema-and-migrations-design.md §6.4, §6.5.

CREATE TABLE IF NOT EXISTS anonymous_participants (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  display_name VARCHAR(100) NOT NULL,
  grade_level TINYINT UNSIGNED NOT NULL,
  major_id INT UNSIGNED NOT NULL,
  cookie_token_hash BINARY(32) NOT NULL,
  last_seen_at DATETIME NULL,
  deleted_by_account_id INT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_anon_cookie (cookie_token_hash, deleted_at),
  INDEX ix_anon_major (major_id, deleted_at),
  CONSTRAINT fk_anon_major FOREIGN KEY (major_id) REFERENCES majors (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_anon_deleter FOREIGN KEY (deleted_by_account_id) REFERENCES accounts (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT chk_anon_grade CHECK (grade_level BETWEEN 10 AND 12)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS registrations (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  quiz_id INT UNSIGNED NOT NULL,
  student_id INT UNSIGNED NULL,
  anonymous_participant_id INT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_reg_student (quiz_id, student_id, deleted_at),
  UNIQUE KEY uq_reg_anon (quiz_id, anonymous_participant_id, deleted_at),
  INDEX ix_reg_quiz (quiz_id, deleted_at),
  INDEX ix_reg_student (student_id, deleted_at),
  INDEX ix_reg_anon (anonymous_participant_id, deleted_at),
  CONSTRAINT fk_reg_quiz FOREIGN KEY (quiz_id) REFERENCES quizzes (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_reg_student FOREIGN KEY (student_id) REFERENCES students (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_reg_anon FOREIGN KEY (anonymous_participant_id) REFERENCES anonymous_participants (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT chk_reg_participant CHECK ((student_id IS NULL) <> (anonymous_participant_id IS NULL))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
