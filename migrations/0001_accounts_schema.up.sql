-- 0001 — accounts schema.
-- Spec: docs/superpowers/specs/2026-10-23-schema-and-migrations-design.md §6.15, §6.2.

CREATE TABLE IF NOT EXISTS schema_migrations (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_migrations_name (name, deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS accounts (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  full_name VARCHAR(100) NOT NULL,
  email VARCHAR(254) CHARACTER SET ascii COLLATE ascii_general_ci NOT NULL,
  password_hash VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  role ENUM('teacher','student') CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  status ENUM('pending','active') CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  approved_by_account_id INT UNSIGNED NULL,
  approved_at DATETIME NULL,
  must_change_password TINYINT(1) UNSIGNED NOT NULL DEFAULT 0,
  last_login_at DATETIME NULL,
  deleted_by_account_id INT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_accounts_email (email, deleted_at),
  INDEX ix_accounts_role (role, status),
  CONSTRAINT fk_accounts_approver FOREIGN KEY (approved_by_account_id) REFERENCES accounts (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_accounts_deleter FOREIGN KEY (deleted_by_account_id) REFERENCES accounts (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT chk_accounts_must_change_password CHECK (must_change_password IN (0,1)),
  CONSTRAINT chk_accounts_student_active CHECK (role = 'teacher' OR status = 'active'),
  CONSTRAINT chk_accounts_teacher_approval CHECK (role = 'student' OR (status = 'active') = (approved_at IS NOT NULL))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
