-- 0006 — auth tokens schema: sessions, then password_reset_tokens.
-- Spec: docs/superpowers/specs/2026-10-23-schema-and-migrations-design.md §6.13, §6.14.

CREATE TABLE IF NOT EXISTS sessions (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  account_id INT UNSIGNED NOT NULL,
  token_hash BINARY(32) NOT NULL,
  expires_at DATETIME NOT NULL,
  ip VARBINARY(16) NULL,
  user_agent VARCHAR(255) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_sessions_token (token_hash, deleted_at),
  INDEX ix_sessions_account (account_id),
  INDEX ix_sessions_expiry (expires_at),
  CONSTRAINT fk_sessions_account FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS password_reset_tokens (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  student_id INT UNSIGNED NOT NULL,
  issued_by_account_id INT UNSIGNED NOT NULL,
  token_hash BINARY(32) NOT NULL,
  expires_at DATETIME NOT NULL,
  used_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (id),
  UNIQUE KEY uq_reset_token (token_hash, deleted_at),
  INDEX ix_reset_student (student_id, expires_at),
  CONSTRAINT fk_reset_student FOREIGN KEY (student_id) REFERENCES students (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_reset_issuer FOREIGN KEY (issued_by_account_id) REFERENCES accounts (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT chk_reset_window CHECK (expires_at > created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
