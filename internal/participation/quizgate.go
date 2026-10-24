package participation

// quizgate.go is PAR's read seam over the AUT-owned quiz tables. PAR never
// writes them (PAR-200, PAR-202): every loader here selects the live rows with
// the sentinel, optionally under a shared lock when a command must hold a
// state decision until commit (§9.1, §9.5.1). Attempts are read because the
// participation state and every score display are derived from them (§4.1,
// PAR-204); PAR still never inserts or updates an attempt row.

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

// quizColumns is the column list every quiz scan agrees on. The owner name is
// joined for the teacher label every participant surface shows; the target
// major's name is joined for the audience label (F-§6.3). Missing owners or a
// deleted major leave the strings empty rather than failing the page.
const quizColumns = `q.id, q.owner_account_id, q.title, q.description, q.visibility, q.state, q.code,
	q.invite_token, q.target_grade, q.target_major_id, q.starts_at, q.ends_at,
	q.allow_retake, q.max_attempts, q.show_score, q.review_mode, q.published_at,
	q.created_at, COALESCE(a.full_name, ''), COALESCE(m.name, '')`

// quizFrom is the FROM clause every quiz loader shares: owner for the label,
// target major for the audience.
const quizFrom = ` FROM quizzes q` +
	` LEFT JOIN accounts a ON a.id = q.owner_account_id` +
	` LEFT JOIN majors m ON m.id = q.target_major_id`

// quizRow is one live quizzes row plus the owning teacher's display name.
// Nullable columns stay sql.Null* everywhere (data contract R-15).
type quizRow struct {
	ID          int64
	OwnerID     int64
	Title       string
	Description string
	Visibility  string
	State       string
	Code        sql.NullString
	InviteToken string
	TargetGrade sql.NullInt64
	TargetMajor sql.NullInt64
	StartsAt    sql.NullTime
	EndsAt      sql.NullTime
	AllowRetake bool
	MaxAttempts int
	ShowScore   bool
	ReviewMode  string
	PublishedAt sql.NullTime
	CreatedAt   time.Time

	TeacherName     string
	TargetMajorName string
}

func scanQuiz(row interface{ Scan(...any) error }) (quizRow, error) {
	var q quizRow
	err := row.Scan(
		&q.ID, &q.OwnerID, &q.Title, &q.Description, &q.Visibility, &q.State,
		&q.Code, &q.InviteToken, &q.TargetGrade, &q.TargetMajor,
		&q.StartsAt, &q.EndsAt, &q.AllowRetake, &q.MaxAttempts,
		&q.ShowScore, &q.ReviewMode, &q.PublishedAt, &q.CreatedAt,
		&q.TeacherName, &q.TargetMajorName,
	)
	return q, err
}

// readQuizByID loads one live quiz, optionally with a row lock. An empty lock
// string is a plain read; " FOR UPDATE" and " LOCK IN SHARE MODE" are the two
// house-style locks (§9.1). Missing and soft-deleted are the same 404 (§3.5).
func readQuizByID(ctx context.Context, q store.Querier, id int64, lock string) (quizRow, error) {
	row, err := scanQuiz(q.QueryRowContext(ctx,
		"SELECT "+quizColumns+quizFrom+
			" WHERE q.id = ? AND q.deleted_at = ?"+lock,
		id, store.LiveAt))
	if errors.Is(err, sql.ErrNoRows) {
		return quizRow{}, apperr.NotFoundf().WithRule("PAR-063")
	}
	if err != nil {
		return quizRow{}, internal(err)
	}
	return row, nil
}

// readQuizByCode resolves a six-digit code to its live row (PAR-030). Private
// rows cannot carry a code (INV-07); no match is the caller's unknown_code.
func readQuizByCode(ctx context.Context, q store.Querier, code string) (quizRow, bool, error) {
	row, err := scanQuiz(q.QueryRowContext(ctx,
		"SELECT "+quizColumns+quizFrom+
			" WHERE q.code = ? AND q.deleted_at = ?",
		code, store.LiveAt))
	if errors.Is(err, sql.ErrNoRows) {
		return quizRow{}, false, nil
	}
	if err != nil {
		return quizRow{}, false, internal(err)
	}
	return row, true, nil
}

// readQuizByInvite resolves an invite token to its live row (PAR-037).
func readQuizByInvite(ctx context.Context, q store.Querier, token string) (quizRow, bool, error) {
	row, err := scanQuiz(q.QueryRowContext(ctx,
		"SELECT "+quizColumns+quizFrom+
			" WHERE q.invite_token = ? AND q.deleted_at = ?",
		token, store.LiveAt))
	if errors.Is(err, sql.ErrNoRows) {
		return quizRow{}, false, nil
	}
	if err != nil {
		return quizRow{}, false, internal(err)
	}
	return row, true, nil
}

// readQuizzesByIds batch-loads live quizzes in one snapshot. The caller owns
// the transaction choice; the order of ids governs nothing.
func readQuizzesByIds(ctx context.Context, q store.Querier, ids []int64) (map[int64]quizRow, error) {
	out := map[int64]quizRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx,
		"SELECT "+quizColumns+quizFrom+
			" WHERE q.id IN ("+placeholders(len(ids))+") AND q.deleted_at = ?",
		args(ids, store.LiveAt)...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanQuiz(rows)
		if err != nil {
			return nil, internal(err)
		}
		out[item.ID] = item
	}
	return out, rows.Err()
}

// readAssignedQuizzes loads the open live quizzes one student is assigned to
// (PAR-127): public, or private with both targets equal to the profile.
func readAssignedQuizzes(ctx context.Context, q store.Querier, grade uint8, majorID uint32) ([]quizRow, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT "+quizColumns+quizFrom+
			" WHERE q.state = 'open' AND q.deleted_at = ?"+
			" AND (q.visibility = 'public' OR (q.target_grade = ? AND q.target_major_id = ?))"+
			" ORDER BY q.published_at DESC, q.id ASC",
		store.LiveAt, int64(grade), int64(majorID))
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []quizRow{}
	for rows.Next() {
		item, err := scanQuiz(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// liveQuestionCount is the defense-in-depth zero-question gate (PAR-057,
// PAR-079, PAR-201). It counts live questions of one quiz.
func liveQuestionCount(ctx context.Context, q store.Querier, quizID int64) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM questions WHERE quiz_id = ? AND deleted_at = ?",
		quizID, store.LiveAt).Scan(&n); err != nil {
		return 0, internal(err)
	}
	return n, nil
}

// liveQuestionCounts batch-loads the live question counts for a set of quizzes.
// Missing keys mean zero live questions.
func liveQuestionCounts(ctx context.Context, q store.Querier, quizIDs []int64) (map[int64]int, error) {
	out := map[int64]int{}
	if len(quizIDs) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx,
		"SELECT quiz_id, COUNT(*) FROM questions WHERE quiz_id IN ("+
			placeholders(len(quizIDs))+") AND deleted_at = ? GROUP BY quiz_id",
		args(quizIDs, store.LiveAt)...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, internal(err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

// registrations is one live registration row.
type registrationRow struct {
	ID        int64
	QuizID    int64
	StudentID sql.NullInt64
	AnonID    sql.NullInt64
	CreatedAt time.Time
}

// readStudentRegistrations loads every live registration of one student
// (PAR-127). Zero-attempt and closed-quiz rows stay; cancelled ones do not.
func readStudentRegistrations(ctx context.Context, q store.Querier, studentID int64) ([]registrationRow, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT id, quiz_id, student_id, anonymous_participant_id, created_at FROM registrations"+
			" WHERE student_id = ? AND deleted_at = ? ORDER BY id",
		studentID, store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	return scanRegistrations(rows)
}

// readAnonRegistrations loads every live registration of one anonymous
// identity. Cancelled ones do not appear (PAR-INV-02).
func readAnonRegistrations(ctx context.Context, q store.Querier, anonID int64) ([]registrationRow, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT id, quiz_id, student_id, anonymous_participant_id, created_at FROM registrations"+
			" WHERE anonymous_participant_id = ? AND deleted_at = ? ORDER BY id",
		anonID, store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	return scanRegistrations(rows)
}

func scanRegistrations(rows *sql.Rows) ([]registrationRow, error) {
	out := []registrationRow{}
	for rows.Next() {
		var r registrationRow
		if err := rows.Scan(&r.ID, &r.QuizID, &r.StudentID, &r.AnonID, &r.CreatedAt); err != nil {
			return nil, internal(err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// readRegistration looks up one live registration for (quiz, student) without
// locking. The commands re-read it under a lock when they act on it.
func readStudentRegistration(ctx context.Context, q store.Querier, quizID, studentID int64) (registrationRow, bool, error) {
	var r registrationRow
	err := q.QueryRowContext(ctx,
		"SELECT id, quiz_id, student_id, anonymous_participant_id, created_at FROM registrations"+
			" WHERE quiz_id = ? AND student_id = ? AND deleted_at = ?",
		quizID, studentID, store.LiveAt).
		Scan(&r.ID, &r.QuizID, &r.StudentID, &r.AnonID, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return registrationRow{}, false, nil
	}
	if err != nil {
		return registrationRow{}, false, internal(err)
	}
	return r, true, nil
}

// readAnonRegistration is the anonymous twin of readStudentRegistration.
func readAnonRegistration(ctx context.Context, q store.Querier, quizID, anonID int64) (registrationRow, bool, error) {
	var r registrationRow
	err := q.QueryRowContext(ctx,
		"SELECT id, quiz_id, student_id, anonymous_participant_id, created_at FROM registrations"+
			" WHERE quiz_id = ? AND anonymous_participant_id = ? AND deleted_at = ?",
		quizID, anonID, store.LiveAt).
		Scan(&r.ID, &r.QuizID, &r.StudentID, &r.AnonID, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return registrationRow{}, false, nil
	}
	if err != nil {
		return registrationRow{}, false, internal(err)
	}
	return r, true, nil
}

// attemptRecord is one live quiz_attempts row. PAR reads it for state,
// numbering, scores and last-activity; it never writes it (PAR-202).
type attemptRecord struct {
	ID         int64
	Number     int
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Submitted  sql.NullTime
	FinalScore sql.NullFloat64
}

// readAttemptsByRegistration batch-loads every live attempt of a set of
// registrations, grouped and ordered by attempt_no ascending (PAR-142).
func readAttemptsByRegistration(ctx context.Context, q store.Querier, regIDs []int64) (map[int64][]attemptRecord, error) {
	out := map[int64][]attemptRecord{}
	if len(regIDs) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx,
		"SELECT registration_id, id, attempt_no, created_at, updated_at, submitted_at, final_score"+
			" FROM quiz_attempts WHERE registration_id IN ("+placeholders(len(regIDs))+")"+
			" AND deleted_at = ? ORDER BY attempt_no, id",
		args(regIDs, store.LiveAt)...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a attemptRecord
		var regID int64
		if err := rows.Scan(&regID, &a.ID, &a.Number, &a.CreatedAt, &a.UpdatedAt,
			&a.Submitted, &a.FinalScore); err != nil {
			return nil, internal(err)
		}
		out[regID] = append(out[regID], a)
	}
	return out, rows.Err()
}

// buildAttemptRows projects attempt records to display rows under one quiz's
// score visibility (PAR-147, PAR-193, PAR-196): a hidden score and a NULL
// score both render "—", a graded 0 renders "0%", and waiting is not a score.
// Out-of-range stored scores render "—" and are logged, never clamped (8.6.4).
func (s *Service) buildAttemptRows(records []attemptRecord, showScore bool) []viewmodel.AttemptRow {
	out := make([]viewmodel.AttemptRow, 0, len(records))
	for _, a := range records {
		row := viewmodel.AttemptRow{
			ID:          a.ID,
			Number:      a.Number,
			StartedAt:   localPtr(sql.NullTime{Time: a.CreatedAt, Valid: true}),
			SubmittedAt: localPtr(a.Submitted),
			State:       viewmodel.AttemptCompleted,
			FocusLoss:   0, // PAR-158: focus totals are never shown on student surfaces.
		}
		if a.Submitted.Valid {
			row.ResultHref = attemptResultPath(uint32(a.ID))
		} else {
			row.State = viewmodel.AttemptInProgress
			row.ResultHref = attemptPath(uint32(a.ID))
		}
		row.Waiting = a.Submitted.Valid && !a.FinalScore.Valid
		if showScore && a.FinalScore.Valid {
			if a.FinalScore.Float64 < 0 || a.FinalScore.Float64 > 1 {
				s.oops("PAR-189", "stored final_score is outside the 0–1 range", nil)
			} else {
				score := viewmodel.NewScoreFromRatio(a.FinalScore.Float64)
				row.Score = &score
			}
		}
		row.ReviewHref = row.ResultHref
		out = append(out, row)
	}
	return out
}

// deriveState is the §4.1 table (PAR-INV-03): a pure function of the live
// attempt count and whether any attempt is still open.
func deriveState(total int, hasOpen bool) viewmodel.ParticipationState {
	switch {
	case total == 0:
		return viewmodel.ParticipationRegistered
	case hasOpen:
		return viewmodel.ParticipationInProgress
	default:
		return viewmodel.ParticipationCompleted
	}
}

// eligible is the eligibility predicate of PAR-127/PAR-062/PAR-079: public
// admits everyone; private requires both targets to match the live profile.
func eligible(q quizRow, grade uint8, majorID uint32) bool {
	if viewmodel.Visibility(q.Visibility) == viewmodel.VisibilityPublic {
		return true
	}
	return q.TargetGrade.Valid && int(q.TargetGrade.Int64) == int(grade) &&
		q.TargetMajor.Valid && uint32(q.TargetMajor.Int64) == majorID
}

// maxAttemptsFor normalises a quiz row (PAR-081): stored 0 behaves as 1 and a
// no-retake row caps at one attempt. The boolean tells the caller to WARN
// (fail closed) before the limited value is used.
func maxAttemptsFor(q quizRow) (int, bool) {
	return effectiveAttempts(q.MaxAttempts, q.AllowRetake)
}

// audienceLabel renders the audience pair shared by the invite landing and the
// intro/history surfaces (PAR-190, F-§6.3). A private quiz whose major was
// since deleted degrades to the grade-only label instead of an error.
func audienceLabel(q quizRow) string {
	if viewmodel.Visibility(q.Visibility) == viewmodel.VisibilityPublic {
		return "Semua kelas"
	}
	majorName := q.TargetMajorName
	grade, hasGrade := 0, q.TargetGrade.Valid
	if hasGrade {
		grade = int(q.TargetGrade.Int64)
	}
	if hasGrade {
		return viewmodel.ClassLabel(grade, majorName)
	}
	if q.TargetMajor.Valid {
		if majorName == "" {
			return "Semua tingkat"
		}
		return "Semua tingkat — " + majorName
	}
	return "Semua kelas"
}

// placeholders builds a "?," list for an IN clause; an empty set binds nothing.
func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

// args flattens a slice of ids plus trailing extra values into driver args, in
// the order the SQL declares them: the IN (...) list first, then any trailing
// sentinel such as deleted_at. Callers must keep that clause order.
func args(ids []int64, extra ...any) []any {
	out := make([]any, 0, len(ids)+len(extra))
	for _, id := range ids {
		out = append(out, id)
	}
	return append(out, extra...)
}
