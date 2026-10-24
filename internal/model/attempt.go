package model

import "time"

// QuizAttempt is one attempt at a quiz by a registration.
type QuizAttempt struct {
	ID             uint32
	RegistrationID uint32
	AttemptNo      uint8
	SubmittedAt    *time.Time
	FinalScore     *float64
	ShuffleSeed    uint32
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      time.Time
}

// AttemptAnswer is one question's answer within an attempt.
type AttemptAnswer struct {
	ID                uint32
	AttemptID         uint32
	QuestionID        uint32
	AnswerText        *string
	Score             *float64
	Feedback          *string
	GradedByAccountID *uint32
	GradedAt          *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         time.Time
}

// AttemptAnswerSelection is one selected option of an objective answer.
type AttemptAnswerSelection struct {
	ID              uint32
	AttemptAnswerID uint32
	OptionID        uint32
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       time.Time
}

// FocusLossEvent records one focus-loss event during an attempt.
type FocusLossEvent struct {
	ID         uint32
	AttemptID  uint32
	Kind       string
	OccurredAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  time.Time
}
