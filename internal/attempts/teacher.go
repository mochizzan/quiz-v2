package attempts

// teacher.go implements the teacher-facing read models and the one teacher
// write (business logic 04 §5.8–§5.15): the dashboard cards, the per-quiz
// results table, the live monitor snapshot, the essay grading queue and
// detail, the grade command, and the attempt recap.
//
// Every screen is owner-scoped through quizzes.owner_account_id; a foreign or
// missing resource is ErrNotFound, never 403 (ATT-020, INV-26). Each read runs
// in one read-only repeatable-read transaction so the aggregates on a page
// come from one snapshot (ATT-161, ATT-171, ATT-181, ATT-201, ATT-211, ATT-231).
// The lists load their rows with joins and grouped aggregates, never one query
// per row (ATT-249).

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

const (
	resultsPerPage  = 10
	gradingPerPage  = 20
	dashboardTopCap = 5
	joinCardCap     = 3
	excerptRunes    = 120
)

// ---- Dashboard ------------------------------------------------------------

// TeacherDashboard is ATT-161…ATT-166: the four cards read from one snapshot.
// GreetingName comes from the session and DateLine from the database clock in
// WIB, so the page never mixes two clocks.
func (s *Service) TeacherDashboard(ctx context.Context, accountID int64, fullName string) (viewmodel.TeacherDashboard, error) {
	var out viewmodel.TeacherDashboard
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		top, err := loadTopScores(ctx, tx, accountID)
		if err != nil {
			return err
		}
		recent, err := loadRecentQuizzes(ctx, tx, accountID)
		if err != nil {
			return err
		}
		joined, err := loadParticipation(ctx, tx, accountID)
		if err != nil {
			return err
		}
		now, err := nowT(ctx, tx)
		if err != nil {
			return err
		}
		out = viewmodel.TeacherDashboard{
			GreetingName: fullName,
			DateLine:     viewmodel.NewLocalTime(now).Long(),
			TopScores:    top,
			Recent:       recent,
			MostJoined:   rankParticipation(joined, true),
			LeastJoined:  rankParticipation(joined, false),
		}
		return nil
	})
	if err != nil {
		return viewmodel.TeacherDashboard{}, err
	}
	return out, nil
}

// loadTopScores implements ATT-162. One ordered query returns every eligible
// attempt; the first row per quiz is that quiz's best attempt, and the cap
// applies to quizzes, not attempts. Hidden scores are excluded by the query,
// never rendered as zero (INV-17).
func loadTopScores(ctx context.Context, tx *sql.Tx, accountID int64) ([]viewmodel.QuizTopScore, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT z.id, z.title, att.final_score,
		       COALESCE(a.full_name, ap.display_name),
		       COALESCE(COALESCE(s.grade_level, ap.grade_level), 0), mj.name
		FROM quiz_attempts att
		JOIN registrations r ON r.id = att.registration_id AND r.deleted_at = ?
		JOIN quizzes z ON z.id = r.quiz_id AND z.deleted_at = ?
		LEFT JOIN students s ON s.id = r.student_id AND s.deleted_at = ?
		LEFT JOIN accounts a ON a.id = s.account_id AND a.deleted_at = ?
		LEFT JOIN anonymous_participants ap ON ap.id = r.anonymous_participant_id AND ap.deleted_at = ?
		LEFT JOIN majors mj ON mj.id = COALESCE(s.major_id, ap.major_id) AND mj.deleted_at = ?
		WHERE z.owner_account_id = ? AND z.show_score = 1
		  AND att.deleted_at = ? AND att.submitted_at IS NOT NULL AND att.final_score IS NOT NULL
		ORDER BY z.created_at DESC, z.id DESC,
		         att.final_score DESC, att.submitted_at ASC, att.id ASC`,
		store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt,
		accountID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	var out []viewmodel.QuizTopScore
	seen := map[int64]bool{}
	for rows.Next() {
		var (
			quizID int64
			title  string
			final  sql.NullFloat64
			name   sql.NullString
			grade  sql.NullInt64
			major  sql.NullString
		)
		if err := rows.Scan(&quizID, &title, &final, &name, &grade, &major); err != nil {
			return nil, internal(err)
		}
		if seen[quizID] || len(out) == dashboardTopCap {
			continue
		}
		seen[quizID] = true
		if !name.Valid {
			return nil, integrity("quiz %d top score has no readable name (ATT-282)", quizID)
		}
		out = append(out, viewmodel.QuizTopScore{
			Quiz:        viewmodel.QuizRef{ID: quizID, Title: title},
			StudentName: name.String,
			ClassLabel:  viewmodel.ClassLabel(int(grade.Int64), major.String),
			Score:       viewmodel.Score{Percent: DisplayPercent(ScoreUnits(final.Float64))},
			Href:        resultsPath(quizID),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// loadRecentQuizzes implements ATT-163: the five newest owned live quizzes,
// drafts included, with their live question and registration counts.
func loadRecentQuizzes(ctx context.Context, tx *sql.Tx, accountID int64) ([]viewmodel.RecentQuiz, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT z.id, z.title, z.state, z.created_at,
		       (SELECT COUNT(*) FROM questions qq
		         WHERE qq.quiz_id = z.id AND qq.deleted_at = ?) AS question_count,
		       (SELECT COUNT(*) FROM registrations r
		         WHERE r.quiz_id = z.id AND r.deleted_at = ?) AS joined
		FROM quizzes z
		WHERE z.owner_account_id = ? AND z.deleted_at = ?
		ORDER BY z.created_at DESC, z.id DESC
		LIMIT ?`,
		store.LiveAt, store.LiveAt, accountID, store.LiveAt, dashboardTopCap,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	var out []viewmodel.RecentQuiz
	for rows.Next() {
		var (
			row viewmodel.RecentQuiz
			at  time.Time
		)
		if err := rows.Scan(&row.ID, &row.Title, &row.State, &at,
			&row.QuestionCount, &row.Joined); err != nil {
			return nil, internal(err)
		}
		row.CreatedAt = viewmodel.NewLocalTime(at)
		row.Href = quizPath(row.ID)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// loadParticipation implements ATT-164: only open and closed quizzes can be
// joined, so drafts never occupy a "least joined" slot.
func loadParticipation(ctx context.Context, tx *sql.Tx, accountID int64) ([]viewmodel.QuizParticipation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT z.id, z.title,
		       (SELECT COUNT(*) FROM registrations r
		         WHERE r.quiz_id = z.id AND r.deleted_at = ?) AS joined
		FROM quizzes z
		WHERE z.owner_account_id = ? AND z.deleted_at = ?
		  AND z.state IN ('open', 'closed')`,
		store.LiveAt, accountID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	var out []viewmodel.QuizParticipation
	for rows.Next() {
		var p viewmodel.QuizParticipation
		if err := rows.Scan(&p.Quiz.ID, &p.Quiz.Title, &p.Joined); err != nil {
			return nil, internal(err)
		}
		p.Href = quizPath(p.Quiz.ID)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// rankParticipation sorts a copy for the most/least card. Ties break by
// LOWER(title) and then id, so the two cards are deterministic (ATT-164).
func rankParticipation(in []viewmodel.QuizParticipation, most bool) []viewmodel.QuizParticipation {
	if len(in) == 0 {
		return nil
	}
	out := append([]viewmodel.QuizParticipation(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Joined != out[j].Joined {
			if most {
				return out[i].Joined > out[j].Joined
			}
			return out[i].Joined < out[j].Joined
		}
		left, right := strings.ToLower(out[i].Quiz.Title), strings.ToLower(out[j].Quiz.Title)
		if left != right {
			return left < right
		}
		return out[i].Quiz.ID < out[j].Quiz.ID
	})
	if len(out) > joinCardCap {
		out = out[:joinCardCap]
	}
	return out
}

// ---- Results --------------------------------------------------------------

// ResultsInput is GET /teacher/quizzes/:id/results material.
type ResultsInput struct {
	QuizID    int64
	AccountID int64
	StateRaw  string
	ClassRaw  string
	QueryRaw  string
	PageRaw   string
}

// Results implements ATT-171…ATT-178. Tabs count the rows after the search and
// class filters but before the state filter; the header metrics and the
// average ignore every filter.
func (s *Service) Results(ctx context.Context, in ResultsInput) (viewmodel.QuizResults, error) {
	var out viewmodel.QuizResults
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		quiz, err := loadOwnedQuiz(ctx, tx, in.QuizID, in.AccountID)
		if err != nil {
			return err
		}
		if quiz.State == viewmodel.QuizDraft {
			// A draft cannot have attempts (ATT-171).
			return ErrNotFound
		}

		search := normalizeSearch(in.QueryRaw)
		state := resultsState(in.StateRaw)
		class, classOn := parseRomanClass(in.ClassRaw)

		attempts, err := loadResultAttempts(ctx, tx, quiz.ID)
		if err != nil {
			return err
		}

		// The header metrics, the average, and the tab counts are quiz-wide:
		// ATT-173 and ATT-174 make them independent of every filter. Only the
		// rows are filtered, so the search and class predicates run below.
		submitted, inProgress := 0, 0
		var finals []int
		for i := range attempts {
			if !attempts[i].submittedAt.Valid {
				inProgress++
				continue
			}
			submitted++
			if attempts[i].final.Valid {
				finals = append(finals, ScoreUnits(attempts[i].final.Float64))
			}
		}

		filtered := make([]resultAttemptRow, 0, len(attempts))
		for i := range attempts {
			if !attempts[i].submittedAt.Valid {
				continue
			}
			if classOn && attempts[i].grade != class {
				continue
			}
			if search != "" && !strings.Contains(strings.ToLower(attempts[i].name), search) {
				continue
			}
			filtered = append(filtered, attempts[i])
		}

		waiting, graded := 0, 0
		for i := range filtered {
			if filtered[i].final.Valid {
				graded++
			} else {
				waiting++
			}
		}

		shown := make([]resultAttemptRow, 0, len(filtered))
		for i := range filtered {
			switch state {
			case "waiting":
				if filtered[i].final.Valid {
					continue
				}
			case "graded":
				if !filtered[i].final.Valid {
					continue
				}
			}
			shown = append(shown, filtered[i])
		}

		rows := make([]viewmodel.ResultsRow, 0, len(shown))
		for i := range shown {
			row := shown[i]
			view := viewmodel.ResultsRow{
				AttemptID:   row.attemptID,
				StudentName: row.name,
				ClassLabel:  viewmodel.ClassLabel(row.grade, row.major),
				AttemptNo:   row.attemptNo,
				MaxAttempts: quiz.effectiveMax(),
				SubmittedAt: ptrLocalTime(row.submittedAt),
				FocusLoss:   row.focus,
				RecapHref:   viewmodel.RecapHref(row.attemptID),
				Waiting:     !row.final.Valid,
			}
			if row.final.Valid {
				view.Score = scorePtr(viewmodel.Score{Percent: DisplayPercent(ScoreUnits(row.final.Float64))})
			}
			rows = append(rows, view)
		}

		list := viewmodel.NewList(rows, parsePage(in.PageRaw), resultsPerPage)
		values := resultsValues(state, class, classOn, search)
		out = viewmodel.QuizResults{
			Quiz:       viewmodel.QuizRef{ID: quiz.ID, Title: quiz.Title},
			Counts:     viewmodel.ResultsCounts{Submitted: submitted, InProgress: inProgress},
			Rows:       list,
			Pagination: viewmodel.NewPagination(resultsPath(quiz.ID), values, list.Page, list.PerPage, list.Total),
			Tabs:       resultsTabs(quiz.ID, state, class, classOn, search, len(filtered), waiting, graded),
			Query:      viewmodel.QueryState{Q: search, Tab: state},
			BackHref:   quizPath(quiz.ID),
			// The header action is this quiz's slice of the essay queue, not the
			// whole-school /teacher/grading the shared nav points at.
			GradingHref: "/teacher/grading?quiz=" + strconv.FormatInt(quiz.ID, 10),
			ExportHref:  "/teacher/quizzes/" + strconv.FormatInt(quiz.ID, 10) + "/export.xlsx",
			ReviewMode:  quiz.ReviewMode,
		}
		if tenths, ok := AverageTenths(finals); ok {
			avg := viewmodel.NewAverage(float64(tenths) / 10)
			out.Counts.Average = &avg
		}
		out.Empty = resultsEmpty(quiz.ID, submitted)
		return nil
	})
	if err != nil {
		return viewmodel.QuizResults{}, err
	}
	return out, nil
}

type ownedQuiz struct {
	ID          int64
	Title       string
	State       viewmodel.QuizState
	ReviewMode  viewmodel.ReviewMode
	AllowRetake bool
	MaxAttempts int
}

func (q ownedQuiz) effectiveMax() int {
	return EffectiveMaxAttempts(q.AllowRetake, q.MaxAttempts)
}

func loadOwnedQuiz(ctx context.Context, tx *sql.Tx, quizID, accountID int64) (ownedQuiz, error) {
	var q ownedQuiz
	err := tx.QueryRowContext(ctx, `
		SELECT id, title, state, review_mode, allow_retake, max_attempts
		FROM quizzes
		WHERE id = ? AND owner_account_id = ? AND deleted_at = ?`,
		quizID, accountID, store.LiveAt,
	).Scan(&q.ID, &q.Title, &q.State, &q.ReviewMode, &q.AllowRetake, &q.MaxAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		return ownedQuiz{}, ErrNotFound
	}
	if err != nil {
		return ownedQuiz{}, internal(err)
	}
	return q, nil
}

type resultAttemptRow struct {
	attemptID   int64
	attemptNo   int
	submittedAt sql.NullTime
	final       sql.NullFloat64
	name        string
	grade       int
	major       string
	focus       int
}

// loadResultAttempts loads every live attempt of the quiz (submitted and in
// progress) because the header counts need both; the caller drops the
// in-progress rows (ATT-176). The focus count is a correlated aggregate in the
// same query, never a second query per row (ATT-249). Search runs in the
// caller so the quiz-wide metrics stay unfiltered (ATT-173).
func loadResultAttempts(ctx context.Context, tx *sql.Tx, quizID int64) ([]resultAttemptRow, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT att.id, att.attempt_no, att.submitted_at, att.final_score,
		       COALESCE(a.full_name, ap.display_name),
		       COALESCE(COALESCE(s.grade_level, ap.grade_level), 0), mj.name,
		       (SELECT COUNT(*) FROM focus_loss_events f
		         WHERE f.attempt_id = att.id AND f.deleted_at = ?) AS focus_loss
		FROM quiz_attempts att
		JOIN registrations r ON r.id = att.registration_id AND r.deleted_at = ?
		LEFT JOIN students s ON s.id = r.student_id AND s.deleted_at = ?
		LEFT JOIN accounts a ON a.id = s.account_id AND a.deleted_at = ?
		LEFT JOIN anonymous_participants ap ON ap.id = r.anonymous_participant_id AND ap.deleted_at = ?
		LEFT JOIN majors mj ON mj.id = COALESCE(s.major_id, ap.major_id) AND mj.deleted_at = ?
		WHERE r.quiz_id = ? AND att.deleted_at = ?
		ORDER BY att.submitted_at DESC, att.id ASC`,
		store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt,
		quizID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	out := make([]resultAttemptRow, 0)
	for rows.Next() {
		var (
			row  resultAttemptRow
			name sql.NullString
			mj   sql.NullString
		)
		if err := rows.Scan(&row.attemptID, &row.attemptNo, &row.submittedAt, &row.final,
			&name, &row.grade, &mj, &row.focus); err != nil {
			return nil, internal(err)
		}
		if !name.Valid {
			return nil, integrity("attempt %d has no readable name (ATT-282)", row.attemptID)
		}
		row.name = name.String
		row.major = mj.String
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

func resultsState(raw string) string {
	switch strings.TrimSpace(raw) {
	case "waiting", "graded":
		return strings.TrimSpace(raw)
	default:
		return "all"
	}
}

func resultsTabs(quizID int64, state string, class int, classOn bool, search string, total, waiting, graded int) []viewmodel.FilterTab {
	return []viewmodel.FilterTab{
		{Label: "Semua", Href: resultsHref(quizID, "all", class, classOn, search), Active: state == "all", Count: total},
		{Label: "Menunggu", Href: resultsHref(quizID, "waiting", class, classOn, search), Active: state == "waiting", Count: waiting},
		{Label: "Sudah dinilai", Href: resultsHref(quizID, "graded", class, classOn, search), Active: state == "graded", Count: graded},
	}
}

func resultsHref(quizID int64, state string, class int, classOn bool, search string) string {
	values := resultsValues(state, class, classOn, search)
	if len(values) == 0 {
		return resultsPath(quizID)
	}
	return resultsPath(quizID) + "?" + values.Encode()
}

func resultsValues(state string, class int, classOn bool, search string) url.Values {
	values := url.Values{}
	if state != "" && state != "all" {
		values.Set("state", state)
	}
	if classOn {
		values.Set("class", viewmodel.RomanGrade(class))
	}
	if search != "" {
		values.Set("q", search)
	}
	return values
}

// resultsEmpty carries an action only for the filtered-empty list: a quiz with
// no attempt yet has nothing to do here except go back, and the back link above
// the heading is already that destination (DESIGN §8).
func resultsEmpty(quizID int64, submitted int) viewmodel.EmptyState {
	if submitted == 0 {
		return viewmodel.EmptyState{
			Title: "Belum ada percobaan.",
			Body:  "Percobaan siswa akan muncul di sini setelah mereka mengirim jawaban.",
			Icon:  "quiz",
		}
	}
	return viewmodel.EmptyState{
		Title:  "Tidak ada percobaan yang cocok.",
		Body:   "Coba kata kunci lain atau tampilkan semua percobaan.",
		Icon:   "search",
		Action: actionPtr(viewmodel.Outline("Tampilkan semua", resultsPath(quizID))),
	}
}

// ---- Monitor --------------------------------------------------------------

// MonitorInput is GET /teacher/quizzes/:id/monitor material.
type MonitorInput struct {
	QuizID    int64
	AccountID int64
}

// participantSnapshot is one live registration's derived row (ATT-182). The
// monitor page and the SSE stream both read this shape; the stream is the
// monitor model minus StreamHref (ATT-187).
//
// Score and StartedAt are page-only: the monitor keeps the score and attempt
// start columns by product decision (against ATT-185), so they are rendered
// once from the server and refreshed by a reload. They never enter the SSE
// payload (ATT-198). participantChanged still treats a score change as a change
// so an open monitor learns that a late essay grade is ready.
type participantSnapshot struct {
	RegistrationID int64
	AttemptID      *int64
	Name           string
	Initials       string
	ClassLabel     string
	State          viewmodel.ParticipationState
	Answered       int
	Total          int
	FocusLoss      int
	LastSeen       time.Time
	StartedAt      time.Time
	Score          *viewmodel.Score
	Anonymous      bool
}

type monitorSnapshotData struct {
	QuizRef       viewmodel.QuizRef
	State         viewmodel.QuizState
	QuestionCount int
	Rows          []participantSnapshot
	Progress      viewmodel.MonitorProgress
	GeneratedAt   time.Time
}

// MonitorPage implements ATT-181…ATT-187. Draft and closed quizzes render the
// not-on-air empty state with 200; only a missing/foreign quiz is 404.
func (s *Service) MonitorPage(ctx context.Context, in MonitorInput) (viewmodel.MonitorPage, error) {
	var out viewmodel.MonitorPage
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		snap, err := loadMonitorSnapshot(ctx, tx, in.QuizID, in.AccountID)
		if err != nil {
			return err
		}
		out = viewmodel.MonitorPage{
			Quiz:        snap.QuizRef,
			Progress:    snap.Progress,
			QuizHref:    quizPath(snap.QuizRef.ID),
			GeneratedAt: viewmodel.NewLocalTime(snap.GeneratedAt),
		}
		if snap.State == viewmodel.QuizOpen {
			out.StreamHref = quizPath(snap.QuizRef.ID) + "/stream"
			out.Rows = make([]viewmodel.ParticipantRow, 0, len(snap.Rows))
			for i := range snap.Rows {
				row := snap.Rows[i]
				lastSeen := viewmodel.NewLocalTime(row.LastSeen)
				mapped := viewmodel.ParticipantRow{
					RegistrationID: row.RegistrationID,
					AttemptID:      row.AttemptID,
					StudentName:    row.Name,
					Initials:       row.Initials,
					ClassLabel:     row.ClassLabel,
					State:          row.State,
					Answered:       row.Answered,
					Total:          row.Total,
					FocusLoss:      row.FocusLoss,
					Score:          row.Score,
					UpdatedAt:      &lastSeen,
					Anonymous:      row.Anonymous,
				}
				if !row.StartedAt.IsZero() {
					started := viewmodel.NewLocalTime(row.StartedAt)
					mapped.StartedAt = &started
				}
				out.Rows = append(out.Rows, mapped)
			}
			// The empty branches carry no action: the back link above the
			// heading already leads to the quiz, and a second control for the
			// same destination is the duplicate this screen must not have.
			out.Empty = viewmodel.EmptyState{
				Title: "Belum ada peserta",
				Body:  "Peserta yang bergabung akan muncul di sini secara langsung.",
				Icon:  "users",
			}
			return nil
		}
		out.Empty = viewmodel.EmptyState{
			Title: "Kuis tidak sedang berlangsung",
			Body:  "Monitor hanya tersedia selama kuis dibuka. Setelah ditutup, gunakan halaman hasil untuk melihat rekap.",
			Icon:  "clock",
		}
		return nil
	})
	if err != nil {
		return viewmodel.MonitorPage{}, err
	}
	return out, nil
}

// loadMonitorSnapshot is the one derivation the monitor page and the stream
// share (ATT-187). It reads the quiz, registrations, selected attempts, focus
// counts, and answered counts in constant queries, then derives and orders the
// rows exactly once (ATT-182–ATT-184).
func loadMonitorSnapshot(ctx context.Context, tx *sql.Tx, quizID, accountID int64) (monitorSnapshotData, error) {
	var snap monitorSnapshotData
	err := tx.QueryRowContext(ctx, `
		SELECT id, title, state
		FROM quizzes
		WHERE id = ? AND owner_account_id = ? AND deleted_at = ?`,
		quizID, accountID, store.LiveAt,
	).Scan(&snap.QuizRef.ID, &snap.QuizRef.Title, &snap.State)
	if errors.Is(err, sql.ErrNoRows) {
		return monitorSnapshotData{}, ErrNotFound
	}
	if err != nil {
		return monitorSnapshotData{}, internal(err)
	}

	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM questions WHERE quiz_id = ? AND deleted_at = ?`,
		quizID, store.LiveAt,
	).Scan(&snap.QuestionCount); err != nil {
		return monitorSnapshotData{}, internal(err)
	}
	if snap.GeneratedAt, err = nowT(ctx, tx); err != nil {
		return monitorSnapshotData{}, err
	}
	if snap.State != viewmodel.QuizOpen {
		return snap, nil
	}

	type registrationRow struct {
		id        int64
		name      string
		grade     int
		major     string
		anonymous bool
		createdAt time.Time
	}
	regs, err := tx.QueryContext(ctx, `
		SELECT r.id, COALESCE(a.full_name, ap.display_name),
		       COALESCE(COALESCE(s.grade_level, ap.grade_level), 0), mj.name,
		       r.anonymous_participant_id IS NOT NULL, r.created_at
		FROM registrations r
		LEFT JOIN students s ON s.id = r.student_id AND s.deleted_at = ?
		LEFT JOIN accounts a ON a.id = s.account_id AND a.deleted_at = ?
		LEFT JOIN anonymous_participants ap ON ap.id = r.anonymous_participant_id AND ap.deleted_at = ?
		LEFT JOIN majors mj ON mj.id = COALESCE(s.major_id, ap.major_id) AND mj.deleted_at = ?
		WHERE r.quiz_id = ? AND r.deleted_at = ?
		ORDER BY r.id ASC`,
		store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, quizID, store.LiveAt,
	)
	if err != nil {
		return monitorSnapshotData{}, internal(err)
	}
	defer regs.Close()

	registrations := make([]registrationRow, 0)
	for regs.Next() {
		var (
			row  registrationRow
			name sql.NullString
			mj   sql.NullString
		)
		if err := regs.Scan(&row.id, &name, &row.grade, &mj, &row.anonymous, &row.createdAt); err != nil {
			return monitorSnapshotData{}, internal(err)
		}
		if !name.Valid {
			return monitorSnapshotData{}, integrity("registration %d has no readable name (ATT-282)", row.id)
		}
		row.name = name.String
		row.major = mj.String
		registrations = append(registrations, row)
	}
	if err := regs.Err(); err != nil {
		return monitorSnapshotData{}, internal(err)
	}

	type attemptRow struct {
		id           int64
		registration int64
		number       int
		submittedAt  sql.NullTime
		updatedAt    time.Time
		createdAt    time.Time
		finalScore   sql.NullFloat64
	}
	attemptRows, err := tx.QueryContext(ctx, `
		SELECT att.id, att.registration_id, att.attempt_no, att.submitted_at,
		       att.updated_at, att.created_at, att.final_score
		FROM quiz_attempts att
		JOIN registrations r ON r.id = att.registration_id AND r.deleted_at = ?
		WHERE r.quiz_id = ? AND att.deleted_at = ?
		ORDER BY att.registration_id, att.attempt_no, att.id`,
		store.LiveAt, quizID, store.LiveAt,
	)
	if err != nil {
		return monitorSnapshotData{}, internal(err)
	}
	defer attemptRows.Close()

	byRegistration := map[int64][]attemptRow{}
	for attemptRows.Next() {
		var row attemptRow
		if err := attemptRows.Scan(&row.id, &row.registration, &row.number, &row.submittedAt,
			&row.updatedAt, &row.createdAt, &row.finalScore); err != nil {
			return monitorSnapshotData{}, internal(err)
		}
		byRegistration[row.registration] = append(byRegistration[row.registration], row)
	}
	if err := attemptRows.Err(); err != nil {
		return monitorSnapshotData{}, internal(err)
	}

	selected := make([]attemptRow, 0, len(registrations))
	selectedIDs := make([]int64, 0, len(registrations))
	for i := range registrations {
		var current *attemptRow
		list := byRegistration[registrations[i].id]
		for j := range list {
			if !list[j].submittedAt.Valid && (current == nil || list[j].number > current.number ||
				(list[j].number == current.number && list[j].id > current.id)) {
				current = &list[j]
			}
		}
		if current == nil {
			for j := range list {
				if current == nil || list[j].number > current.number ||
					(list[j].number == current.number && list[j].id > current.id) {
					current = &list[j]
				}
			}
		}
		if current != nil {
			selected = append(selected, *current)
			selectedIDs = append(selectedIDs, current.id)
		}
	}

	focus := map[int64]int{}
	hits := map[int64]int{}
	if len(selectedIDs) > 0 {
		list := intInList(selectedIDs)
		rows, err := tx.QueryContext(ctx, `
			SELECT attempt_id, COUNT(*) FROM focus_loss_events
			WHERE attempt_id IN (`+list+`) AND deleted_at = ?
			GROUP BY attempt_id`,
			store.LiveAt,
		)
		if err != nil {
			return monitorSnapshotData{}, internal(err)
		}
		for rows.Next() {
			var attemptID int64
			var count int
			if err := rows.Scan(&attemptID, &count); err != nil {
				rows.Close()
				return monitorSnapshotData{}, internal(err)
			}
			focus[attemptID] = count
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return monitorSnapshotData{}, internal(err)
		}
		rows.Close()

		rows, err = tx.QueryContext(ctx, `
			SELECT aa.attempt_id, COUNT(*)
			FROM attempt_answers aa
			JOIN questions qq ON qq.id = aa.question_id AND qq.deleted_at = ?
			WHERE aa.attempt_id IN (`+list+`) AND aa.deleted_at = ?
			  AND (`+essayAnsweredSQL+` OR `+objectiveAnsweredSQL+`)
			GROUP BY aa.attempt_id`,
			store.LiveAt, store.LiveAt,
		)
		if err != nil {
			return monitorSnapshotData{}, internal(err)
		}
		for rows.Next() {
			var attemptID int64
			var count int
			if err := rows.Scan(&attemptID, &count); err != nil {
				rows.Close()
				return monitorSnapshotData{}, internal(err)
			}
			hits[attemptID] = count
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return monitorSnapshotData{}, internal(err)
		}
		rows.Close()
	}

	attempts := map[int64]*attemptRow{}
	for i := range selected {
		attempts[selected[i].registration] = &selected[i]
	}

	snap.Rows = make([]participantSnapshot, 0, len(registrations))
	for i := range registrations {
		reg := registrations[i]
		row := participantSnapshot{
			RegistrationID: reg.id,
			Name:           reg.name,
			Initials:       viewmodel.Initials(reg.name),
			ClassLabel:     viewmodel.ClassLabel(reg.grade, reg.major),
			State:          viewmodel.ParticipationRegistered,
			Total:          snap.QuestionCount,
			LastSeen:       reg.createdAt,
			Anonymous:      reg.anonymous,
		}
		if att := attempts[reg.id]; att != nil {
			id := att.id
			row.AttemptID = &id
			row.Answered = hits[att.id]
			row.FocusLoss = focus[att.id]
			row.LastSeen = att.updatedAt
			row.StartedAt = att.createdAt
			if att.finalScore.Valid {
				score := viewmodel.NewScoreFromRatio(att.finalScore.Float64)
				row.Score = &score
			}
			if att.submittedAt.Valid {
				row.State = viewmodel.ParticipationCompleted
			} else {
				row.State = viewmodel.ParticipationInProgress
			}
		}
		snap.Rows = append(snap.Rows, row)
	}

	sort.SliceStable(snap.Rows, func(i, j int) bool {
		ri, rj := participationRank(snap.Rows[i].State), participationRank(snap.Rows[j].State)
		if ri != rj {
			return ri < rj
		}
		left, right := strings.ToLower(snap.Rows[i].Name), strings.ToLower(snap.Rows[j].Name)
		if left != right {
			return left < right
		}
		return snap.Rows[i].RegistrationID < snap.Rows[j].RegistrationID
	})

	snap.Progress = viewmodel.MonitorProgress{
		Registered: len(snap.Rows),
		Total:      len(snap.Rows) * snap.QuestionCount,
	}
	for i := range snap.Rows {
		switch snap.Rows[i].State {
		case viewmodel.ParticipationInProgress:
			snap.Progress.InProgress++
		case viewmodel.ParticipationCompleted:
			snap.Progress.Completed++
		}
		snap.Progress.Answered += snap.Rows[i].Answered
	}
	return snap, nil
}

func participationRank(state viewmodel.ParticipationState) int {
	switch state {
	case viewmodel.ParticipationInProgress:
		return 0
	case viewmodel.ParticipationCompleted:
		return 1
	default:
		return 2
	}
}

// ---- Grading queue --------------------------------------------------------

// GradingQueueInput is GET /teacher/grading material. StateRaw selects the
// pending queue (default) or the graded history tab.
type GradingQueueInput struct {
	AccountID int64
	StateRaw  string
	QuizRaw   string
	ClassRaw  string
	QueryRaw  string
	PageRaw   string
}

// GradingDetailInput is GET /teacher/grading/:answerID material. The queue is
// rendered with the same filters as the list screen (ATT-211).
type GradingDetailInput struct {
	GradingQueueInput
	AnswerID int64
}

type gradingItem struct {
	answerID    int64
	attemptID   int64
	questionID  int64
	studentName string
	classLabel  string
	grade       int
	quizID      int64
	quizTitle   string
	prompt      string
	feedback    sql.NullString
	submittedAt time.Time
	score       sql.NullFloat64
	gradedAt    sql.NullTime
	updatedAt   time.Time
	href        string
}

// gradingState is the queue tab. Only the exact value "graded" leaves the
// pending default; a typo or an old link never 422s (ATT-240).
func gradingState(raw string) string {
	if strings.TrimSpace(raw) == "graded" {
		return "graded"
	}
	return "pending"
}

// GradingQueue implements the queue read (grading-history spec §2): the pending
// tab is the ATT-201…ATT-206 screen, the graded tab is the assessment history.
// Counts ignore the filters (ATT-203); the rows apply them before pagination.
func (s *Service) GradingQueue(ctx context.Context, in GradingQueueInput) (viewmodel.GradingPage, error) {
	var out viewmodel.GradingPage
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		queue, _, _, err := buildGradingQueue(ctx, tx, in)
		if err != nil {
			return err
		}
		back, err := gradingBackHref(ctx, tx, in)
		if err != nil {
			return err
		}
		out = viewmodel.GradingPage{Queue: queue, BackHref: back}
		return nil
	})
	if err != nil {
		return viewmodel.GradingPage{}, err
	}
	return out, nil
}

// GradingDetail implements the detail read (ATT-211…ATT-214, grading-history
// spec §3): the queue, the selected essay, and the attempt context in one
// read-only transaction. A graded answer is editable by the owner.
func (s *Service) GradingDetail(ctx context.Context, in GradingDetailInput) (viewmodel.GradingPage, error) {
	var out viewmodel.GradingPage
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		queue, all, values, err := buildGradingQueue(ctx, tx, in.GradingQueueInput)
		if err != nil {
			return err
		}
		detailHref := hrefWith(gradingAnswerPath(in.AnswerID), values)
		queueHref := hrefWith("/teacher/grading", values)
		detail, err := loadGradingDetail(ctx, tx, in.AccountID, in.AnswerID, all, detailHref, queueHref)
		if err != nil {
			return err
		}
		back, err := gradingBackHref(ctx, tx, in.GradingQueueInput)
		if err != nil {
			return err
		}
		out = viewmodel.GradingPage{Queue: queue, Selected: detail, BackHref: back}
		return nil
	})
	if err != nil {
		return viewmodel.GradingPage{}, err
	}
	return out, nil
}

// buildGradingQueue loads the active tab's essays of the teacher's quizzes once,
// derives the counts and the filtered page, and returns the full ordered list
// plus the normalized query so the detail route can build links without a
// second read.
func buildGradingQueue(ctx context.Context, tx *sql.Tx, in GradingQueueInput) (viewmodel.GradingQueue, []gradingItem, url.Values, error) {
	state := gradingState(in.StateRaw)
	graded := state == "graded"

	pendingCount, err := countGradingItems(ctx, tx, in.AccountID, false)
	if err != nil {
		return viewmodel.GradingQueue{}, nil, nil, err
	}
	gradedCount, err := countGradingItems(ctx, tx, in.AccountID, true)
	if err != nil {
		return viewmodel.GradingQueue{}, nil, nil, err
	}
	all, err := loadGradingItems(ctx, tx, in.AccountID, graded)
	if err != nil {
		return viewmodel.GradingQueue{}, nil, nil, err
	}

	now, err := nowT(ctx, tx)
	if err != nil {
		return viewmodel.GradingQueue{}, nil, nil, err
	}
	gradedToday, err := countGradedToday(ctx, tx, in.AccountID, now)
	if err != nil {
		return viewmodel.GradingQueue{}, nil, nil, err
	}

	search := normalizeSearch(in.QueryRaw)
	class, classOn := parseRomanClass(in.ClassRaw)
	quizID, quizOn := parsePositiveID(in.QuizRaw)
	values := gradingValues(state, search, class, classOn, quizID, quizOn)

	// Every item carries its own detail link so rows and the next link keep the
	// active tab and filters.
	for i := range all {
		all[i].href = hrefWith(gradingAnswerPath(all[i].answerID), values)
	}

	filtered := make([]gradingItem, 0, len(all))
	for i := range all {
		item := all[i]
		if quizOn && item.quizID != quizID {
			continue
		}
		if classOn && item.grade != class {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(item.studentName + "\n" + item.quizTitle + "\n" + item.prompt)
			if graded {
				haystack += "\n" + strings.ToLower(item.feedback.String)
			}
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		filtered = append(filtered, item)
	}

	rows := make([]viewmodel.GradingRow, 0, len(filtered))
	nowLocal := viewmodel.NewLocalTime(now)
	for i := range filtered {
		item := filtered[i]
		submitted := viewmodel.NewLocalTime(item.submittedAt)
		row := viewmodel.GradingRow{
			AnswerID:        item.answerID,
			StudentName:     item.studentName,
			ClassLabel:      item.classLabel,
			QuizTitle:       item.quizTitle,
			QuestionExcerpt: excerpt(item.prompt),
			WaitingSince:    submitted,
			Href:            item.href,
		}
		if graded {
			units := ScoreUnits(item.score.Float64)
			score := viewmodel.Score{Percent: DisplayPercent(units)}
			row.Score = &score
			row.ScoreLabel = TenthsValue(ScoreTenths(units))
			if item.gradedAt.Valid {
				gradedAt := viewmodel.NewLocalTime(item.gradedAt.Time)
				row.GradedLabel = "Dinilai " + gradedAt.Short()
				if item.updatedAt.After(item.gradedAt.Time) {
					row.EditedLabel = "Diubah " + viewmodel.NewLocalTime(item.updatedAt).Short()
				}
			}
		} else {
			row.WaitingLabel = waitingLabel(submitted, nowLocal)
		}
		rows = append(rows, row)
	}

	list := viewmodel.NewList(rows, parsePage(in.PageRaw), gradingPerPage)
	queue := viewmodel.GradingQueue{
		Counts: viewmodel.GradingCounts{
			Pending:     pendingCount,
			GradedToday: gradedToday,
			Graded:      gradedCount,
		},
		Rows:       list,
		Pagination: viewmodel.NewPagination("/teacher/grading", values, list.Page, list.PerPage, list.Total),
		Tabs:       gradingTabs(state, search, class, classOn, quizID, quizOn, pendingCount, gradedCount),
		Query:      viewmodel.QueryState{Q: search, Tab: state},
	}
	reset := hrefWith("/teacher/grading", gradingValues(state, "", 0, false, 0, false))
	switch {
	case state == "graded" && gradedCount == 0:
		queue.Empty = viewmodel.EmptyState{
			Title: "Belum ada esai yang dinilai.",
			Body:  "Esai yang sudah dinilai akan muncul di sini.",
			Icon:  "grade",
		}
	case state == "pending" && pendingCount == 0:
		queue.Empty = viewmodel.EmptyState{
			Title: "Tidak ada esai yang menunggu penilaian.",
			Body:  "Esai yang dikirim siswa akan muncul di sini.",
			Icon:  "check",
		}
	case len(filtered) == 0:
		queue.Empty = viewmodel.EmptyState{
			Title:  "Tidak ada esai yang cocok dengan filter.",
			Body:   "Coba kata kunci lain atau tampilkan semua esai.",
			Icon:   "search",
			Action: actionPtr(viewmodel.Outline("Tampilkan semua", reset)),
		}
	}
	return queue, all, values, nil
}

// gradingTabs is the pending/history switch. Both links keep the active
// filters; the counts ignore them (ATT-203).
func gradingTabs(state, search string, class int, classOn bool, quizID int64, quizOn bool, pending, graded int) []viewmodel.FilterTab {
	return []viewmodel.FilterTab{
		{
			Label:  "Menunggu",
			Href:   hrefWith("/teacher/grading", gradingValues("pending", search, class, classOn, quizID, quizOn)),
			Active: state == "pending",
			Count:  pending,
		},
		{
			Label:  "Sudah dinilai",
			Href:   hrefWith("/teacher/grading", gradingValues("graded", search, class, classOn, quizID, quizOn)),
			Active: state == "graded",
			Count:  graded,
		},
	}
}

// gradingBackHref is the scoped breadcrumb on the grading screen. A ?quiz=
// filter that resolves to a live quiz the acting teacher owns has a results page
// as its parent screen; the unscoped queue is a top-level nav destination and
// gets no back link (DESIGN §7).
//
// Ownership reuses loadOwnedQuiz, the owner/deleted predicate loadGradingItems
// applies in its own query, so the breadcrumb can never point at a quiz the
// queue itself filtered out. An unowned or missing id is simply no breadcrumb:
// the queue keeps its existing 200-with-an-empty-list behaviour rather than
// turning a filter into a 404.
func gradingBackHref(ctx context.Context, tx *sql.Tx, in GradingQueueInput) (string, error) {
	quizID, quizOn := parsePositiveID(in.QuizRaw)
	if !quizOn {
		return "", nil
	}
	quiz, err := loadOwnedQuiz(ctx, tx, quizID, in.AccountID)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	// A draft has no results page (Results 404s it), so it cannot be a parent.
	if quiz.State == viewmodel.QuizDraft {
		return "", nil
	}
	return resultsPath(quiz.ID), nil
}

// gradingJoin is the shared ownership/identity join. The bound parameters are
// the eight live sentinels, the owner account, and the answer sentinel, in that
// order (gradingArgs).
const gradingJoin = `
		FROM attempt_answers aa
		JOIN quiz_attempts att ON att.id = aa.attempt_id AND att.deleted_at = ?
		JOIN registrations r ON r.id = att.registration_id AND r.deleted_at = ?
		JOIN quizzes z ON z.id = r.quiz_id AND z.deleted_at = ? AND z.owner_account_id = ?
		JOIN questions qq ON qq.id = aa.question_id AND qq.deleted_at = ? AND qq.type = 'essay'
		LEFT JOIN students s ON s.id = r.student_id AND s.deleted_at = ?
		LEFT JOIN accounts a ON a.id = s.account_id AND a.deleted_at = ?
		LEFT JOIN anonymous_participants ap ON ap.id = r.anonymous_participant_id AND ap.deleted_at = ?
		LEFT JOIN majors mj ON mj.id = COALESCE(s.major_id, ap.major_id) AND mj.deleted_at = ?
		WHERE aa.deleted_at = ? AND att.submitted_at IS NOT NULL AND `

// gradingPredicate is the one place pending and graded diverge.
func gradingPredicate(graded bool) string {
	if graded {
		return "aa.score IS NOT NULL AND aa.graded_at IS NOT NULL"
	}
	return "aa.score IS NULL AND aa.graded_at IS NULL"
}

func gradingArgs(accountID int64) []any {
	return []any{
		store.LiveAt, store.LiveAt, store.LiveAt, accountID, store.LiveAt,
		store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt,
	}
}

// loadGradingItems loads every essay of the teacher's quizzes in the active
// state once. Pending keeps the submission order; graded is the assessment
// history, most recently assessed or edited first.
func loadGradingItems(ctx context.Context, tx *sql.Tx, accountID int64, graded bool) ([]gradingItem, error) {
	order := "att.submitted_at DESC, aa.id ASC"
	if graded {
		order = "aa.updated_at DESC, aa.id ASC"
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT aa.id, aa.attempt_id, aa.question_id,
		       COALESCE(a.full_name, ap.display_name),
		       COALESCE(COALESCE(s.grade_level, ap.grade_level), 0), mj.name,
		       z.id, z.title, qq.prompt, aa.feedback, att.submitted_at,
		       aa.score, aa.graded_at, aa.updated_at`+gradingJoin+
		gradingPredicate(graded)+`
		ORDER BY `+order,
		gradingArgs(accountID)...,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	out := make([]gradingItem, 0)
	for rows.Next() {
		var (
			item gradingItem
			name sql.NullString
			mj   sql.NullString
		)
		if err := rows.Scan(&item.answerID, &item.attemptID, &item.questionID,
			&name, &item.grade, &mj, &item.quizID, &item.quizTitle,
			&item.prompt, &item.feedback, &item.submittedAt,
			&item.score, &item.gradedAt, &item.updatedAt); err != nil {
			return nil, internal(err)
		}
		if !name.Valid {
			return nil, integrity("answer %d has no readable name (ATT-282)", item.answerID)
		}
		item.studentName = name.String
		item.classLabel = viewmodel.ClassLabel(item.grade, mj.String)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// countGradingItems counts one state across every owned quiz, ignoring the
// queue filters (ATT-203).
func countGradingItems(ctx context.Context, tx *sql.Tx, accountID int64, graded bool) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*)`+gradingJoin+gradingPredicate(graded),
		gradingArgs(accountID)...,
	).Scan(&count)
	if err != nil {
		return 0, internal(err)
	}
	return count, nil
}

func countGradedToday(ctx context.Context, tx *sql.Tx, accountID int64, now time.Time) (int, error) {
	start := startOfWIBDay(now)
	var count int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM attempt_answers
		WHERE graded_by_account_id = ? AND graded_at >= ? AND graded_at < ?
		  AND deleted_at = ?`,
		accountID, start, start.Add(24*time.Hour), store.LiveAt,
	).Scan(&count)
	if err != nil {
		return 0, internal(err)
	}
	return count, nil
}

// startOfWIBDay converts a UTC instant to the half-open start of its WIB
// calendar day, back in UTC for the database comparison (ATT-203).
func startOfWIBDay(now time.Time) time.Time {
	local := now.In(viewmodel.WIB)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, viewmodel.WIB).UTC()
}

// gradingValues is the normalized query shared by pagination, tabs and detail
// links. Pending is the default and is omitted; `page` is added only by
// NewPagination.
func gradingValues(state, search string, class int, classOn bool, quizID int64, quizOn bool) url.Values {
	values := url.Values{}
	if state == "graded" {
		values.Set("state", state)
	}
	if search != "" {
		values.Set("q", search)
	}
	if classOn {
		values.Set("class", viewmodel.RomanGrade(class))
	}
	if quizOn {
		values.Set("quiz", strconv.FormatInt(quizID, 10))
	}
	return values
}

// hrefWith appends a non-empty query to a path.
func hrefWith(base string, values url.Values) string {
	if enc := values.Encode(); enc != "" {
		return base + "?" + enc
	}
	return base
}

// loadGradingDetail loads one essay plus everything the attempt context needs.
// Objective answers and in-progress attempts are not gradable resources and
// answer 404 (ATT-211). The prompt, image, saved answer, and every context row
// come from loadResultData, so the detail and the recap share one derivation.
// active is the current tab's list, used for the next link.
func loadGradingDetail(ctx context.Context, tx *sql.Tx, accountID, answerID int64, active []gradingItem, detailHref, queueHref string) (*viewmodel.GradingDetail, error) {
	var (
		attemptID   int64
		questionID  int64
		attemptNo   int
		seed        uint32
		submittedAt time.Time
		createdAt   time.Time
		quizID      int64
		quizTitle   string
		allowRetake bool
		maxAttempts int
		randomQ     bool
		randomO     bool
		regLive     int64
		name        sql.NullString
		grade       sql.NullInt64
		major       sql.NullString
		gradedAt    sql.NullTime
		updatedAt   time.Time
		version     int
	)
	err := tx.QueryRowContext(ctx, `
		SELECT aa.attempt_id, aa.question_id,
		       att.attempt_no, att.shuffle_seed, att.submitted_at, att.created_at,
		       z.id, z.title, z.allow_retake, z.max_attempts,
		       z.randomize_questions, z.randomize_options,
		       r.deleted_at = ?,
		       COALESCE(a.full_name, ap.display_name),
		       COALESCE(COALESCE(s.grade_level, ap.grade_level), 0), mj.name,
		       aa.graded_at, aa.updated_at, aa.grading_version
		FROM attempt_answers aa
		JOIN quiz_attempts att ON att.id = aa.attempt_id AND att.deleted_at = ?
		JOIN registrations r ON r.id = att.registration_id
		JOIN quizzes z ON z.id = r.quiz_id AND z.deleted_at = ? AND z.owner_account_id = ?
		JOIN questions qq ON qq.id = aa.question_id AND qq.deleted_at = ? AND qq.type = 'essay'
		LEFT JOIN students s ON s.id = r.student_id AND s.deleted_at = ?
		LEFT JOIN accounts a ON a.id = s.account_id AND a.deleted_at = ?
		LEFT JOIN anonymous_participants ap ON ap.id = r.anonymous_participant_id AND ap.deleted_at = ?
		LEFT JOIN majors mj ON mj.id = COALESCE(s.major_id, ap.major_id) AND mj.deleted_at = ?
		WHERE aa.id = ? AND aa.deleted_at = ? AND att.submitted_at IS NOT NULL`,
		store.LiveAt, store.LiveAt, store.LiveAt, accountID, store.LiveAt,
		store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt,
		answerID, store.LiveAt,
	).Scan(&attemptID, &questionID, &attemptNo, &seed, &submittedAt, &createdAt,
		&quizID, &quizTitle, &allowRetake, &maxAttempts, &randomQ, &randomO,
		&regLive, &name, &grade, &major, &gradedAt, &updatedAt, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, internal(err)
	}
	if regLive != 1 {
		return nil, integrity("attempt %d is live under a dead registration (ATT-281)", attemptID)
	}
	if !name.Valid {
		return nil, integrity("answer %d has no readable name (ATT-282)", answerID)
	}
	now, err := nowT(ctx, tx)
	if err != nil {
		return nil, err
	}

	h := attemptView{
		attemptID:          uint32(attemptID),
		seed:               seed,
		randomizeQuestions: randomQ,
		randomizeOptions:   randomO,
	}
	slots, opts, sels, err := loadResultData(ctx, tx, h)
	if err != nil {
		return nil, err
	}
	current := -1
	for i := range slots {
		if slots[i].questionID == questionID {
			current = i
			break
		}
	}
	if current < 0 {
		return nil, integrity("answer %d is not a live slot of attempt %d (ATT-254)", answerID, attemptID)
	}
	cur := slots[current]
	classLabel := viewmodel.ClassLabel(int(grade.Int64), major.String)
	submittedLocal := viewmodel.NewLocalTime(submittedAt)

	detail := &viewmodel.GradingDetail{
		Answer: viewmodel.EssayAnswer{
			ID:            answerID,
			QuestionID:    questionID,
			StudentName:   name.String,
			ClassLabel:    classLabel,
			QuizTitle:     quizTitle,
			QuestionBody:  cur.prompt,
			QuestionImage: questionImage(cur.imagePath, cur.imageAlt, cur.imageBytes),
			Text:          essayText(cur.answerText),
			SubmittedAt:   submittedLocal,
			WaitingLabel:  waitingLabel(submittedLocal, viewmodel.NewLocalTime(now)),
			State:         viewmodel.EssayUngraded,
		},
		Attempt: viewmodel.AttemptHeader{
			ID:          attemptID,
			QuizID:      quizID,
			QuizTitle:   quizTitle,
			StudentName: name.String,
			ClassLabel:  classLabel,
			StartedAt:   viewmodel.NewLocalTime(createdAt),
			AttemptNo:   attemptNo,
			MaxAttempts: EffectiveMaxAttempts(allowRetake, maxAttempts),
		},
		Action:   detailHref,
		BackHref: queueHref,
		NextHref: nextGradingHref(answerID, active),
	}
	if cur.score.Valid != gradedAt.Valid {
		return nil, integrity("answer %d has an inconsistent score/graded_at pair (ATT-254)", answerID)
	}
	if cur.score.Valid {
		units := ScoreUnits(cur.score.Float64)
		score := viewmodel.Score{Percent: DisplayPercent(units)}
		detail.Answer.Score = &score
		detail.Answer.State = viewmodel.EssayGraded
		detail.Answer.WaitingLabel = ""
		detail.Score = &score
		detail.ScoreInput = TenthsValue(ScoreTenths(units))
		detail.Answer.GradedAt = viewmodel.Ptr(viewmodel.NewLocalTime(gradedAt.Time))
		detail.Answer.UpdatedAt = viewmodel.Ptr(viewmodel.NewLocalTime(updatedAt))
		detail.Answer.Edited = updatedAt.After(gradedAt.Time)
		detail.Answer.Version = version
	}
	if cur.feedback.Valid {
		detail.Answer.Feedback = cur.feedback.String
		detail.Feedback = cur.feedback.String
	}

	detail.Context = make([]viewmodel.GradingContextRow, 0, len(slots)-1)
	for i := range slots {
		if i == current {
			continue
		}
		row := teacherRecapRow(slots[i],
			orderedOptions(opts[slots[i].questionID], seed, randomO),
			sels[slots[i].questionID], true, i+1)
		detail.Context = append(detail.Context, viewmodel.GradingContextRow{
			Position:      row.Position,
			Type:          row.Type,
			Body:          row.Body,
			StudentAnswer: row.StudentAnswer,
			Key:           row.Key,
			Score:         row.Score,
			Verdict:       row.Verdict,
		})
	}
	return detail, nil
}

// essayText is the stored essay body or "" for a blank/NULL answer.
func essayText(text sql.NullString) string {
	if !text.Valid {
		return ""
	}
	return text.String
}

// nextGradingHref is the "Lewati"/"Berikutnya" target: the next answer in the
// active tab's order, wrapping to its head, or "" when the tab has no other
// row. Each item already carries the active filters in its href.
func nextGradingHref(answerID int64, active []gradingItem) string {
	if len(active) == 0 {
		return ""
	}
	for i := range active {
		if active[i].answerID != answerID {
			continue
		}
		if len(active) < 2 {
			return ""
		}
		return active[(i+1)%len(active)].href
	}
	return active[0].href
}

// gradingAnswerPath is the direct grading detail URL.
func gradingAnswerPath(id int64) string {
	return "/teacher/grading/" + strconv.FormatInt(id, 10)
}

// ---- Grade write ----------------------------------------------------------

// GradeInput is POST /teacher/grading/:answerID material. AccountID is the
// acting teacher (ATT-226); VersionRaw is the optimistic-concurrency token the
// detail form rendered (grading-history spec §4).
type GradeInput struct {
	AccountID   int64
	AnswerID    int64
	ScoreRaw    string
	FeedbackRaw string
	VersionRaw  string
}

// parseGradeVersion reads the version field. An absent field is legal for a
// first grade (old link) but never for an edit; a malformed value is never
// legal. MySQL stores it as SMALLINT UNSIGNED.
func parseGradeVersion(raw string) (version int, present, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 65535 {
		return 0, true, false
	}
	return n, true, true
}

// GradeEssay implements ATT-221…ATT-228 plus the re-grade rules of the
// grading-history spec §4. Validation runs before any lock; the transaction
// then takes the attempt lock, then the answer lock, so two grades of one
// attempt serialize (ATT-270, ATT-275). The final score is recomputed from
// every live slot in the same transaction (ATT-223).
func (s *Service) GradeEssay(ctx context.Context, in GradeInput) error {
	units, feedback, err := validateGrade(in)
	if err != nil {
		return err
	}
	// OPS-102 excludes a grade from the one permitted deadlock retry, so this
	// command uses the non-retrying helper.
	return s.writeTxOnce(ctx, func(tx *sql.Tx) error {
		var attemptID int64
		err := tx.QueryRowContext(ctx, `
			SELECT attempt_id FROM attempt_answers
			WHERE id = ? AND deleted_at = ?`,
			in.AnswerID, store.LiveAt,
		).Scan(&attemptID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return internal(err)
		}

		// Lock only the attempt row; quiz and registration are read plainly so
		// the command never takes a second domain's lock (ATT-270).
		var (
			registrationID int64
			submittedAt    sql.NullTime
		)
		err = tx.QueryRowContext(ctx, `
			SELECT registration_id, submitted_at
			FROM quiz_attempts
			WHERE id = ? AND deleted_at = ?
			FOR UPDATE`,
			attemptID, store.LiveAt,
		).Scan(&registrationID, &submittedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return internal(err)
		}

		var (
			regLive  int64
			ownerID  int64
			quizLive int64
		)
		err = tx.QueryRowContext(ctx, `
			SELECT r.deleted_at = ?, z.owner_account_id, z.deleted_at = ?
			FROM registrations r
			JOIN quizzes z ON z.id = r.quiz_id
			WHERE r.id = ?`,
			store.LiveAt, store.LiveAt, registrationID,
		).Scan(&regLive, &ownerID, &quizLive)
		if errors.Is(err, sql.ErrNoRows) {
			return integrity("attempt %d has no registration (ATT-281)", attemptID)
		}
		if err != nil {
			return internal(err)
		}
		if regLive != 1 {
			return integrity("attempt %d is live under a dead registration %d (ATT-281)", attemptID, registrationID)
		}
		if quizLive != 1 || ownerID != in.AccountID {
			return ErrNotFound
		}
		if !submittedAt.Valid {
			// An in-progress attempt is not a gradable resource (ATT-221).
			return ErrNotFound
		}

		var (
			questionID int64
			score      sql.NullFloat64
			gradedAt   sql.NullTime
			version    int
		)
		err = tx.QueryRowContext(ctx, `
			SELECT question_id, score, graded_at, grading_version
			FROM attempt_answers
			WHERE id = ? AND attempt_id = ? AND deleted_at = ?
			FOR UPDATE`,
			in.AnswerID, attemptID, store.LiveAt,
		).Scan(&questionID, &score, &gradedAt, &version)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return internal(err)
		}

		// The resource must be a live essay before an edit is even a candidate:
		// an objective answer is not a gradable resource (ATT-221).
		var qtype string
		err = tx.QueryRowContext(ctx, `
			SELECT type FROM questions WHERE id = ? AND deleted_at = ?`,
			questionID, store.LiveAt,
		).Scan(&qtype)
		if errors.Is(err, sql.ErrNoRows) {
			return integrity("answer %d has no live question (ATT-254)", in.AnswerID)
		}
		if err != nil {
			return internal(err)
		}
		if qtype != "essay" {
			return ErrNotFound
		}
		if score.Valid != gradedAt.Valid {
			return integrity("answer %d has an inconsistent score/graded_at pair (ATT-254)", in.AnswerID)
		}

		// The version token is the stale-form guard. A first grade accepts an
		// absent field as 0 (a tab opened before the deploy); an edit always
		// requires the exact stored counter.
		submittedVersion, versionPresent, versionOK := parseGradeVersion(in.VersionRaw)
		if !versionOK {
			return &Conflict{Flash: FlashGradeConflict, Err: ErrGradeConflict}
		}
		graded := score.Valid
		if graded {
			if !versionPresent || submittedVersion != version {
				return &Conflict{Flash: FlashGradeConflict, Err: ErrGradeConflict}
			}
		} else if versionPresent && submittedVersion != 0 {
			return &Conflict{Flash: FlashGradeConflict, Err: ErrGradeConflict}
		}

		now, err := nowT(ctx, tx)
		if err != nil {
			return err
		}
		var feedbackArg any
		if feedback != nil {
			feedbackArg = *feedback
		}
		scoreArg := float64(units) / float64(scoreScale)
		var res sql.Result
		if graded {
			// Re-grade: graded_at and the first-grade instant are preserved;
			// updated_at records the edit and the version advances.
			res, err = tx.ExecContext(ctx, `
				UPDATE attempt_answers
				SET score = ?, feedback = ?, graded_by_account_id = ?,
				    grading_version = grading_version + 1
				WHERE id = ? AND score IS NOT NULL AND grading_version = ?
				  AND deleted_at = ?`,
				scoreArg, feedbackArg, in.AccountID,
				in.AnswerID, version, store.LiveAt,
			)
		} else {
			res, err = tx.ExecContext(ctx, `
				UPDATE attempt_answers
				SET score = ?, feedback = ?, graded_by_account_id = ?, graded_at = ?,
				    grading_version = grading_version + 1
				WHERE id = ? AND score IS NULL AND deleted_at = ?`,
				scoreArg, feedbackArg, in.AccountID, now,
				in.AnswerID, store.LiveAt,
			)
		}
		if err != nil {
			return internal(err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return internal(err)
		}
		if affected == 0 {
			return &Conflict{Flash: FlashGradeConflict, Err: ErrGradeConflict}
		}
		return recomputeFinal(ctx, tx, uint32(attemptID))
	})
}

// recomputeFinal implements ATT-143/ATT-223(4): read every live slot's stored
// score in the same transaction; only a complete set updates final_score. The
// attempt's updated_at is deliberately preserved so grading does not look like
// student activity on the monitor (ATT-182 last_seen).
func recomputeFinal(ctx context.Context, tx *sql.Tx, attemptID uint32) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT aa.score
		FROM attempt_answers aa
		JOIN questions qq ON qq.id = aa.question_id AND qq.deleted_at = ?
		WHERE aa.attempt_id = ? AND aa.deleted_at = ?
		ORDER BY aa.id`,
		store.LiveAt, attemptID, store.LiveAt,
	)
	if err != nil {
		return internal(err)
	}
	defer rows.Close()

	total := 0
	scored := make([]int, 0)
	for rows.Next() {
		var score sql.NullFloat64
		if err := rows.Scan(&score); err != nil {
			return internal(err)
		}
		total++
		if score.Valid {
			scored = append(scored, ScoreUnits(score.Float64))
		}
	}
	if err := rows.Err(); err != nil {
		return internal(err)
	}
	if total == 0 || len(scored) != total {
		return nil
	}
	units, ok := FinalScoreUnits(scored, total)
	if !ok {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE quiz_attempts SET final_score = ?, updated_at = updated_at
		WHERE id = ?`,
		float64(units)/float64(scoreScale), attemptID,
	); err != nil {
		return internal(err)
	}
	return nil
}

// validateGrade parses the two form values. The score grammar accepts one
// optional decimal separator and at most one digit after it (ATT-222), and the
// stored value is always 4-dp units, never 0–100.
func validateGrade(in GradeInput) (int, *string, error) {
	fields := map[string]string{}
	units, scoreErr := GradeScore(in.ScoreRaw)
	if scoreErr != nil {
		for k, v := range scoreErr.Fields() {
			fields[k] = v
		}
	}
	feedback, feedbackErr := GradeFeedback(in.FeedbackRaw)
	if feedbackErr != nil {
		for k, v := range feedbackErr.Fields() {
			fields[k] = v
		}
	}
	if len(fields) > 0 {
		return 0, nil, apperr.Validationf(fields).WithRule("ATT-222")
	}
	return units, feedback, nil
}

// GradeScore parses `score` into 4-dp units. Exported so the 422 re-render
// echoes the submitted value through the same grammar as the write.
func GradeScore(raw string) (int, *apperr.Error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, apperr.Field("score", FieldErrRequired)
	}
	if !validScoreGrammar(s) {
		return 0, apperr.Field("score", FieldErrRange)
	}
	intPart, fracPart := s, ""
	for i, r := range s {
		if r == ',' || r == '.' {
			intPart, fracPart = s[:i], s[i+1:]
			break
		}
	}
	whole, err := strconv.Atoi(intPart)
	if err != nil || whole < 0 || whole > 100 {
		return 0, apperr.Field("score", FieldErrRange)
	}
	frac := 0
	if fracPart != "" {
		frac = int(fracPart[0] - '0')
	}
	units := whole*100 + frac*10
	if units < 0 || units > scoreScale {
		return 0, apperr.Field("score", FieldErrRange)
	}
	return units, nil
}

// validScoreGrammar is ^[0-9]+([.,][0-9])?$ without pulling in a regexp: at
// least one leading digit, an optional single separator, and at most one digit
// after it. Signs, exponents, and thousands separators are rejected (ATT-228).
func validScoreGrammar(s string) bool {
	digits, separators := 0, 0
	decimals := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
			if separators > 0 {
				decimals++
			}
		case r == ',' || r == '.':
			if separators > 0 || digits == 0 {
				return false
			}
			separators++
		default:
			return false
		}
	}
	if digits == 0 || (separators > 0 && decimals != 1) {
		return false
	}
	return true
}

// GradeFeedback trims for emptiness and validates the 1000-character bound;
// a non-empty value is returned verbatim for storage (ATT-226).
func GradeFeedback(raw string) (*string, *apperr.Error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(raw) > 1000 {
		return nil, apperr.Field("feedback", FieldErrFeedback)
	}
	value := raw
	return &value, nil
}

// ---- Recap ----------------------------------------------------------------

// RecapInput is GET /teacher/recap/:attemptID material.
type RecapInput struct {
	AccountID int64
	AttemptID int64
}

// Recap implements ATT-231…ATT-236. The teacher always sees the key and the
// true score; review_mode is a student rule only (ATT-232, ATT-246).
func (s *Service) Recap(ctx context.Context, in RecapInput) (viewmodel.Recap, error) {
	var out viewmodel.Recap
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		h, err := loadRecapHeader(ctx, tx, in.AccountID, in.AttemptID)
		if err != nil {
			return err
		}
		slots, opts, sels, err := loadResultData(ctx, tx, h.attempt)
		if err != nil {
			return err
		}

		submitted := h.submittedAt.Valid
		recap := viewmodel.Recap{
			Attempt: viewmodel.RecapAttempt{
				ID:          int64(h.attempt.attemptID),
				StudentName: h.name,
				ClassLabel:  h.class,
				QuizTitle:   h.attempt.quizTitle,
				QuizID:      int64(h.attempt.quizID),
				AttemptNo:   h.attempt.attemptNo,
				MaxAttempts: h.attempt.effectiveMax,
				StartedAt:   viewmodel.Ptr(viewmodel.NewLocalTime(h.attempt.createdAt)),
				FocusLoss:   h.focus,
			},
			BackHref: resultsPath(int64(h.attempt.quizID)),
		}
		if submitted {
			recap.Attempt.SubmittedAt = ptrLocalTimeOf(h.submittedAt)
		}
		if h.attempt.final.Valid {
			score := viewmodel.Score{Percent: DisplayPercent(ScoreUnits(h.attempt.final.Float64))}
			recap.Score = &score
			recap.Attempt.Score = &score
		} else {
			// Unsubmitted or still waiting for an essay: the template's one
			// "waiting" branch is the closest frozen rendering (template delta:
			// ATT-232 requires "Sedang dikerjakan" for the unsubmitted case).
			recap.Waiting = true
			recap.Attempt.Waiting = true
		}

		recap.Answers = make([]viewmodel.RecapRow, 0, len(slots))
		for i := range slots {
			cur := slots[i]
			var ordered []optionRow
			if !cur.essay() {
				ordered = orderedOptions(opts[cur.questionID], h.attempt.seed, h.attempt.randomizeOptions)
			}
			recap.Answers = append(recap.Answers, teacherRecapRow(cur, ordered, sels[cur.questionID], submitted, i+1))
		}
		out = recap
		return nil
	})
	if err != nil {
		return viewmodel.Recap{}, err
	}
	return out, nil
}

type recapHeader struct {
	attempt     attemptView
	submittedAt sql.NullTime
	name        string
	class       string
	focus       int
}

// loadRecapHeader resolves the attempt, its registration, the quiz owner, and
// the participant label in one read. A dead registration is a 404 only when
// its attempts were cascaded (ATT-235); a live attempt under a dead
// registration is the ATT-281 integrity signal.
func loadRecapHeader(ctx context.Context, tx *sql.Tx, accountID, attemptID int64) (recapHeader, error) {
	var (
		h        recapHeader
		regLive  int64
		fullName sql.NullString
		grade    sql.NullInt64
		major    sql.NullString
	)
	a := &h.attempt
	err := tx.QueryRowContext(ctx, `
		SELECT att.id, r.quiz_id, att.attempt_no, att.shuffle_seed, att.registration_id,
		       att.submitted_at, att.final_score, att.created_at,
		       z.title, z.state, z.allow_retake, z.max_attempts, z.show_score,
		       z.review_mode, z.randomize_questions, z.randomize_options,
		       r.deleted_at = ?,
		       COALESCE(a.full_name, ap.display_name),
		       COALESCE(COALESCE(s.grade_level, ap.grade_level), 0), mj.name,
		       (SELECT COUNT(*) FROM focus_loss_events f
		         WHERE f.attempt_id = att.id AND f.deleted_at = ?) AS focus_loss
		FROM quiz_attempts att
		JOIN registrations r ON r.id = att.registration_id
		JOIN quizzes z ON z.id = r.quiz_id AND z.deleted_at = ? AND z.owner_account_id = ?
		LEFT JOIN students s ON s.id = r.student_id AND s.deleted_at = ?
		LEFT JOIN accounts a ON a.id = s.account_id AND a.deleted_at = ?
		LEFT JOIN anonymous_participants ap ON ap.id = r.anonymous_participant_id AND ap.deleted_at = ?
		LEFT JOIN majors mj ON mj.id = COALESCE(s.major_id, ap.major_id) AND mj.deleted_at = ?
		WHERE att.id = ? AND att.deleted_at = ?`,
		store.LiveAt, store.LiveAt, store.LiveAt, accountID,
		store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt,
		attemptID, store.LiveAt,
	).Scan(&a.attemptID, &a.quizID, &a.attemptNo, &a.seed, &a.registrationID,
		&a.submittedAt, &a.final, &a.createdAt,
		&a.quizTitle, &a.state, &a.allowRetake, &a.maxAttempts, &a.showScore,
		&a.reviewMode, &a.randomizeQuestions, &a.randomizeOptions,
		&regLive, &fullName, &grade, &major, &h.focus)
	if errors.Is(err, sql.ErrNoRows) {
		return recapHeader{}, ErrNotFound
	}
	if err != nil {
		return recapHeader{}, internal(err)
	}
	if regLive != 1 {
		return recapHeader{}, integrity("attempt %d is live under a dead registration %d (ATT-281)", attemptID, a.registrationID)
	}
	if !fullName.Valid {
		return recapHeader{}, integrity("attempt %d has no readable name (ATT-282)", attemptID)
	}
	a.submitted = a.submittedAt.Valid
	a.effectiveMax = EffectiveMaxAttempts(a.allowRetake, a.maxAttempts)
	h.submittedAt = a.submittedAt
	h.name = fullName.String
	h.class = viewmodel.ClassLabel(int(grade.Int64), major.String)
	return h, nil
}

// teacherRecapRow derives one ATT-233 row. The teacher sees the truth: the key
// is unconditional, objective answers score 100/0 once submitted, essays show
// their stored score once graded, and everything else is "—".
func teacherRecapRow(cur takeSlot, ordered []optionRow, selected []int64, submitted bool, position int) viewmodel.RecapRow {
	row := viewmodel.RecapRow{
		Position: position,
		Type:     cur.typ,
		Body:     cur.prompt,
		Essay:    cur.essay(),
	}
	if cur.essay() {
		row.Key = "—"
		if cur.answerText.Valid && strings.TrimSpace(cur.answerText.String) != "" {
			row.StudentAnswer = cur.answerText.String
		} else {
			row.StudentAnswer = "—"
		}
		if cur.feedback.Valid {
			row.Feedback = cur.feedback.String
		}
		if !submitted {
			row.Verdict = "—"
			return row
		}
		if cur.score.Valid {
			score := viewmodel.Score{Percent: DisplayPercent(ScoreUnits(cur.score.Float64))}
			row.Score = &score
			row.Graded = true
			row.Verdict = "Sudah dinilai"
		} else {
			row.Verdict = "Menunggu nilai"
		}
		return row
	}

	set := map[int64]bool{}
	for _, id := range selected {
		set[id] = true
	}
	row.StudentAnswer = bodiesOf(ordered, set)
	if row.StudentAnswer == "" {
		row.StudentAnswer = "—"
	}
	row.Key = correctKeyText(cur, ordered)
	if row.Key == "" {
		row.Key = "—"
	}
	if !submitted {
		row.Verdict = "—"
		return row
	}
	row.Graded = true
	correct := map[int64]bool{}
	for _, o := range ordered {
		if o.isCorrect {
			correct[o.id] = true
		}
	}
	if len(selected) == 0 {
		zero := viewmodel.Score{Percent: 0}
		row.Score = &zero
		row.Verdict = "Tidak dijawab"
		return row
	}
	if objectiveScore(string(cur.typ), selected, correct) {
		full := viewmodel.Score{Percent: 100}
		row.Score = &full
		row.Verdict = "Benar"
	} else {
		zero := viewmodel.Score{Percent: 0}
		row.Score = &zero
		row.Verdict = "Salah"
	}
	return row
}

// ---- Small helpers --------------------------------------------------------

func quizPath(id int64) string { return "/teacher/quizzes/" + strconv.FormatInt(id, 10) }

func resultsPath(id int64) string { return quizPath(id) + "/results" }

func parsePage(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func parseRomanClass(raw string) (int, bool) {
	switch raw {
	case "X":
		return 10, true
	case "XI":
		return 11, true
	case "XII":
		return 12, true
	}
	return 0, false
}

func parsePositiveID(raw string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// normalizeSearch applies the shared list search rule: trimmed, lowercased,
// capped at 100 runes (ATT-172, ATT-240).
func normalizeSearch(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	runes := []rune(s)
	if len(runes) > 100 {
		s = string(runes[:100])
	}
	return s
}

func excerpt(s string) string {
	runes := []rune(s)
	if len(runes) <= excerptRunes {
		return s
	}
	return string(runes[:excerptRunes]) + "…"
}

// waitingLabel implements ATT-205: "Baru dikirim" for the freshest rows and
// "Menunggu <duration>" otherwise.
func waitingLabel(submitted, now viewmodel.LocalTime) string {
	rel := submitted.RelativeShort(now)
	if rel == "baru saja" {
		return "Baru dikirim"
	}
	return "Menunggu " + rel
}

func ptrLocalTime(t sql.NullTime) *viewmodel.LocalTime {
	if !t.Valid {
		return nil
	}
	v := viewmodel.NewLocalTime(t.Time)
	return &v
}

func scorePtr(s viewmodel.Score) *viewmodel.Score { return &s }

func actionPtr(a viewmodel.CardAction) *viewmodel.CardAction { return &a }
