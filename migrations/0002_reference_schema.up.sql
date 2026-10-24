-- 0002 — reference schema: majors, then students.
-- Spec: docs/superpowers/specs/2026-10-23-schema-and-migrations-design.md §6.1, §6.3.

CREATE TABLE IF NOT EXISTS majors (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(50) NOT NULL,
  code VARCHAR(10) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  deleted_by_account_id INT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_majors_name (name, deleted_at),
  UNIQUE KEY uq_majors_code (code, deleted_at),
  CONSTRAINT fk_majors_deleter FOREIGN KEY (deleted_by_account_id) REFERENCES accounts (id) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS students (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  account_id INT UNSIGNED NOT NULL,
  grade_level TINYINT UNSIGNED NOT NULL,
  major_id INT UNSIGNED NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_students_account (account_id, deleted_at),
  INDEX ix_students_major (major_id, deleted_at),
  INDEX ix_students_grade (grade_level),
  CONSTRAINT fk_students_account FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_students_major FOREIGN KEY (major_id) REFERENCES majors (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT chk_students_grade CHECK (grade_level BETWEEN 10 AND 12)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
