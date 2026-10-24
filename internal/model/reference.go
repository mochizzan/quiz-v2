package model

import "time"

// Major is one row of majors (§6.1), a reference table read constantly and
// written rarely.
type Major struct {
	ID                 uint32
	Name               string
	Code               string
	DeletedByAccountID *uint32
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          time.Time
}

// Student is one row of students (§6.3): the profile of a role = 'student'
// account.
type Student struct {
	ID         uint32
	AccountID  uint32
	GradeLevel uint8
	MajorID    uint32
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  time.Time
}
