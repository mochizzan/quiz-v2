package model

import "time"

// Session is one row of sessions (§6.13): a DB-backed login session owned by
// an account.
type Session struct {
	ID        uint32
	AccountID uint32
	TokenHash []byte
	ExpiresAt time.Time
	IP        []byte
	UserAgent *string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt time.Time
}

// PasswordResetToken is one row of password_reset_tokens (§6.14): a single-use
// token a teacher issued to reset a student's password.
type PasswordResetToken struct {
	ID                uint32
	StudentID         uint32
	IssuedByAccountID uint32
	TokenHash         []byte
	ExpiresAt         time.Time
	UsedAt            *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         time.Time
}
