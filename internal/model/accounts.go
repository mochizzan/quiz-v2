package model

import "time"

// Account is one row of accounts (§6.2): one human who can log in, and the
// single home of an email address.
type Account struct {
	ID                  uint32
	FullName            string
	Email               string
	PasswordHash        string
	Role                string
	Status              string
	ApprovedByAccountID *uint32
	ApprovedAt          *time.Time
	MustChangePassword  bool
	LastLoginAt         *time.Time
	DeletedByAccountID  *uint32
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           time.Time
}
