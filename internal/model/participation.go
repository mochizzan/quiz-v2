package model

import "time"

// AnonymousParticipant is one row of anonymous_participants (§6.4): one
// identity per person, not per quiz.
type AnonymousParticipant struct {
	ID                 uint32
	DisplayName        string
	GradeLevel         uint8
	MajorID            uint32
	CookieTokenHash    []byte
	LastSeenAt         *time.Time
	DeletedByAccountID *uint32
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          time.Time
}

// Registration is one row of registrations (§6.5): one participant's entry
// into one quiz, where the participant is exactly one of a student profile or
// an anonymous identity.
type Registration struct {
	ID                     uint32
	QuizID                 uint32
	StudentID              *uint32
	AnonymousParticipantID *uint32
	CreatedAt              time.Time
	UpdatedAt              time.Time
	DeletedAt              time.Time
}
