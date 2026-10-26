package participation

// studentread.go implements the PAR student read surfaces: the dashboard
// (PAR-126…PAR-141), the history list (PAR-142…PAR-153) and the history detail
// (PAR-154…PAR-162). Each page is built inside one read-only repeatable-read
// transaction (Service.readTx) so the stats, the lists, and the pagination all
// describe the same snapshot (§6.1.1, §6.2.5, §8.7.4).
//
// The page is projection only: no rule here writes, and a state is always
// derived from the live attempt timestamps (§4.1, PAR-INV-03). Scores follow
// PAR-147/PAR-196: a hidden or absent score is nil and renders "—", never 0.

import (
	"context"
	"database/sql"
	"net/url"
	"sort"
	"strings"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/viewmodel"
)

// Student dashboard caps and the history page size (PAR-134, PAR-135, PAR-145,
// R-21).
const (
	studentLatestCap      = 6
	studentRecentCap      = 5
	studentHistoryPerPage = 10
)

// StudentDashboardInput is the GET /student material. The actor carries the
// session's account/role/name; the domain re-resolves the live student profile
// inside the snapshot transaction.
type StudentDashboardInput struct {
	Actor ActorInput
}

// StudentHistoryInput is the GET /student/history material. Page is the raw
// ?page= value: non-numeric, zero or negative falls back to 1 and a page beyond
// the end clamps to the last page (PAR-145).
type StudentHistoryInput struct {
	Actor ActorInput
	Page  string
}

// StudentHistoryDetailInput is the GET /student/history/:quizID material. A
// non-positive QuizID is the 404 of PAR-154.
type StudentHistoryDetailInput struct {
	Actor  ActorInput
	QuizID int64
}

// studentSnapshot is one consistent read of the student's participation world.
// assigned is only loaded for the dashboard; coalescing all reads in one
// transaction is what keeps the five tiles and four lists in agreement.
type studentSnapshot struct {
	actor          Actor
	fullName       string
	assigned       []quizRow
	registrations  []registrationRow
	quizzes        map[int64]quizRow
	attempts       map[int64][]attemptRecord
	questionCounts map[int64]int
}

// StudentDashboard serves GET /student (PAR-126…PAR-141).
func (s *Service) StudentDashboard(ctx context.Context, in StudentDashboardInput) (viewmodel.StudentDashboard, error) {
	var out viewmodel.StudentDashboard
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		snap, err := s.loadStudentSnapshot(ctx, tx, in.Actor, "PAR-126", true)
		if err != nil {
			return err
		}
		out = s.buildStudentDashboard(snap)
		return nil
	})
	return out, err
}

// StudentHistory serves GET /student/history (PAR-142…PAR-153): one row per
// live registration, last activity descending, ten per page.
func (s *Service) StudentHistory(ctx context.Context, in StudentHistoryInput) (viewmodel.HistoryPage, error) {
	var out viewmodel.HistoryPage
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		snap, err := s.loadStudentSnapshot(ctx, tx, in.Actor, "PAR-142", false)
		if err != nil {
			return err
		}
		out = s.buildStudentHistory(snap, in.Page)
		return nil
	})
	return out, err
}

// StudentHistoryDetail serves GET /student/history/:quizID (PAR-154…PAR-162).
// A missing quiz, a missing live registration, or a foreign registration is the
// same 404 (PAR-155).
func (s *Service) StudentHistoryDetail(ctx context.Context, in StudentHistoryDetailInput) (viewmodel.HistoryDetail, error) {
	var out viewmodel.HistoryDetail
	if in.QuizID <= 0 {
		return out, apperr.NotFoundf().WithRule("PAR-154")
	}
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveActor(ctx, tx, in.Actor)
		if err != nil {
			return err
		}
		if !actor.IsStudent() {
			return apperr.NotFoundf().WithRule("PAR-154")
		}
		quiz, err := readQuizByID(ctx, tx, in.QuizID, "")
		if err != nil {
			return err
		}
		reg, hasReg, err := readStudentRegistration(ctx, tx, quiz.ID, int64(actor.StudentID))
		if err != nil {
			return err
		}
		if !hasReg {
			return apperr.NotFoundf().WithRule("PAR-155")
		}
		byReg, err := readAttemptsByRegistration(ctx, tx, []int64{reg.ID})
		if err != nil {
			return err
		}
		out = s.buildStudentHistoryDetail(actor, quiz, byReg[reg.ID])
		return nil
	})
	return out, err
}

// loadStudentSnapshot resolves the actor and loads every row the two list
// pages need in one snapshot. withAssigned adds the dashboard's assigned-open
// quizzes and question counts; the history list does not need them.
func (s *Service) loadStudentSnapshot(ctx context.Context, tx *sql.Tx, in ActorInput, rule string, withAssigned bool) (studentSnapshot, error) {
	snap := studentSnapshot{}
	actor, err := resolveActor(ctx, tx, in)
	if err != nil {
		return snap, err
	}
	if !actor.IsStudent() {
		return snap, apperr.NotFoundf().WithRule(rule)
	}
	snap.actor = actor
	snap.fullName = in.FullName

	registrations, err := readStudentRegistrations(ctx, tx, int64(actor.StudentID))
	if err != nil {
		return snap, err
	}
	snap.registrations = registrations

	quizIDs := make([]int64, 0, len(registrations))
	regIDs := make([]int64, 0, len(registrations))
	seenQuiz := make(map[int64]struct{}, len(registrations))
	for _, reg := range registrations {
		regIDs = append(regIDs, reg.ID)
		if _, ok := seenQuiz[reg.QuizID]; ok {
			continue
		}
		seenQuiz[reg.QuizID] = struct{}{}
		quizIDs = append(quizIDs, reg.QuizID)
	}
	quizzes, err := readQuizzesByIds(ctx, tx, quizIDs)
	if err != nil {
		return snap, err
	}
	attempts, err := readAttemptsByRegistration(ctx, tx, regIDs)
	if err != nil {
		return snap, err
	}
	snap.quizzes = quizzes
	snap.attempts = attempts

	if !withAssigned {
		return snap, nil
	}
	assigned, err := readAssignedQuizzes(ctx, tx, actor.GradeLevel, actor.MajorID)
	if err != nil {
		return snap, err
	}
	snap.assigned = assigned

	countIDs := append([]int64{}, quizIDs...)
	for _, quiz := range assigned {
		if _, ok := seenQuiz[quiz.ID]; ok {
			continue
		}
		seenQuiz[quiz.ID] = struct{}{}
		countIDs = append(countIDs, quiz.ID)
	}
	counts, err := liveQuestionCounts(ctx, tx, countIDs)
	if err != nil {
		return snap, err
	}
	snap.questionCounts = counts
	return snap, nil
}

// buildStudentDashboard projects one snapshot to PAR-126…PAR-141. Stats and
// lists are computed in a single pass over the live registrations so a
// registration can never be counted in one tile and missing from its list.
func (s *Service) buildStudentDashboard(snap studentSnapshot) viewmodel.StudentDashboard {
	out := viewmodel.StudentDashboard{
		GreetingName: greetingFirstName(snap.fullName),
		DateLine:     viewmodel.NewLocalTime(s.now()).Long(),
	}

	// A live quiz row is required for every registration read (§8.2.1): a
	// registration whose quiz was soft-deleted out of band disappears.
	regQuiz := make(map[int64]quizRow, len(snap.registrations))
	for _, reg := range snap.registrations {
		if quiz, ok := snap.quizzes[reg.QuizID]; ok {
			regQuiz[reg.ID] = quiz
		}
	}

	// PAR-128: distinct quiz ids in the union of assigned open quizzes and
	// live registrations, any quiz state.
	assigned := make(map[int64]struct{}, len(snap.assigned)+len(regQuiz))
	for _, quiz := range snap.assigned {
		assigned[quiz.ID] = struct{}{}
	}
	for _, quiz := range regQuiz {
		assigned[quiz.ID] = struct{}{}
	}
	out.Stats.Assigned = len(assigned)

	type readyEntry struct {
		quiz quizRow
		reg  registrationRow
	}
	type recentEntry struct {
		quiz    quizRow
		attempt attemptRecord
	}
	type liveEntry struct {
		quiz     quizRow
		reg      registrationRow
		attempt  attemptRecord
		attempts int
	}

	var (
		ready  []readyEntry
		recent []recentEntry
		live   []liveEntry
		best   *viewmodel.Score
	)

	for _, reg := range snap.registrations {
		quiz, ok := regQuiz[reg.ID]
		if !ok {
			continue
		}
		records := snap.attempts[reg.ID]
		state := deriveState(len(records), hasOpenAttempt(records))

		switch state {
		case viewmodel.ParticipationRegistered:
			// PAR-129: only an open quiz is actionable on the dashboard; a
			// closed zero-attempt registration stays in history only.
			if viewmodel.QuizState(quiz.State) == viewmodel.QuizOpen {
				out.Stats.Registered++
				ready = append(ready, readyEntry{quiz: quiz, reg: reg})
			}
		case viewmodel.ParticipationInProgress:
			out.Stats.InProgress++
		case viewmodel.ParticipationCompleted:
			out.Stats.Completed++
		}

		// PAR-131: hidden and NULL scores never contribute.
		if quiz.ShowScore {
			if score := s.bestVisibleScore(records); score != nil {
				if best == nil || score.Percent > best.Percent {
					best = score
				}
			}
		}

		// PAR-133: a live card per registration with an in-progress attempt.
		var open *attemptRecord
		for i := range records {
			a := &records[i]
			if a.Submitted.Valid {
				continue
			}
			if open == nil || a.UpdatedAt.After(open.UpdatedAt) ||
				(a.UpdatedAt.Equal(open.UpdatedAt) && a.ID < open.ID) {
				open = a
			}
		}
		if open != nil {
			live = append(live, liveEntry{quiz: quiz, reg: reg, attempt: *open, attempts: len(records)})
		}

		// PAR-135: submitted attempts across every live registration.
		for _, a := range records {
			if !a.Submitted.Valid {
				continue
			}
			recent = append(recent, recentEntry{quiz: quiz, attempt: a})
		}
	}
	out.Stats.BestScore = best

	// PAR-132: ready cards sort by registration time desc, tie id asc.
	sort.SliceStable(ready, func(i, j int) bool {
		if !ready[i].reg.CreatedAt.Equal(ready[j].reg.CreatedAt) {
			return ready[i].reg.CreatedAt.After(ready[j].reg.CreatedAt)
		}
		return ready[i].reg.ID < ready[j].reg.ID
	})
	for _, e := range ready {
		card := s.studentQuizCard(e.quiz, snap.questionCounts[e.quiz.ID])
		card.Participation = viewmodel.ParticipationRegistered
		card.AttemptsUsed = 0
		card.JoinedAt = localPtr(sql.NullTime{Time: e.reg.CreatedAt, Valid: true})
		// The intro owns the POST forms; CardAction cannot carry a method, so
		// the dashboard card links there instead of POSTing directly.
		card.Primary = viewmodel.Primary("Mulai kuis", card.Href)
		out.Ready = append(out.Ready, card)
	}

	// PAR-133: live cards sort by attempt.updated_at desc, tie attempt id asc.
	sort.SliceStable(live, func(i, j int) bool {
		if !live[i].attempt.UpdatedAt.Equal(live[j].attempt.UpdatedAt) {
			return live[i].attempt.UpdatedAt.After(live[j].attempt.UpdatedAt)
		}
		return live[i].attempt.ID < live[j].attempt.ID
	})
	for _, e := range live {
		card := s.studentQuizCard(e.quiz, snap.questionCounts[e.quiz.ID])
		card.Participation = viewmodel.ParticipationInProgress
		card.AttemptsUsed = e.attempts
		card.JoinedAt = localPtr(sql.NullTime{Time: e.reg.CreatedAt, Valid: true})
		card.Primary = viewmodel.Primary("Lanjutkan", attemptPath(uint32(e.attempt.ID)))
		out.Live = append(out.Live, card)
	}

	// PAR-134: open eligible quizzes with no live registration, published_at
	// desc (the loader already ordered them), cap six, zero-question excluded.
	registered := make(map[int64]struct{}, len(regQuiz))
	for _, quiz := range regQuiz {
		registered[quiz.ID] = struct{}{}
	}
	for _, quiz := range snap.assigned {
		if _, ok := registered[quiz.ID]; ok {
			continue
		}
		if snap.questionCounts[quiz.ID] == 0 {
			s.warn("PAR-134", "open quiz has no live questions; hidden from the latest list", nil)
			continue
		}
		if len(out.Latest) >= studentLatestCap {
			break
		}
		card := s.studentQuizCard(quiz, snap.questionCounts[quiz.ID])
		card.Primary = viewmodel.Primary("Lihat kuis", card.Href)
		out.Latest = append(out.Latest, card)
	}

	// PAR-135: five most recent submitted attempts, submitted_at desc, tie
	// attempt id asc.
	sort.SliceStable(recent, func(i, j int) bool {
		if !recent[i].attempt.Submitted.Time.Equal(recent[j].attempt.Submitted.Time) {
			return recent[i].attempt.Submitted.Time.After(recent[j].attempt.Submitted.Time)
		}
		return recent[i].attempt.ID < recent[j].attempt.ID
	})
	if len(recent) > studentRecentCap {
		recent = recent[:studentRecentCap]
	}
	for _, e := range recent {
		row := viewmodel.RecentResult{
			Quiz:        viewmodel.QuizRef{ID: e.quiz.ID, Title: e.quiz.Title},
			AttemptID:   e.attempt.ID,
			ShowScore:   e.quiz.ShowScore,
			Waiting:     !e.attempt.FinalScore.Valid,
			SubmittedAt: localPtr(e.attempt.Submitted),
			Href:        attemptResultPath(uint32(e.attempt.ID)),
		}
		if e.quiz.ShowScore {
			if score, ok := s.visibleScore(e.attempt.FinalScore); ok {
				row.Score = &score
			}
		}
		out.Recent = append(out.Recent, row)
	}

	// PAR-137: the empty state only when every list is empty and every count
	// is zero; dead closed registrations keep the stats visible instead.
	if len(out.Ready) == 0 && len(out.Live) == 0 && len(out.Latest) == 0 &&
		len(out.Recent) == 0 && out.Stats.Assigned == 0 && out.Stats.Registered == 0 &&
		out.Stats.InProgress == 0 && out.Stats.Completed == 0 {
		out.Empty = studentDashboardEmpty()
	}
	return out
}

// buildStudentHistory projects one snapshot to PAR-142…PAR-153.
func (s *Service) buildStudentHistory(snap studentSnapshot, rawPage string) viewmodel.HistoryPage {
	type historyEntry struct {
		quiz    quizRow
		reg     registrationRow
		records []attemptRecord
		last    time.Time
	}

	entries := make([]historyEntry, 0, len(snap.registrations))
	for _, reg := range snap.registrations {
		quiz, ok := snap.quizzes[reg.QuizID]
		if !ok {
			continue
		}
		records := snap.attempts[reg.ID]
		last := reg.CreatedAt
		for _, a := range records {
			if a.CreatedAt.After(last) {
				last = a.CreatedAt
			}
			if a.UpdatedAt.After(last) {
				last = a.UpdatedAt
			}
			if a.Submitted.Valid && a.Submitted.Time.After(last) {
				last = a.Submitted.Time
			}
		}
		entries = append(entries, historyEntry{quiz: quiz, reg: reg, records: records, last: last})
	}

	// PAR-144: last activity desc, tie registration id asc.
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].last.Equal(entries[j].last) {
			return entries[i].last.After(entries[j].last)
		}
		return entries[i].reg.ID < entries[j].reg.ID
	})

	rows := make([]viewmodel.HistoryRow, 0, len(entries))
	for _, e := range entries {
		row := viewmodel.HistoryRow{
			Quiz:        viewmodel.QuizRef{ID: e.quiz.ID, Title: e.quiz.Title},
			TeacherName: e.quiz.TeacherName,
			State:       deriveState(len(e.records), hasOpenAttempt(e.records)),
			Attempts:    s.buildAttemptRows(e.records, e.quiz.ShowScore),
			ShowScore:   e.quiz.ShowScore,
			DetailHref:  historyPath(uint32(e.quiz.ID)),
		}
		if e.quiz.ShowScore {
			row.BestScore = s.bestVisibleScore(e.records)
		}
		if last := lastSubmitted(e.records); last.Valid {
			row.LastAttempt = localPtr(last)
		}
		rows = append(rows, row)
	}

	// PAR-145: page size 10, bad or too-large pages clamp inside NewList.
	page := clampPage(rawPage, studentHistoryPerPage, len(rows))
	list := viewmodel.NewList(rows, page, studentHistoryPerPage)
	return viewmodel.HistoryPage{
		Rows:       list,
		Pagination: viewmodel.NewPagination("/student/history", url.Values{}, list.Page, list.PerPage, list.Total),
		Empty:      studentHistoryEmpty(),
	}
}

// buildStudentHistoryDetail projects one registration to PAR-154…PAR-162.
func (s *Service) buildStudentHistoryDetail(actor Actor, quiz quizRow, records []attemptRecord) viewmodel.HistoryDetail {
	limit, normalised := maxAttemptsFor(quiz)
	if normalised {
		s.warn("PAR-081", "quiz carries an invalid attempt quota; failing closed at one", nil)
	}
	open := viewmodel.QuizState(quiz.State) == viewmodel.QuizOpen
	hasOpen := hasOpenAttempt(records)
	attemptsUsed := len(records)

	// PAR-157: the detail's start action includes the first attempt. A
	// closed quiz, an open attempt, a spent quota, a no-retake row with an
	// attempt, or a profile that is no longer eligible disables it.
	canRetry := open && !hasOpen && attemptsUsed < limit &&
		(attemptsUsed == 0 || quiz.AllowRetake) &&
		eligible(quiz, actor.GradeLevel, actor.MajorID)

	attemptRows := s.buildAttemptRows(records, quiz.ShowScore)
	for i := range attemptRows {
		attemptRows[i].MaxAttempts = limit
	}

	detail := viewmodel.HistoryDetail{
		Quiz:        viewmodel.QuizRef{ID: quiz.ID, Title: quiz.Title},
		TeacherName: quiz.TeacherName,
		Audience:    audienceLabel(quiz),
		State:       deriveState(attemptsUsed, hasOpen),
		ShowScore:   quiz.ShowScore,
		Attempts:    attemptRows,
		MaxAttempts: limit,
		CanRetry:    canRetry,
		BackHref:    "/student/history",
	}
	// PAR-157/PAR-088: the quiz page is 404 for a student once the quiz
	// closes, so the detail page only offers it while the quiz is open. A
	// closed quiz leaves the per-attempt rows as the only destinations.
	if open {
		detail.RetryHref = quizPath(uint32(quiz.ID))
	}
	return detail
}

// studentQuizCard is the shared card base for the three dashboard lists. It
// carries the quiz identity, the teacher's byline (name plus photo-or-initials),
// current labels, schedule, and the normalized attempt cap; the caller adds
// participation and one primary action.
func (s *Service) studentQuizCard(quiz quizRow, questionCount int) viewmodel.QuizCard {
	limit, normalised := maxAttemptsFor(quiz)
	if normalised {
		s.warn("PAR-081", "quiz carries an invalid attempt quota; failing closed at one", nil)
	}
	card := viewmodel.QuizCard{
		ID:            quiz.ID,
		Href:          quizPath(uint32(quiz.ID)),
		Title:         quiz.Title,
		Description:   quiz.Description,
		TeacherName:   quiz.TeacherName,
		TeacherAvatar: viewmodel.NewAvatar(quiz.TeacherAvatarName, quiz.TeacherName, "app-avatar--sm"),
		QuestionCount: questionCount,
		State:         viewmodel.QuizState(quiz.State),
		Visibility:    viewmodel.Visibility(quiz.Visibility),
		Audience:      audienceLabel(quiz),
		StartsAt:      localPtr(quiz.StartsAt),
		EndsAt:        localPtr(quiz.EndsAt),
		MaxAttempts:   limit,
		ReviewMode:    viewmodel.ReviewMode(quiz.ReviewMode),
		ShowScore:     quiz.ShowScore,
	}
	// PAR-136: the code is a public-quiz affordance only.
	if viewmodel.Visibility(quiz.Visibility) == viewmodel.VisibilityPublic &&
		quiz.Code.Valid && quiz.Code.String != "" {
		code := quiz.Code.String
		card.Code = &code
	}
	return card
}

// bestVisibleScore is PAR-131/PAR-194: the maximum final score of the live
// attempts, ignoring NULL. Callers only pass records of a show_score quiz.
func (s *Service) bestVisibleScore(records []attemptRecord) *viewmodel.Score {
	var best *viewmodel.Score
	for _, a := range records {
		score, ok := s.visibleScore(a.FinalScore)
		if !ok {
			continue
		}
		if best == nil || score.Percent > best.Percent {
			best = &score
		}
	}
	return best
}

// visibleScore converts a stored 0-1 ratio to a whole percent (PAR-189). An
// out-of-range value is a data fault: it renders nil and logs, never 0 or 100
// (8.6.4).
func (s *Service) visibleScore(v sql.NullFloat64) (viewmodel.Score, bool) {
	if !v.Valid {
		return viewmodel.Score{}, false
	}
	if v.Float64 < 0 || v.Float64 > 1 {
		s.oops("PAR-189", "stored final_score is outside the 0-1 range", nil)
		return viewmodel.Score{}, false
	}
	return viewmodel.NewScoreFromRatio(v.Float64), true
}

// hasOpenAttempt reports whether any live attempt is still submitted_at NULL.
func hasOpenAttempt(records []attemptRecord) bool {
	for _, a := range records {
		if !a.Submitted.Valid {
			return true
		}
	}
	return false
}

// lastSubmitted is the newest submitted_at of one registration, or an invalid
// NullTime when every attempt is still open (PAR-146's last-attempt label).
func lastSubmitted(records []attemptRecord) sql.NullTime {
	last := sql.NullTime{}
	for _, a := range records {
		if !a.Submitted.Valid {
			continue
		}
		if !last.Valid || a.Submitted.Time.After(last.Time) {
			last = a.Submitted
		}
	}
	return last
}

// greetingFirstName is PAR-138: the greeting uses the first word of the full
// name, never a nickname or initials.
func greetingFirstName(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// studentDashboardEmpty is the PAR-137 copy, byte for byte.
func studentDashboardEmpty() viewmodel.EmptyState {
	action := viewmodel.Primary("Gabung dengan kode", "/join")
	return viewmodel.EmptyState{
		Title:  "Belum ada kuis untuk Anda.",
		Body:   "Kuis baru akan muncul di sini setelah guru membukanya untuk kelas Anda.",
		Icon:   "quiz",
		Action: &action,
	}
}

// studentHistoryEmpty is the PAR-151 copy, byte for byte.
func studentHistoryEmpty() viewmodel.EmptyState {
	action := viewmodel.Outline("Gabung kuis", "/join")
	return viewmodel.EmptyState{
		Title:  "Belum ada riwayat kuis.",
		Body:   "Kuis yang Anda ikuti akan tercatat di sini beserta setiap percobaannya.",
		Icon:   "history",
		Action: &action,
	}
}
