package authoring

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// quizColumns is the column list every quiz scan agrees on.
const quizColumns = `id, owner_account_id, title, description, visibility, state, code, invite_token,
	target_grade, target_major_id, starts_at, ends_at, allow_retake, max_attempts,
	show_score, review_mode, randomize_questions, randomize_options, created_at`

// quizRow is one quizzes row with nullable columns kept nullable (OPS data
// rules: sql.Null* everywhere a column is nullable).
type quizRow struct {
	ID                 int64
	OwnerID            int64
	Title              string
	Description        string
	Visibility         string
	State              string
	Code               sql.NullString
	InviteToken        string
	TargetGrade        sql.NullInt64
	TargetMajorID      sql.NullInt64
	StartsAt           sql.NullTime
	EndsAt             sql.NullTime
	AllowRetake        bool
	MaxAttempts        int
	ShowScore          bool
	ReviewMode         string
	RandomizeQuestions bool
	RandomizeOptions   bool
	CreatedAt          time.Time
}

func scanQuizRow(row interface{ Scan(...any) error }) (*quizRow, error) {
	var q quizRow
	err := row.Scan(
		&q.ID, &q.OwnerID, &q.Title, &q.Description, &q.Visibility, &q.State,
		&q.Code, &q.InviteToken, &q.TargetGrade, &q.TargetMajorID,
		&q.StartsAt, &q.EndsAt, &q.AllowRetake, &q.MaxAttempts,
		&q.ShowScore, &q.ReviewMode, &q.RandomizeQuestions, &q.RandomizeOptions,
		&q.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// readOwnedQuiz resolves one live quiz owned by the actor. Missing, deleted and
// non-owned are the same 404 (AUT-020).
func readOwnedQuiz(ctx context.Context, q store.Querier, quizID, ownerID int64, lock bool) (*quizRow, error) {
	query := "SELECT " + quizColumns + " FROM quizzes WHERE id = ? AND owner_account_id = ? AND deleted_at = ?"
	if lock {
		query += " FOR UPDATE"
	}
	row, err := scanQuizRow(q.QueryRowContext(ctx, query, quizID, ownerID, store.LiveAt))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFoundf()
	}
	if err != nil {
		return nil, internal(err)
	}
	return row, nil
}

// liveQuestion is one question plus its live options.
type liveQuestion struct {
	ID         int64
	Position   int
	Kind       string
	Prompt     string
	ImagePath  sql.NullString
	ImageMime  sql.NullString
	ImageBytes sql.NullInt64
	ImageSHA   []byte
	ImageAlt   sql.NullString
	Options    []liveOption
}

// liveOption is one live answer option.
type liveOption struct {
	Position int
	Body     string
	Correct  bool
}

// readQuestions loads the live question set and its live options from one
// snapshot. The caller owns the transaction/snapshot choice.
func readQuestions(ctx context.Context, q store.Querier, quizID int64) ([]liveQuestion, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, position, type, prompt, image_path, image_mime, image_bytes, image_sha256, image_alt
		FROM questions WHERE quiz_id = ? AND deleted_at = ? ORDER BY position, id`, quizID, store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	byID := map[int64]int{}
	var out []liveQuestion
	for rows.Next() {
		var item liveQuestion
		if err := rows.Scan(&item.ID, &item.Position, &item.Kind, &item.Prompt,
			&item.ImagePath, &item.ImageMime, &item.ImageBytes, &item.ImageSHA, &item.ImageAlt); err != nil {
			return nil, internal(err)
		}
		byID[item.ID] = len(out)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}

	optRows, err := q.QueryContext(ctx, `SELECT o.question_id, o.position, o.body, o.is_correct
		FROM answer_options o JOIN questions z ON z.id = o.question_id
		WHERE z.quiz_id = ? AND z.deleted_at = ? AND o.deleted_at = ?
		ORDER BY o.question_id, o.position, o.id`, quizID, store.LiveAt, store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer optRows.Close()
	for optRows.Next() {
		var (
			qid  int64
			item liveOption
		)
		if err := optRows.Scan(&qid, &item.Position, &item.Body, &item.Correct); err != nil {
			return nil, internal(err)
		}
		if i, ok := byID[qid]; ok {
			out[i].Options = append(out[i].Options, item)
		}
	}
	if err := optRows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// toPublishQuestions projects the read model into the gate input.
func toPublishQuestions(items []liveQuestion) []PublishQuestion {
	out := make([]PublishQuestion, 0, len(items))
	for _, item := range items {
		options := make([]PublishOption, 0, len(item.Options))
		for _, opt := range item.Options {
			options = append(options, PublishOption{Position: opt.Position, Body: opt.Body, Correct: opt.Correct})
		}
		out = append(out, PublishQuestion{
			ID:         item.ID,
			Position:   item.Position,
			Type:       item.Kind,
			Prompt:     item.Prompt,
			ImagePath:  item.ImagePath,
			ImageMime:  item.ImageMime,
			ImageBytes: item.ImageBytes,
			ImageSHA:   item.ImageSHA,
			Options:    options,
		})
	}
	return out
}

// readMajorNames loads the reference names in the same snapshot as the page.
// Deleted majors are included so an existing quiz still displays its audience.
func readMajorNames(ctx context.Context, q store.Querier) (map[int64]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, name FROM majors")
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var (
			id   int64
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return nil, internal(err)
		}
		out[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// MajorLive reports whether a target major is still live. It is called inside
// the publish/create transaction.
func readMajorLive(ctx context.Context, q store.Querier, majorID int64) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM majors WHERE id = ? AND deleted_at = ?)",
		majorID, store.LiveAt).Scan(&exists)
	if err != nil {
		return false, internal(err)
	}
	return exists, nil
}

// audience renders the visibility/target pair with the shared formatter.
func audience(v string, grade sql.NullInt64, majorID sql.NullInt64, majorName string) string {
	if viewmodel.Visibility(v) == viewmodel.VisibilityPublic {
		return "Semua kelas"
	}
	hasGrade := grade.Valid
	hasMajor := majorID.Valid && majorID.Int64 > 0
	switch {
	case hasGrade && hasMajor:
		return viewmodel.ClassLabel(int(grade.Int64), majorName)
	case hasGrade:
		return viewmodel.RomanGrade(int(grade.Int64))
	case hasMajor:
		if majorName == "" {
			return "Semua tingkat"
		}
		return "Semua tingkat — " + majorName
	default:
		return "Semua kelas"
	}
}

// correctLabel renders the teacher-only key as option letters, never option
// ids (AUT-052).
func correctLabel(q liveQuestion) string {
	if viewmodel.QuestionType(q.Kind) == viewmodel.QuestionEssay {
		return "Penilaian esai"
	}
	letters := make([]string, 0, len(q.Options))
	for i, opt := range q.Options {
		if opt.Correct {
			letters = append(letters, viewmodel.OptionLetter(i))
		}
	}
	return strings.Join(letters, ", ")
}

// localPtr converts a nullable DATETIME into the view model's pointer.
func localPtr(t sql.NullTime) *viewmodel.LocalTime {
	if !t.Valid {
		return nil
	}
	v := viewmodel.NewLocalTime(t.Time)
	return &v
}

// timePtr converts a nullable DATETIME into a *time.Time, for Exec arguments.
func timePtr(t sql.NullTime) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

// stringArg turns a nullable string into a driver argument.
func stringArg(s sql.NullString) any {
	if !s.Valid {
		return nil
	}
	return s.String
}

// codePtr exposes a public quiz code to the owner.
func codePtr(s sql.NullString) *string {
	if !s.Valid || s.String == "" {
		return nil
	}
	v := s.String
	return &v
}
