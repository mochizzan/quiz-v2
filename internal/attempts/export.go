package attempts

// export.go is the read model behind the owner-only XLSX export
// (docs/superpowers/specs/2026-10-25-quiz-xlsx-export-design.md). It loads one
// snapshot of a non-draft quiz: metadata, every live question and option, every
// live registration, and every submitted attempt with its per-question response
// and score. The workbook builder in internal/export consumes these types; it
// never queries the database.
//
// The read model deliberately does not load shuffle_seed, randomize_options, or
// answer feedback: responses are canonical to the live option order, and
// feedback is out of scope.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// ExportInput is GET /teacher/quizzes/:id/export.xlsx material.
type ExportInput struct {
	QuizID    int64
	AccountID int64
}

// ExportData is one consistent snapshot of an owned quiz.
type ExportData struct {
	Quiz         ExportQuiz
	Questions    []ExportQuestion
	Participants []ExportParticipant
	Attempts     []ExportAttempt
}

// ExportQuiz is the metadata block on Ringkasan.
type ExportQuiz struct {
	ID          int64
	Title       string
	OwnerName   string
	State       viewmodel.QuizState
	CreatedAt   time.Time
	PublishedAt *time.Time
	ClosedAt    *time.Time
	MaxAttempts int
}

// ExportQuestion is one live question with its canonical options.
type ExportQuestion struct {
	ID         int64
	Position   int
	Type       viewmodel.QuestionType
	Prompt     string
	ImagePath  string
	ImageMIME  string
	ImageAlt   string
	ImageBytes int64
	Options    []ExportOption
}

// ExportOption is one live option in canonical position order.
type ExportOption struct {
	ID       int64
	Position int
	Body     string
	Correct  bool
}

// ExportParticipant is one live registration, student or anonymous.
type ExportParticipant struct {
	RegistrationID int64
	Name           string
	Anonymous      bool
	Grade          int
	Major          string
	CreatedAt      time.Time
}

// ExportAttempt is one submitted attempt, never collapsed with another.
type ExportAttempt struct {
	ID             int64
	RegistrationID int64
	Number         int
	SubmittedAt    time.Time
	// FinalUnits is the stored final score in 0–10000 units, or nil while the
	// attempt is waiting for essay grading.
	FinalUnits *int
	// Answers is one entry per live question, in the same order as
	// ExportData.Questions. Text is the readable canonical response; a blank
	// or unanswered response is "—".
	Answers []ExportAnswer
}

// ExportAnswer is one attempt's response and score for one live question.
type ExportAnswer struct {
	QuestionID int64
	Text       string
	// ScoreUnits is the stored per-question score in 0–10000 units, or nil
	// when the question is not graded yet.
	ScoreUnits *int
}

// exportNoAnswer is the readable placeholder for an unanswered response.
const exportNoAnswer = "—"

// Export implements the export read model. A missing, foreign, or draft quiz is
// ErrNotFound; the caller maps it to a 404. Every list is loaded in one read-only
// repeatable-read transaction.
func (s *Service) Export(ctx context.Context, in ExportInput) (ExportData, error) {
	var out ExportData
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		quiz, err := loadOwnedQuiz(ctx, tx, in.QuizID, in.AccountID)
		if err != nil {
			return err
		}
		if quiz.State == viewmodel.QuizDraft {
			// A draft has no results page and no export (design §4.2).
			return ErrNotFound
		}

		meta, err := loadExportMeta(ctx, tx, quiz)
		if err != nil {
			return err
		}
		questions, err := loadExportQuestions(ctx, tx, quiz.ID)
		if err != nil {
			return err
		}
		participants, err := loadExportParticipants(ctx, tx, quiz.ID)
		if err != nil {
			return err
		}
		attempts, err := loadExportAttempts(ctx, tx, quiz.ID)
		if err != nil {
			return err
		}
		if err := loadExportAnswers(ctx, tx, quiz.ID, questions, attempts); err != nil {
			return err
		}

		out = ExportData{
			Quiz:         meta,
			Questions:    questions,
			Participants: participants,
			Attempts:     attempts,
		}
		return nil
	})
	if err != nil {
		return ExportData{}, err
	}
	return out, nil
}

// loadExportMeta loads the owner and lifecycle columns the owned-quiz lookup
// does not carry. The owner account is live; a missing one is an integrity
// violation, never a second ownership answer.
func loadExportMeta(ctx context.Context, tx *sql.Tx, quiz ownedQuiz) (ExportQuiz, error) {
	var (
		owner     string
		createdAt time.Time
		published sql.NullTime
		closed    sql.NullTime
	)
	err := tx.QueryRowContext(ctx, `
		SELECT a.full_name, z.created_at, z.published_at, z.closed_at
		FROM quizzes z
		JOIN accounts a ON a.id = z.owner_account_id AND a.deleted_at = ?
		WHERE z.id = ? AND z.deleted_at = ?`,
		store.LiveAt, quiz.ID, store.LiveAt,
	).Scan(&owner, &createdAt, &published, &closed)
	if errors.Is(err, sql.ErrNoRows) {
		return ExportQuiz{}, integrity("quiz %d has no live owner account", quiz.ID)
	}
	if err != nil {
		return ExportQuiz{}, internal(err)
	}

	out := ExportQuiz{
		ID:          quiz.ID,
		Title:       quiz.Title,
		OwnerName:   owner,
		State:       quiz.State,
		CreatedAt:   createdAt,
		MaxAttempts: EffectiveMaxAttempts(quiz.AllowRetake, quiz.MaxAttempts),
	}
	if published.Valid {
		at := published.Time
		out.PublishedAt = &at
	}
	if closed.Valid {
		at := closed.Time
		out.ClosedAt = &at
	}
	return out, nil
}

// loadExportQuestions loads every live question of the quiz and attaches its
// live options in canonical position order. It never reads randomize_options:
// the workbook is always the stored order.
func loadExportQuestions(ctx context.Context, tx *sql.Tx, quizID int64) ([]ExportQuestion, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, position, type, prompt,
		       image_path, image_mime, image_alt, image_bytes
		FROM questions
		WHERE quiz_id = ? AND deleted_at = ?
		ORDER BY position, id`,
		quizID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	questions := make([]ExportQuestion, 0)
	for rows.Next() {
		var (
			q        ExportQuestion
			path     sql.NullString
			mime     sql.NullString
			alt      sql.NullString
			imgBytes sql.NullInt64
		)
		if err := rows.Scan(&q.ID, &q.Position, &q.Type, &q.Prompt,
			&path, &mime, &alt, &imgBytes); err != nil {
			return nil, internal(err)
		}
		q.ImagePath = path.String
		q.ImageMIME = mime.String
		q.ImageAlt = alt.String
		q.ImageBytes = imgBytes.Int64
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	if len(questions) == 0 {
		return questions, nil
	}

	rows, err = tx.QueryContext(ctx, `
		SELECT o.id, o.question_id, o.position, o.body, o.is_correct
		FROM answer_options o
		JOIN questions q ON q.id = o.question_id AND q.deleted_at = ?
		WHERE q.quiz_id = ? AND o.deleted_at = ?
		ORDER BY o.question_id, o.position, o.id`,
		store.LiveAt, quizID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	index := make(map[int64]int, len(questions))
	for i := range questions {
		index[questions[i].ID] = i
	}
	for rows.Next() {
		var (
			option     ExportOption
			questionID int64
		)
		if err := rows.Scan(&option.ID, &questionID, &option.Position,
			&option.Body, &option.Correct); err != nil {
			return nil, internal(err)
		}
		if i, ok := index[questionID]; ok {
			questions[i].Options = append(questions[i].Options, option)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return questions, nil
}

// loadExportParticipants loads every live registration, student or anonymous,
// in name order. The name is the readable identity; a null one is an integrity
// error rather than a blank row.
func loadExportParticipants(ctx context.Context, tx *sql.Tx, quizID int64) ([]ExportParticipant, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT r.id, COALESCE(a.full_name, ap.display_name),
		       r.anonymous_participant_id IS NOT NULL,
		       COALESCE(COALESCE(s.grade_level, ap.grade_level), 0),
		       mj.name, r.created_at
		FROM registrations r
		LEFT JOIN students s ON s.id = r.student_id AND s.deleted_at = ?
		LEFT JOIN accounts a ON a.id = s.account_id AND a.deleted_at = ?
		LEFT JOIN anonymous_participants ap ON ap.id = r.anonymous_participant_id AND ap.deleted_at = ?
		LEFT JOIN majors mj ON mj.id = COALESCE(s.major_id, ap.major_id) AND mj.deleted_at = ?
		WHERE r.quiz_id = ? AND r.deleted_at = ?
		ORDER BY LOWER(COALESCE(a.full_name, ap.display_name)), r.id`,
		store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, quizID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	participants := make([]ExportParticipant, 0)
	for rows.Next() {
		var (
			p     ExportParticipant
			name  sql.NullString
			major sql.NullString
		)
		if err := rows.Scan(&p.RegistrationID, &name, &p.Anonymous, &p.Grade,
			&major, &p.CreatedAt); err != nil {
			return nil, internal(err)
		}
		if !name.Valid {
			return nil, integrity("registration %d has no readable name (ATT-282)", p.RegistrationID)
		}
		p.Name = name.String
		p.Major = major.String
		participants = append(participants, p)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return participants, nil
}

// loadExportAttempts loads every submitted attempt of the quiz, ordered by
// participant name, registration, attempt number, and id. It keeps one row per
// attempt; in-progress attempts are excluded by the query.
func loadExportAttempts(ctx context.Context, tx *sql.Tx, quizID int64) ([]ExportAttempt, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT att.id, att.registration_id, att.attempt_no,
		       att.submitted_at, att.final_score
		FROM quiz_attempts att
		JOIN registrations r ON r.id = att.registration_id AND r.deleted_at = ?
		LEFT JOIN students s ON s.id = r.student_id AND s.deleted_at = ?
		LEFT JOIN accounts a ON a.id = s.account_id AND a.deleted_at = ?
		LEFT JOIN anonymous_participants ap ON ap.id = r.anonymous_participant_id AND ap.deleted_at = ?
		WHERE r.quiz_id = ? AND att.deleted_at = ? AND att.submitted_at IS NOT NULL
		ORDER BY LOWER(COALESCE(a.full_name, ap.display_name)), r.id, att.attempt_no, att.id`,
		store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, quizID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	attempts := make([]ExportAttempt, 0)
	for rows.Next() {
		var (
			attempt     ExportAttempt
			submittedAt time.Time
			final       sql.NullFloat64
		)
		if err := rows.Scan(&attempt.ID, &attempt.RegistrationID, &attempt.Number,
			&submittedAt, &final); err != nil {
			return nil, internal(err)
		}
		attempt.SubmittedAt = submittedAt
		if final.Valid {
			units := ScoreUnits(final.Float64)
			attempt.FinalUnits = &units
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return attempts, nil
}

// loadExportAnswers loads every live answer of the submitted attempts and its
// selections in two bulk queries, then places each answer in live-question
// order. A question without an answer row still gets an entry, filled with the
// unanswered placeholder.
func loadExportAnswers(ctx context.Context, tx *sql.Tx, quizID int64, questions []ExportQuestion, attempts []ExportAttempt) error {
	if len(attempts) == 0 {
		return nil
	}
	attemptIDs := make([]int64, 0, len(attempts))
	for i := range attempts {
		attemptIDs = append(attemptIDs, attempts[i].ID)
		attempts[i].Answers = make([]ExportAnswer, 0, len(questions))
	}
	if len(questions) == 0 {
		return nil
	}

	type answerRow struct {
		id         int64
		questionID int64
		text       sql.NullString
		score      sql.NullFloat64
	}
	answers := make(map[int64]map[int64]answerRow, len(attempts))
	rows, err := tx.QueryContext(ctx, `
		SELECT aa.id, aa.attempt_id, aa.question_id, aa.answer_text, aa.score
		FROM attempt_answers aa
		JOIN questions q ON q.id = aa.question_id AND q.deleted_at = ?
		WHERE aa.attempt_id IN (`+intInList(attemptIDs)+`) AND aa.deleted_at = ?
		  AND q.quiz_id = ?
		ORDER BY aa.attempt_id, q.position, q.id`,
		store.LiveAt, store.LiveAt, quizID,
	)
	if err != nil {
		return internal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			row       answerRow
			attemptID int64
		)
		if err := rows.Scan(&row.id, &attemptID, &row.questionID,
			&row.text, &row.score); err != nil {
			return internal(err)
		}
		byQuestion := answers[attemptID]
		if byQuestion == nil {
			byQuestion = map[int64]answerRow{}
			answers[attemptID] = byQuestion
		}
		byQuestion[row.questionID] = row
	}
	if err := rows.Err(); err != nil {
		return internal(err)
	}

	selections := map[int64][]int64{}
	rows, err = tx.QueryContext(ctx, `
		SELECT sel.attempt_answer_id, sel.option_id
		FROM attempt_answer_selections sel
		JOIN attempt_answers aa ON aa.id = sel.attempt_answer_id AND aa.deleted_at = ?
		WHERE aa.attempt_id IN (`+intInList(attemptIDs)+`) AND sel.deleted_at = ?
		ORDER BY sel.attempt_answer_id, sel.option_id`,
		store.LiveAt, store.LiveAt,
	)
	if err != nil {
		return internal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var answerID, optionID int64
		if err := rows.Scan(&answerID, &optionID); err != nil {
			return internal(err)
		}
		selections[answerID] = append(selections[answerID], optionID)
	}
	if err := rows.Err(); err != nil {
		return internal(err)
	}

	for i := range attempts {
		attempt := &attempts[i]
		byQuestion := answers[attempt.ID]
		attempt.Answers = make([]ExportAnswer, 0, len(questions))
		for j := range questions {
			q := &questions[j]
			answer := ExportAnswer{QuestionID: q.ID, Text: exportNoAnswer}
			if row, ok := byQuestion[q.ID]; ok {
				answer.Text = exportResponse(*q, selections[row.id], row.text)
				if row.score.Valid {
					units := ScoreUnits(row.score.Float64)
					answer.ScoreUnits = &units
				}
			}
			attempt.Answers = append(attempt.Answers, answer)
		}
	}
	return nil
}

// exportResponse renders one canonical response. Objective answers name the
// stored option letter and body in canonical position order; essays return the
// trimmed stored text. A missing selection, a selection that no longer maps to
// a live option, and a blank essay are all the unanswered placeholder.
func exportResponse(q ExportQuestion, selected []int64, text sql.NullString) string {
	if q.Type == viewmodel.QuestionEssay {
		if text.Valid {
			if trimmed := strings.TrimSpace(text.String); trimmed != "" {
				return trimmed
			}
		}
		return exportNoAnswer
	}
	if len(selected) == 0 {
		return exportNoAnswer
	}

	chosen := make(map[int64]bool, len(selected))
	for _, id := range selected {
		chosen[id] = true
	}
	parts := make([]string, 0, len(selected))
	for i := range q.Options {
		option := q.Options[i]
		if !chosen[option.ID] {
			continue
		}
		parts = append(parts, viewmodel.OptionLetter(option.Position-1)+". "+option.Body)
	}
	if len(parts) == 0 {
		return exportNoAnswer
	}
	return strings.Join(parts, "; ")
}
