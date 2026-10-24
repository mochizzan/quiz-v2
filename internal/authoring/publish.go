package authoring

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"quiz-v2/internal/viewmodel"
)

// PublishQuiz is the quiz-shell slice the gate needs.
type PublishQuiz struct {
	Visibility         string
	Code               sql.NullString
	InviteToken        string
	TargetGrade        sql.NullInt64
	TargetMajorID      sql.NullInt64
	MajorLive          bool
	AllowRetake        bool
	MaxAttempts        int
	ShowScore          bool
	ReviewMode         string
	RandomizeQuestions bool
	RandomizeOptions   bool
}

// PublishOption is one live option as the gate sees it.
type PublishOption struct {
	Position int
	Body     string
	Correct  bool
}

// PublishQuestion is one live question as the gate sees it.
type PublishQuestion struct {
	ID         int64
	Position   int
	Type       string
	Prompt     string
	ImagePath  sql.NullString
	ImageMime  sql.NullString
	ImageBytes sql.NullInt64
	ImageSHA   []byte
	Options    []PublishOption
}

// PublishInput is one consistent snapshot for the gate.
type PublishInput struct {
	Quiz       PublishQuiz
	Questions  []PublishQuestion
	UploadsDir string
}

// PublishResult is the gate outcome: Reason is the first user-fixable failure
// key, Err is an integrity fault that must render 500.
type PublishResult struct {
	OK     bool
	Reason string
	Err    error
}

// EvaluatePublish is the single predicate shared by the detail read model and
// the publish command. The priority is fixed by AUT-027.
func EvaluatePublish(in PublishInput) PublishResult {
	if len(in.Questions) == 0 {
		return PublishResult{Reason: FlashErrNoQuestions}
	}

	q := in.Quiz
	if q.Visibility == string(viewmodel.VisibilityPrivate) {
		if !q.TargetGrade.Valid || q.TargetGrade.Int64 < 10 || q.TargetGrade.Int64 > 12 ||
			!q.TargetMajorID.Valid || q.TargetMajorID.Int64 <= 0 {
			return PublishResult{Reason: FlashErrTargets}
		}
		if !q.MajorLive {
			return PublishResult{Reason: FlashErrTargetMissing}
		}
	}

	if !structurallyValid(in.Questions) {
		return PublishResult{Reason: FlashErrInvalidQuestion}
	}
	if reason, err := checkImages(in); err != nil {
		return PublishResult{Err: err}
	} else if reason != "" {
		return PublishResult{Reason: reason}
	}
	if reason, err := checkSettings(q); err != nil {
		return PublishResult{Err: err}
	} else if reason != "" {
		return PublishResult{Reason: reason}
	}
	return PublishResult{OK: true}
}

// structurallyValid enforces AUT-026 and the dense-position invariant.
func structurallyValid(questions []PublishQuestion) bool {
	ordered := append([]PublishQuestion(nil), questions...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Position < ordered[j].Position })
	for i := range ordered {
		if ordered[i].Position != i+1 {
			return false
		}
	}

	for _, q := range ordered {
		if !validPrompt(q.Prompt) {
			return false
		}
		typ := viewmodel.QuestionType(q.Type)
		options := append([]PublishOption(nil), q.Options...)
		sort.SliceStable(options, func(i, j int) bool { return options[i].Position < options[j].Position })
		for i := range options {
			if options[i].Position != i+1 {
				return false
			}
			if !validOption(options[i].Body) {
				return false
			}
		}
		switch typ {
		case viewmodel.QuestionEssay:
			if len(options) != 0 {
				return false
			}
		case viewmodel.QuestionSingle, viewmodel.QuestionMultiple:
			if len(options) < 2 || len(options) > MaxOptions {
				return false
			}
			correct := 0
			for _, opt := range options {
				if opt.Correct {
					correct++
				}
			}
			if typ == viewmodel.QuestionSingle && correct != 1 {
				return false
			}
			if typ == viewmodel.QuestionMultiple && correct < 2 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validPrompt(s string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(s))
	return n >= 8 && n <= 2000
}

func validOption(s string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(s))
	return n >= 1 && n <= 500
}

// checkImages requires a complete all-or-none image row and a present file
// whose size and extension match the stored metadata.
func checkImages(in PublishInput) (string, error) {
	for _, q := range in.Questions {
		hasPath := q.ImagePath.Valid && q.ImagePath.String != ""
		hasMime := q.ImageMime.Valid && q.ImageMime.String != ""
		hasBytes := q.ImageBytes.Valid
		hasSHA := len(q.ImageSHA) == 32

		if !hasPath {
			if hasMime || hasBytes || len(q.ImageSHA) != 0 {
				return "", fmt.Errorf("authoring: question %d has a partial image row", q.ID)
			}
			continue
		}
		if !hasMime || !hasBytes || !hasSHA || q.ImageBytes.Int64 < 0 {
			return "", fmt.Errorf("authoring: question %d has a partial image row", q.ID)
		}
		if _, ok := extensionForMIME(q.ImageMime.String); !ok {
			return "", fmt.Errorf("authoring: question %d has an unsupported image MIME", q.ID)
		}
		reason, err := checkImageFile(in.UploadsDir, q.ImagePath.String, q.ImageMime.String, q.ImageBytes.Int64)
		if err != nil {
			return "", fmt.Errorf("authoring: question %d image: %w", q.ID, err)
		}
		if reason != "" {
			return reason, nil
		}
	}
	return "", nil
}

// checkSettings validates the six settings, the audience XOR, and the public
// capability values. A missing public code or token is an integrity fault, not
// a user-fixable reason (AUT-027).
func checkSettings(q PublishQuiz) (string, error) {
	visibility := viewmodel.Visibility(q.Visibility)
	switch visibility {
	case viewmodel.VisibilityPublic, viewmodel.VisibilityPrivate:
	default:
		return FlashErrInvalidSettings, nil
	}

	if q.MaxAttempts < 1 || q.MaxAttempts > 10 {
		return FlashErrInvalidSettings, nil
	}
	if !q.AllowRetake && q.MaxAttempts != 1 {
		return FlashErrInvalidSettings, nil
	}
	switch viewmodel.ReviewMode(q.ReviewMode) {
	case viewmodel.ReviewQuestionsAndKey, viewmodel.ReviewKeyOnly,
		viewmodel.ReviewQuestionsOnly, viewmodel.ReviewAnswersOnly:
	default:
		return FlashErrInvalidSettings, nil
	}

	switch visibility {
	case viewmodel.VisibilityPublic:
		if q.TargetGrade.Valid || q.TargetMajorID.Valid {
			return FlashErrInvalidSettings, nil
		}
		if !q.Code.Valid || !sixDigits(q.Code.String) {
			return "", fmt.Errorf("authoring: public quiz is missing its join code")
		}
	case viewmodel.VisibilityPrivate:
		if !q.TargetGrade.Valid || !q.TargetMajorID.Valid {
			return FlashErrTargets, nil
		}
	}
	if strings.TrimSpace(q.InviteToken) == "" {
		return "", fmt.Errorf("authoring: quiz is missing its invite token")
	}
	return "", nil
}

func sixDigits(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
