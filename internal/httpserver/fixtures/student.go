// This file is the student page fixture set. Every screen in the student area
// reads the one demo world and derives its numbers from it, so the dashboard,
// history and quiz intro never disagree with each other.
package fixtures

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"quiz-v2/internal/viewmodel"
)

// studentDemoNow is the fixed WIB "now" behind greeting and date copy. The
// demo is a snapshot, so the clock must not drift between renders.
var studentDemoNow = viewmodel.LocalDate(2026, time.October, 23, 9, 42)

// StudentDashboard builds /student: ready-to-start, live, newly available and
// recent-result sections plus the four-number stat strip.
func StudentDashboard() viewmodel.StudentDashboard {
	w := Default()
	student := w.Student(StudentAyu)
	out := viewmodel.StudentDashboard{
		GreetingName: studentFirstName(w.StudentName(StudentAyu)),
		DateLine:     studentDemoNow.Long(),
	}

	for i := range w.Quizzes {
		q := &w.Quizzes[i]
		if !w.Eligible(q, student) {
			continue
		}
		participation, reg := w.ParticipationOf(q.ID, student.ID)
		if q.State == viewmodel.QuizOpen {
			out.Stats.Assigned++
		}
		if reg == nil {
			if q.State == viewmodel.QuizOpen {
				card := studentQuizCard(q, "", nil)
				card.Primary = viewmodel.Primary("Lihat kuis", card.Href)
				out.Latest = append(out.Latest, card)
			}
			continue
		}
		if q.State == viewmodel.QuizOpen && len(reg.Attempts) == 0 {
			out.Stats.Registered++
			card := studentQuizCard(q, participation, reg)
			card.Primary = viewmodel.Primary("Lihat kuis", card.Href)
			out.Ready = append(out.Ready, card)
		}
		if participation == viewmodel.ParticipationInProgress {
			out.Stats.InProgress++
			card := studentQuizCard(q, participation, reg)
			if id := studentInProgressAttempt(studentAttemptRows(reg, q.ShowScore)); id != 0 {
				card.Primary = viewmodel.Primary("Lanjutkan", studentAttemptHref(id))
			} else {
				card.Primary = viewmodel.Primary("Lihat kuis", card.Href)
			}
			out.Live = append(out.Live, card)
		}
		if LastSubmitted(reg) != nil {
			out.Stats.Completed++
		}
		if q.ShowScore {
			if best := BestScore(reg); best != nil && (out.Stats.BestScore == nil || best.Percent > out.Stats.BestScore.Percent) {
				out.Stats.BestScore = best
			}
		}
	}

	out.Recent = studentRecentResults(w, 5)
	out.Empty = studentDashboardEmpty()
	return out
}

// StudentHistory builds /student/history: one collapsible row per quiz the
// student has joined, newest activity first, each with its attempts.
func StudentHistory() viewmodel.HistoryPage {
	w := Default()

	type entry struct {
		quiz *Quiz
		reg  *Registration
		last time.Time
	}
	var entries []entry
	for _, r := range w.Registrations {
		if r.StudentID != StudentAyu {
			continue
		}
		q := w.Quiz(r.QuizID)
		if q == nil {
			continue
		}
		last := r.JoinedAt.Time()
		for _, a := range r.Attempts {
			at := a.StartedAt.Time()
			if a.SubmittedAt != nil && a.SubmittedAt.Time().After(at) {
				at = a.SubmittedAt.Time()
			}
			if at.After(last) {
				last = at
			}
		}
		entries = append(entries, entry{quiz: q, reg: r, last: last})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].last.After(entries[j].last) })

	rows := make([]viewmodel.HistoryRow, 0, len(entries))
	for _, e := range entries {
		participation, _ := w.ParticipationOf(e.quiz.ID, StudentAyu)
		row := viewmodel.HistoryRow{
			Quiz:        w.QuizRef(e.quiz.ID),
			TeacherName: w.TeacherName(e.quiz.TeacherID),
			State:       participation,
			Attempts:    studentAttemptRows(e.reg, e.quiz.ShowScore),
			ShowScore:   e.quiz.ShowScore,
			DetailHref:  studentHistoryHref(e.quiz.ID),
		}
		if e.quiz.ShowScore {
			row.BestScore = BestScore(e.reg)
		}
		if last := LastSubmitted(e.reg); last != nil {
			row.LastAttempt = last.SubmittedAt
		}
		rows = append(rows, row)
	}

	list := viewmodel.NewList(rows, 1, 10)
	return viewmodel.HistoryPage{
		Rows:       list,
		Pagination: viewmodel.NewPagination("/student/history", url.Values{}, list.Page, list.PerPage, list.Total),
		Empty:      studentHistoryEmpty(),
	}
}

// StudentHistoryDetail builds /student/history/:quizID. An unknown id falls
// back to the student's first registration so the screen always renders.
func StudentHistoryDetail(quizID int64) viewmodel.HistoryDetail {
	w := Default()
	var reg *Registration
	for _, r := range w.Registrations {
		if r.StudentID != StudentAyu {
			continue
		}
		if reg == nil {
			reg = r
		}
		if r.QuizID == quizID {
			reg = r
			break
		}
	}
	if reg == nil {
		return viewmodel.HistoryDetail{BackHref: "/student/history"}
	}

	q := w.Quiz(reg.QuizID)
	participation, _ := w.ParticipationOf(q.ID, StudentAyu)
	attempts := studentAttemptRows(reg, q.ShowScore)
	return viewmodel.HistoryDetail{
		Quiz:        w.QuizRef(q.ID),
		TeacherName: w.TeacherName(q.TeacherID),
		Audience:    w.Audience(q),
		State:       participation,
		Attempts:    attempts,
		MaxAttempts: q.MaxAttempts,
		CanRetry: q.State == viewmodel.QuizOpen &&
			q.AllowRetake &&
			participation != viewmodel.ParticipationInProgress &&
			len(attempts) < q.MaxAttempts,
		RetryHref: studentQuizHref(q.ID),
		BackHref:  "/student/history",
	}
}

// StudentProfile builds /profile from Ayu's account row.
func StudentProfile() viewmodel.ProfilePage {
	w := Default()
	s := w.Student(StudentAyu)
	majors := make([]viewmodel.MajorRef, 0, len(w.Majors))
	for _, m := range w.Majors {
		majors = append(majors, viewmodel.MajorRef{ID: m.ID, Name: m.Name})
	}
	return viewmodel.ProfilePage{
		FullName:   s.FullName,
		Email:      s.Email,
		GradeLevel: s.GradeLevel,
		MajorID:    s.MajorID,
		Majors:     majors,
	}
}

// StudentQuizIntro builds /quiz/:id for the signed-in student. Unknown,
// draft or out-of-audience quizzes fall back to the demo's flagship quiz so a
// hand-edited id never renders an empty screen.
func StudentQuizIntro(quizID int64) viewmodel.QuizIntro {
	return studentQuizIntro(quizID, "")
}

// StudentQuizIntroState applies one of the demo-only ?state= previews:
// "anonymous" (public join note), "exhausted" (attempt limit reached) and
// "wrong-audience" (private quiz for another class).
func StudentQuizIntroState(quizID int64, state string) viewmodel.QuizIntro {
	return studentQuizIntro(quizID, state)
}

func studentQuizIntro(quizID int64, state string) viewmodel.QuizIntro {
	w := Default()
	student := w.Student(StudentAyu)
	wrongAudience := state == "wrong-audience"
	q := w.Quiz(quizID)
	if state == "exhausted" {
		q = w.Quiz(QuizBiologi)
	}
	if q == nil && wrongAudience {
		q = w.Quiz(QuizBahasa)
	}
	if q == nil || q.State == viewmodel.QuizDraft || (!w.Eligible(q, student) && !wrongAudience) {
		q = w.Quiz(QuizBiologi)
	}

	participation, reg := w.ParticipationOf(q.ID, student.ID)
	attempts := studentAttemptRows(reg, q.ShowScore)
	if state == "exhausted" && len(attempts) > 0 {
		for i := range attempts {
			if attempts[i].State != viewmodel.AttemptInProgress {
				continue
			}
			attempts[i].State = viewmodel.AttemptCompleted
			attempts[i].Waiting = true
			attempts[i].SubmittedAt = viewmodel.Ptr(studentDemoNow)
			attempts[i].Score = nil
			attempts[i].ResultHref = viewmodel.ResultHref(attempts[i].ID)
		}
		participation = viewmodel.ParticipationCompleted
	}

	eligible := w.Eligible(q, student)
	inProgressID := studentInProgressAttempt(attempts)
	submitted := studentHasSubmitted(attempts)
	canRegister := reg == nil && q.State == viewmodel.QuizOpen && eligible
	canCancel := reg != nil && len(attempts) == 0 && q.State == viewmodel.QuizOpen
	canStart := reg != nil &&
		q.State == viewmodel.QuizOpen &&
		eligible &&
		inProgressID == 0 &&
		(q.MaxAttempts <= 0 || len(attempts) < q.MaxAttempts) &&
		(!submitted || q.AllowRetake)

	var primary, secondary *viewmodel.CardAction
	switch {
	case canRegister:
		a := viewmodel.Primary("Daftar kuis", studentQuizHref(q.ID)+"/register")
		primary = &a
	case canStart:
		label := "Mulai kuis"
		if submitted {
			label = "Coba lagi"
		}
		a := viewmodel.Primary(label, studentQuizHref(q.ID)+"/attempts")
		primary = &a
	case inProgressID != 0:
		a := viewmodel.Primary("Lanjutkan percobaan", studentAttemptHref(inProgressID))
		primary = &a
	default:
		if last, ok := studentLastSubmitted(attempts); ok {
			a := viewmodel.Primary("Lihat hasil terakhir", last.ResultHref)
			primary = &a
		}
	}
	switch {
	case canCancel:
		a := viewmodel.Danger("Batalkan pendaftaran", studentQuizHref(q.ID)+"/cancel")
		secondary = &a
	case submitted:
		a := viewmodel.Outline("Riwayat kuis", studentHistoryHref(q.ID))
		secondary = &a
	}

	var joinErr *viewmodel.JoinError
	if wrongAudience && !eligible {
		e := viewmodel.JoinWrongAudience
		joinErr = &e
	}

	card := studentQuizCard(q, participation, reg)
	if primary != nil {
		card.Primary = *primary
	}
	card.Secondary = secondary

	return viewmodel.QuizIntro{
		Quiz:          card,
		Description:   q.Description,
		StartsAt:      q.StartsAt,
		EndsAt:        q.EndsAt,
		Participation: participation,
		Attempts:      attempts,
		AttemptsUsed:  len(attempts),
		MaxAttempts:   q.MaxAttempts,
		CanRegister:   canRegister,
		CanCancel:     canCancel,
		CanStart:      canStart,
		CanRetry:      canStart && submitted,
		Primary:       primary,
		Secondary:     secondary,
		JoinedAt:      card.JoinedAt,
		Error:         joinErr,
		BackHref:      "/student",
	}
}

// ---- Helpers --------------------------------------------------------------

// studentQuizCard is the viewer-specific half of a quiz card for Ayu.
func studentQuizCard(q *Quiz, participation viewmodel.ParticipationState, reg *Registration) viewmodel.QuizCard {
	w := Default()
	card := w.Card(q)
	card.Href = studentQuizHref(q.ID)
	card.Participation = participation
	if reg != nil {
		card.AttemptsUsed = len(reg.Attempts)
		card.JoinedAt = viewmodel.Ptr(reg.JoinedAt)
	}
	return card
}

// studentAttemptRows copies the world rows and applies the quiz's score
// visibility plus resume links for attempts that are still open.
func studentAttemptRows(reg *Registration, showScore bool) []viewmodel.AttemptRow {
	if reg == nil {
		return nil
	}
	rows := Default().AttemptRows(reg)
	for i := range rows {
		if !showScore {
			rows[i].Score = nil
		}
		if rows[i].State == viewmodel.AttemptInProgress {
			rows[i].ResultHref = studentAttemptHref(rows[i].ID)
			rows[i].ReviewHref = rows[i].ResultHref
			continue
		}
		rows[i].ReviewHref = viewmodel.ResultHref(rows[i].ID)
	}
	return rows
}

func studentRecentResults(w *World, limit int) []viewmodel.RecentResult {
	type submitted struct {
		quiz    *Quiz
		attempt *Attempt
	}
	var items []submitted
	for _, r := range w.Registrations {
		if r.StudentID != StudentAyu {
			continue
		}
		q := w.Quiz(r.QuizID)
		if q == nil {
			continue
		}
		for _, a := range r.Attempts {
			if a.SubmittedAt == nil {
				continue
			}
			items = append(items, submitted{quiz: q, attempt: a})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].attempt.SubmittedAt.Time().After(items[j].attempt.SubmittedAt.Time())
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]viewmodel.RecentResult, 0, len(items))
	for _, item := range items {
		row := viewmodel.RecentResult{
			Quiz:        w.QuizRef(item.quiz.ID),
			AttemptID:   item.attempt.ID,
			ShowScore:   item.quiz.ShowScore,
			Waiting:     item.attempt.Waiting,
			SubmittedAt: item.attempt.SubmittedAt,
			Href:        viewmodel.ResultHref(item.attempt.ID),
		}
		if item.quiz.ShowScore {
			row.Score = item.attempt.Final
		}
		out = append(out, row)
	}
	return out
}

func studentInProgressAttempt(rows []viewmodel.AttemptRow) int64 {
	for _, row := range rows {
		if row.State == viewmodel.AttemptInProgress && row.ID != 0 {
			return row.ID
		}
	}
	return 0
}

func studentHasSubmitted(rows []viewmodel.AttemptRow) bool {
	for _, row := range rows {
		if row.State == viewmodel.AttemptCompleted {
			return true
		}
	}
	return false
}

func studentLastSubmitted(rows []viewmodel.AttemptRow) (viewmodel.AttemptRow, bool) {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].State == viewmodel.AttemptCompleted && rows[i].ID != 0 {
			return rows[i], true
		}
	}
	return viewmodel.AttemptRow{}, false
}

func studentFirstName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return name
	}
	return parts[0]
}

func studentQuizHref(quizID int64) string {
	return "/quiz/" + strconv.FormatInt(quizID, 10)
}

func studentAttemptHref(attemptID int64) string {
	return "/attempt/" + strconv.FormatInt(attemptID, 10)
}

func studentHistoryHref(quizID int64) string {
	return "/student/history/" + strconv.FormatInt(quizID, 10)
}

func studentDashboardEmpty() viewmodel.EmptyState {
	a := viewmodel.Primary("Gabung dengan kode", "/join")
	return viewmodel.EmptyState{
		Title:  "Belum ada kuis untuk Anda.",
		Body:   "Kuis baru akan muncul di sini setelah guru membukanya untuk kelas Anda.",
		Icon:   "quiz",
		Action: &a,
	}
}

func studentHistoryEmpty() viewmodel.EmptyState {
	a := viewmodel.Outline("Gabung kuis", "/join")
	return viewmodel.EmptyState{
		Title:  "Belum ada riwayat kuis.",
		Body:   "Kuis yang Anda ikuti akan tercatat di sini beserta setiap percobaannya.",
		Icon:   "history",
		Action: &a,
	}
}
