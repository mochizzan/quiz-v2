package authoring

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/viewmodel"
)

// ErrReorder marks a malformed or stale reorder payload. The command turns it
// into the err-reorder flash, never a 422 (AUT-014).
var ErrReorder = errors.New("authoring: reorder does not match the live question set")

// QuizInput is the raw create/metadata form (AUT-002/AUT-005).
type QuizInput struct {
	Title         string
	Description   string
	StartAt       string
	EndAt         string
	Visibility    string
	TargetGrade   string
	TargetMajorID string
}

// QuizValues is the normalised quiz shell a command may persist.
type QuizValues struct {
	Title         string
	Description   string
	StartsAt      *time.Time
	EndsAt        *time.Time
	Visibility    viewmodel.Visibility
	TargetGrade   int
	TargetMajorID int64
}

// SettingsValues is the normalised six-field settings branch.
type SettingsValues struct {
	RandomizeQuestions bool
	RandomizeAnswers   bool
	AllowRetake        bool
	MaxAttempts        int
	ReviewMode         viewmodel.ReviewMode
	ShowScore          bool
}

// OptionValues is one option after normalisation.
type OptionValues struct {
	Body    string
	Correct bool
}

// QuestionInput is the raw editor submission (AUT-010/AUT-012).
type QuestionInput struct {
	Type       string
	Body       string
	ImageAlt   string
	AltPresent bool
	Options    []string
	// CorrectIndex holds every correct_index value (single choice); the raw
	// slice is used so more than one distinct value can be rejected.
	CorrectIndex []string
	// CorrectIndexes holds every correct_indexes value (multiple choice).
	CorrectIndexes []string
	RemoveImage    bool
	Images         []Upload
}

// QuestionValues is the normalised question a command may persist.
type QuestionValues struct {
	Type    viewmodel.QuestionType
	Body    string
	AltText string
	// Alt is the value to store when a new image is attached; nil means NULL.
	Alt         *string
	Options     []OptionValues
	Image       *imageBlob
	RemoveImage bool
}

// ValidateQuizCreate validates the create form, including the audience pair.
func ValidateQuizCreate(in QuizInput) (QuizValues, *apperr.Error) {
	return validateQuiz(in, true)
}

// ValidateQuizMeta validates the metadata branch; visibility and targets are
// never read or written there (AUT-005, R-11).
func ValidateQuizMeta(in QuizInput) (QuizValues, *apperr.Error) {
	return validateQuiz(in, false)
}

func validateQuiz(in QuizInput, audience bool) (QuizValues, *apperr.Error) {
	fields := map[string]string{}
	out := QuizValues{
		Title:       NormalizeText(in.Title),
		Description: NormalizeText(in.Description),
	}

	switch {
	case out.Title == "":
		fields["title"] = MsgTitleRequired
	case RuneLen(out.Title) > 150:
		fields["title"] = MsgTitleTooLong
	}
	if RuneLen(out.Description) > 1000 {
		fields["description"] = MsgDescriptionTooLong
	}

	start, startPresent, startOK := ParseLocalDateTime(in.StartAt)
	if !startOK {
		fields["start_at"] = MsgStartInvalid
	}
	end, endPresent, endOK := ParseLocalDateTime(in.EndAt)
	if !endOK {
		fields["end_at"] = MsgEndInvalid
	}
	if startPresent && startOK {
		out.StartsAt = &start
	}
	if endPresent && endOK {
		out.EndsAt = &end
	}
	if startOK && endOK && startPresent && endPresent && !end.After(start) {
		fields["end_at"] = MsgEndNotAfterStart
	}

	if audience {
		switch viewmodel.Visibility(strings.TrimSpace(in.Visibility)) {
		case viewmodel.VisibilityPublic:
			out.Visibility = viewmodel.VisibilityPublic
		case viewmodel.VisibilityPrivate:
			out.Visibility = viewmodel.VisibilityPrivate
		default:
			fields["visibility"] = MsgVisibilityInvalid
		}

		if out.Visibility == viewmodel.VisibilityPrivate {
			gradeRaw := strings.TrimSpace(in.TargetGrade)
			switch gradeRaw {
			case "":
				fields["target_grade"] = MsgGradeRequired
			case "10", "11", "12":
				if n, err := strconv.Atoi(gradeRaw); err == nil {
					out.TargetGrade = n
				}
			default:
				fields["target_grade"] = MsgGradeInvalid
			}
			majorRaw := strings.TrimSpace(in.TargetMajorID)
			if majorRaw == "" {
				fields["target_major_id"] = MsgMajorRequired
			} else if n, err := strconv.ParseInt(majorRaw, 10, 64); err != nil || n <= 0 {
				fields["target_major_id"] = MsgMajorUnavailable
			} else {
				out.TargetMajorID = n
			}
		}
	}

	if len(fields) > 0 {
		return QuizValues{}, apperr.Validationf(fields)
	}
	return out, nil
}

// ValidateSettings applies the settings branch rules. A failed checkbox value
// rejects the whole branch; allow_retake=false forces max_attempts=1 and
// ignores the submitted number (AUT-005).
func ValidateSettings(form url.Values) (SettingsValues, *apperr.Error) {
	fields := map[string]string{}
	out := SettingsValues{}

	out.RandomizeQuestions = checkbox(form, "randomize_questions", fields)
	out.RandomizeAnswers = checkbox(form, "randomize_answers", fields)
	out.AllowRetake = checkbox(form, "allow_retake", fields)
	out.ShowScore = checkbox(form, "show_score", fields)

	if out.AllowRetake {
		raw := strings.TrimSpace(form.Get("max_attempts"))
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 10 {
			fields["max_attempts"] = MsgAttemptsRange
		} else {
			out.MaxAttempts = n
		}
	} else {
		out.MaxAttempts = 1
	}

	mode := viewmodel.ReviewMode(strings.TrimSpace(form.Get("review_mode")))
	if mode.Valid() {
		out.ReviewMode = mode
	} else {
		fields["review_mode"] = MsgReviewModeInvalid
	}

	if len(fields) > 0 {
		return SettingsValues{}, apperr.Validationf(fields)
	}
	return out, nil
}

// checkbox reads the AUT checkbox contract: absent/empty is false, exactly "1"
// is true, and every other non-empty value is a field error.
func checkbox(form url.Values, name string, fields map[string]string) bool {
	values, ok := form[name]
	if !ok {
		return false
	}
	present := false
	valid := true
	for _, v := range values {
		if v == "" {
			continue
		}
		present = true
		if v != "1" {
			valid = false
		}
	}
	if !valid {
		fields[name] = MsgSettingsInvalid
		return false
	}
	return present
}

// ValidateQuestion validates and normalises one editor submission. Every field
// is checked before any file is staged (AUT-028).
func ValidateQuestion(in QuestionInput) (QuestionValues, *apperr.Error) {
	fields := map[string]string{}
	out := QuestionValues{
		Body:        NormalizeText(in.Body),
		RemoveImage: in.RemoveImage,
	}

	switch viewmodel.QuestionType(strings.TrimSpace(in.Type)) {
	case viewmodel.QuestionSingle:
		out.Type = viewmodel.QuestionSingle
	case viewmodel.QuestionMultiple:
		out.Type = viewmodel.QuestionMultiple
	case viewmodel.QuestionEssay:
		out.Type = viewmodel.QuestionEssay
	default:
		fields["type"] = MsgQuestionTypeInvalid
	}

	switch n := RuneLen(out.Body); {
	case n == 0:
		fields["body"] = MsgPromptRequired
	case n < 8:
		fields["body"] = MsgPromptTooShort
	case n > 2000:
		fields["body"] = MsgPromptTooLong
	}

	alt := NormalizeText(in.ImageAlt)
	out.AltText = alt
	if RuneLen(alt) > 255 {
		fields["image_alt"] = MsgAltTooLong
	}

	if out.Type.IsObjective() {
		bodies := normalizeOptions(in.Options)
		switch {
		case len(bodies) < 2:
			fields["option_body"] = MsgOptionsMin
		case len(bodies) > MaxOptions:
			fields["option_body"] = MsgOptionsMax
		default:
			for i, body := range bodies {
				letter := viewmodel.OptionLetter(i)
				switch n := RuneLen(body); {
				case n == 0:
					fields[fmt.Sprintf("option_body[%d]", i)] = fmt.Sprintf("Pilihan %s %s", letter, MsgOptionRequired)
				case n > 500:
					fields[fmt.Sprintf("option_body[%d]", i)] = fmt.Sprintf("Pilihan %s %s", letter, MsgOptionTooLong)
				}
			}
			if len(fields) == 0 {
				options := make([]OptionValues, len(bodies))
				for i, body := range bodies {
					options[i] = OptionValues{Body: body}
				}
				switch out.Type {
				case viewmodel.QuestionSingle:
					idx, ok := singleKey(in.CorrectIndex, len(options))
					if !ok {
						fields["correct_index"] = MsgSingleKey
					} else {
						options[idx].Correct = true
						out.Options = options
					}
				case viewmodel.QuestionMultiple:
					idxs, msg := multipleKeys(in.CorrectIndexes, len(options))
					if msg != "" {
						fields["correct_indexes"] = msg
					} else {
						for _, idx := range idxs {
							options[idx].Correct = true
						}
						out.Options = options
					}
				}
			}
		}
	}

	switch len(in.Images) {
	case 0:
	case 1:
		blob, msg := validateUpload(in.Images[0])
		if msg != "" {
			fields["image"] = msg
		} else {
			out.Image = blob
		}
	default:
		fields["image"] = MsgOneImage
	}

	if len(fields) > 0 {
		return QuestionValues{}, apperr.Validationf(fields)
	}
	if out.Image != nil && alt != "" {
		value := alt
		out.Alt = &value
	}
	return out, nil
}

// normalizeOptions trims every row and drops trailing fully blank rows, while
// keeping interior blanks so they fail validation (AUT-027).
func normalizeOptions(raw []string) []string {
	out := make([]string, len(raw))
	for i, v := range raw {
		out[i] = NormalizeText(v)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// singleKey returns the one distinct valid index, or false.
func singleKey(raw []string, count int) (int, bool) {
	seen := map[int]bool{}
	for _, v := range raw {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n >= count {
			return 0, false
		}
		seen[n] = true
	}
	if len(seen) != 1 {
		return 0, false
	}
	for n := range seen {
		return n, true
	}
	return 0, false
}

// multipleKeys deduplicates valid indexes. An out-of-range or non-numeric value
// is a hard failure with its own copy; fewer than two distinct indexes is a
// cardinality failure (AUT-010).
func multipleKeys(raw []string, count int) ([]int, string) {
	seen := map[int]bool{}
	for _, v := range raw {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n >= count {
			return nil, MsgOptionKeyInvalid
		}
		seen[n] = true
	}
	if len(seen) < 2 {
		return nil, MsgMultipleKey
	}
	out := make([]int, 0, len(seen))
	for i := 0; i < count; i++ {
		if seen[i] {
			out = append(out, i)
		}
	}
	return out, ""
}

// ParseReorder parses the comma-separated order payload. Empty tokens,
// non-numeric tokens, non-positive ids, and duplicates are malformed
// (AUT-014). Exact-set membership is checked against the locked live rows.
func ParseReorder(order string) ([]int64, error) {
	if strings.TrimSpace(order) == "" {
		return nil, ErrReorder
	}
	parts := strings.Split(order, ",")
	ids := make([]int64, 0, len(parts))
	seen := map[int64]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, ErrReorder
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil || n <= 0 {
			return nil, ErrReorder
		}
		if seen[n] {
			return nil, ErrReorder
		}
		seen[n] = true
		ids = append(ids, n)
	}
	if len(ids) == 0 {
		return nil, ErrReorder
	}
	return ids, nil
}
