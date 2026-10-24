package model

import "time"

// Quiz is one row of quizzes (§6.6): a teacher-authored quiz.
type Quiz struct {
	ID                 uint32
	OwnerAccountID     uint32
	UpdatedByAccountID *uint32
	Title              string
	Description        string
	Visibility         string
	State              string
	Code               *string
	InviteToken        string
	TargetGrade        *uint8
	TargetMajorID      *uint32
	StartsAt           *time.Time
	EndsAt             *time.Time
	AllowRetake        bool
	MaxAttempts        uint8
	ShowScore          bool
	ReviewMode         string
	RandomizeQuestions bool
	RandomizeOptions   bool
	PublishedAt        *time.Time
	ClosedAt           *time.Time
	DeletedByAccountID *uint32
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          time.Time
}

// Question is one row of questions (§6.7): one item of a quiz.
type Question struct {
	ID                 uint32
	QuizID             uint32
	Position           uint16
	Type               string
	Prompt             string
	UpdatedByAccountID *uint32
	ImagePath          *string
	ImageMime          *string
	ImageBytes         *uint32
	ImageSHA256        []byte
	ImageAlt           *string
	DeletedByAccountID *uint32
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          time.Time
}

// AnswerOption is one row of answer_options (§6.8): one choice for an
// objective question.
type AnswerOption struct {
	ID         uint32
	QuestionID uint32
	Position   uint16
	Body       string
	IsCorrect  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  time.Time
}
