package fixtures

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"quiz-v2/internal/viewmodel"
)

// This file builds the teacher dashboard, quiz list, results and live-monitor
// models from the single demo world. Every number and row here is derived from
// world.go so the screens agree with each other.

const (
	quizListPageSize = 6
	resultsPageSize  = 10
)

// TeacherDashboardData is /teacher. Top scores follow the world's quiz order
// and stop at five; the recent panel shows the five newest quizzes; the
// participation panels are the three highest and three lowest joined counts.
func TeacherDashboardData() viewmodel.TeacherDashboard {
	w := Default()

	top := make([]viewmodel.QuizTopScore, 0, 5)
	for _, q := range w.Quizzes {
		if len(top) == 5 {
			break
		}
		if score := w.TopScore(q.ID); score != nil {
			top = append(top, *score)
		}
	}

	recentIDs := []int64{QuizGeografi, QuizIPATerpadu, QuizMatematika, QuizKimia, QuizBahasa}
	recent := make([]viewmodel.RecentQuiz, 0, len(recentIDs))
	for _, id := range recentIDs {
		q := w.Quiz(id)
		if q == nil {
			continue
		}
		recent = append(recent, viewmodel.RecentQuiz{
			ID:            q.ID,
			Title:         q.Title,
			State:         q.State,
			CreatedAt:     q.CreatedAt,
			QuestionCount: w.QuestionCount(q.ID),
			Joined:        w.JoinedCount(q.ID),
			Href:          quizPath(q.ID),
		})
	}

	joined := make([]viewmodel.QuizParticipation, 0, len(w.Quizzes))
	for _, q := range w.Quizzes {
		joined = append(joined, viewmodel.QuizParticipation{
			Quiz:   viewmodel.QuizRef{ID: q.ID, Title: q.Title},
			Joined: w.JoinedCount(q.ID),
			Href:   quizPath(q.ID),
		})
	}

	most := append([]viewmodel.QuizParticipation(nil), joined...)
	sort.SliceStable(most, func(i, j int) bool {
		if most[i].Joined != most[j].Joined {
			return most[i].Joined > most[j].Joined
		}
		return most[i].Quiz.Title < most[j].Quiz.Title
	})
	if len(most) > 3 {
		most = most[:3]
	}

	least := append([]viewmodel.QuizParticipation(nil), joined...)
	sort.SliceStable(least, func(i, j int) bool {
		if least[i].Joined != least[j].Joined {
			return least[i].Joined < least[j].Joined
		}
		return least[i].Quiz.Title < least[j].Quiz.Title
	})
	if len(least) > 3 {
		least = least[:3]
	}

	return viewmodel.TeacherDashboard{
		GreetingName: "Bu Ratna",
		DateLine:     viewmodel.LocalDate(2026, time.October, 23, 8, 0).Long(),
		TopScores:    top,
		Recent:       recent,
		MostJoined:   most,
		LeastJoined:  least,
	}
}

// TeacherQuizListData is /teacher/quizzes. state is all|draft|open|closed, q
// filters the title and sort is the closed SortOrder set. page is 1-based.
func TeacherQuizListData(state, query, order string, page int) viewmodel.TeacherQuizList {
	w := Default()
	tab := normalizeQuizTab(state)
	sortOrder := normalizeSortOrder(order)

	counts := map[viewmodel.QuizState]int{}
	for _, q := range w.Quizzes {
		counts[q.State]++
	}

	query = strings.TrimSpace(query)
	lowerQuery := strings.ToLower(query)

	items := make([]viewmodel.QuizCard, 0, len(w.Quizzes))
	for _, q := range w.Quizzes {
		if !quizTabMatches(tab, q.State) {
			continue
		}
		if lowerQuery != "" && !strings.Contains(strings.ToLower(q.Title), lowerQuery) {
			continue
		}
		items = append(items, teacherQuizCard(w, w.Quiz(q.ID)))
	}

	sort.SliceStable(items, func(i, j int) bool {
		left, right := w.Quiz(items[i].ID), w.Quiz(items[j].ID)
		if left == nil || right == nil {
			return items[i].ID < items[j].ID
		}
		switch sortOrder {
		case viewmodel.SortOldest:
			return left.CreatedAt.Time().Before(right.CreatedAt.Time())
		case viewmodel.SortNameAsc:
			return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
		case viewmodel.SortNameDesc:
			return strings.ToLower(items[i].Title) > strings.ToLower(items[j].Title)
		default:
			return left.CreatedAt.Time().After(right.CreatedAt.Time())
		}
	})

	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if tab != "all" {
		values.Set("state", tab)
	}
	if sortOrder != viewmodel.SortNewest {
		values.Set("sort", string(sortOrder))
	}

	list := viewmodel.NewList(items, page, quizListPageSize)
	pagination := viewmodel.NewPagination("/teacher/quizzes", values, list.Page, quizListPageSize, list.Total)

	empty := viewmodel.EmptyState{
		Title:  "Belum ada kuis",
		Body:   "Kuis yang Anda buat akan muncul di sini. Mulai dari satu draf, lalu tambahkan soal.",
		Icon:   "quiz",
		Action: actionPtr(viewmodel.Primary("Buat kuis", "/teacher/quizzes/new")),
	}
	switch {
	case query != "":
		empty = viewmodel.EmptyState{
			Title:  "Tidak ada kuis yang cocok",
			Body:   "Coba kata kunci lain atau hapus saringan yang aktif.",
			Icon:   "search",
			Action: actionPtr(viewmodel.Outline("Hapus pencarian", "/teacher/quizzes")),
		}
	case tab != "all":
		empty = viewmodel.EmptyState{
			Title:  "Belum ada kuis di tab ini",
			Body:   "Kuis dengan status ini akan muncul di sini.",
			Icon:   "quiz",
			Action: actionPtr(viewmodel.Outline("Lihat semua kuis", "/teacher/quizzes")),
		}
	}

	return viewmodel.TeacherQuizList{
		Quizzes:    list,
		Pagination: pagination,
		Tabs: []viewmodel.FilterTab{
			{Label: "Semua", Href: quizListHref("all", query, sortOrder), Active: tab == "all", Count: len(w.Quizzes)},
			{Label: "Draf", Href: quizListHref("draft", query, sortOrder), Active: tab == "draft", Count: counts[viewmodel.QuizDraft]},
			{Label: "Dibuka", Href: quizListHref("open", query, sortOrder), Active: tab == "open", Count: counts[viewmodel.QuizOpen]},
			{Label: "Ditutup", Href: quizListHref("closed", query, sortOrder), Active: tab == "closed", Count: counts[viewmodel.QuizClosed]},
		},
		Query:      viewmodel.QueryState{Q: query, Tab: tab, Sort: sortOrder},
		Empty:      empty,
		CreateHref: "/teacher/quizzes/new",
	}
}

// QuizResultsData is /teacher/quizzes/:id/results. One row per attempt, newest
// submitted attempt first; state is all|in_progress|completed.
func QuizResultsData(quizID int64, state, query string, page int) viewmodel.QuizResults {
	w := Default()
	q := w.Quiz(quizID)

	tab := normalizeResultsTab(state)
	query = strings.TrimSpace(query)
	lowerQuery := strings.ToLower(query)

	var (
		rows       []viewmodel.ResultsRow
		submitted  int
		inProgress int
		scoreSum   int
		scoreCount int
	)

	for _, r := range w.RegistrationsFor(quizID) {
		for _, a := range r.Attempts {
			if a.SubmittedAt == nil {
				inProgress++
			} else {
				submitted++
				if a.Final != nil {
					scoreSum += a.Final.Percent
					scoreCount++
				}
			}
			rows = append(rows, viewmodel.ResultsRow{
				AttemptID:   a.ID,
				StudentName: w.DisplayName(r),
				ClassLabel:  w.RegistrationClass(r),
				AttemptNo:   a.Number,
				MaxAttempts: q.MaxAttempts,
				SubmittedAt: a.SubmittedAt,
				Score:       a.Final,
				Waiting:     a.Waiting,
				FocusLoss:   a.FocusLoss,
				RecapHref:   viewmodel.RecapHref(a.ID),
			})
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		left, right := w.Attempt(rows[i].AttemptID), w.Attempt(rows[j].AttemptID)
		if left == nil || right == nil {
			return rows[i].AttemptID > rows[j].AttemptID
		}
		// Submitted attempts first, newest submission first; in-progress
		// attempts follow, newest start first.
		switch {
		case left.SubmittedAt != nil && right.SubmittedAt == nil:
			return true
		case left.SubmittedAt == nil && right.SubmittedAt != nil:
			return false
		case left.SubmittedAt != nil && right.SubmittedAt != nil:
			if !left.SubmittedAt.Time().Equal(right.SubmittedAt.Time()) {
				return left.SubmittedAt.Time().After(right.SubmittedAt.Time())
			}
		default:
			if !left.StartedAt.Time().Equal(right.StartedAt.Time()) {
				return left.StartedAt.Time().After(right.StartedAt.Time())
			}
		}
		return rows[i].StudentName < rows[j].StudentName
	})

	filtered := make([]viewmodel.ResultsRow, 0, len(rows))
	for _, row := range rows {
		attempt := w.Attempt(row.AttemptID)
		switch tab {
		case "in_progress":
			if attempt == nil || attempt.SubmittedAt != nil {
				continue
			}
		case "completed":
			if attempt == nil || attempt.SubmittedAt == nil {
				continue
			}
		}
		if lowerQuery != "" && !strings.Contains(strings.ToLower(row.StudentName), lowerQuery) {
			continue
		}
		filtered = append(filtered, row)
	}

	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if tab != "all" {
		values.Set("state", tab)
	}

	list := viewmodel.NewList(filtered, page, resultsPageSize)
	base := resultsPath(quizID)
	pagination := viewmodel.NewPagination(base, values, list.Page, resultsPageSize, list.Total)

	var average *viewmodel.Average
	if scoreCount > 0 {
		avg := viewmodel.NewAverage(float64(scoreSum) / float64(scoreCount))
		average = &avg
	}

	empty := viewmodel.EmptyState{
		Title:  "Belum ada percobaan",
		Body:   "Percobaan siswa akan muncul di sini setelah mereka mulai mengerjakan.",
		Icon:   "clipboard",
		Action: actionPtr(viewmodel.Primary("Buka detail kuis", quizPath(quizID))),
	}
	if query != "" || tab != "all" {
		empty = viewmodel.EmptyState{
			Title:  "Tidak ada percobaan yang cocok",
			Body:   "Coba kata kunci lain atau tampilkan semua percobaan.",
			Icon:   "search",
			Action: actionPtr(viewmodel.Outline("Tampilkan semua", base)),
		}
	}

	result := viewmodel.QuizResults{
		Quiz:       w.QuizRef(quizID),
		Rows:       list,
		Pagination: pagination,
		Tabs: []viewmodel.FilterTab{
			{Label: "Semua", Href: resultsHref(quizID, "all", query), Active: tab == "all", Count: submitted + inProgress},
			{Label: "Sedang berlangsung", Href: resultsHref(quizID, "in_progress", query), Active: tab == "in_progress", Count: inProgress},
			{Label: "Selesai", Href: resultsHref(quizID, "completed", query), Active: tab == "completed", Count: submitted},
		},
		Query:      viewmodel.QueryState{Q: query, Tab: tab},
		Empty:      empty,
		BackHref:   quizPath(quizID),
		ReviewMode: viewmodel.ReviewQuestionsOnly,
		Counts: viewmodel.ResultsCounts{
			Submitted:  submitted,
			InProgress: inProgress,
			Average:    average,
		},
	}
	if q != nil {
		result.ReviewMode = q.ReviewMode
	}
	return result
}

// MonitorData is /teacher/quizzes/:id/monitor. A quiz that is not open renders
// the "not on air" empty state; an open quiz renders one row per registration
// with its current attempt.
func MonitorData(quizID int64) viewmodel.MonitorPage {
	w := Default()
	q := w.Quiz(quizID)

	monitor := viewmodel.MonitorPage{
		Quiz:        w.QuizRef(quizID),
		QuizHref:    quizPath(quizID),
		ResultsHref: resultsPath(quizID),
		GradingHref: "/teacher/grading",
		GeneratedAt: viewmodel.LocalDate(2026, time.October, 23, 9, 42),
	}

	if q == nil || q.State != viewmodel.QuizOpen {
		monitor.Empty = viewmodel.EmptyState{
			Title:  "Kuis tidak sedang berlangsung",
			Body:   "Monitor hanya tersedia selama kuis dibuka. Setelah ditutup, gunakan halaman hasil untuk melihat rekap.",
			Icon:   "clock",
			Action: actionPtr(viewmodel.Primary("Kembali ke kuis", quizPath(quizID))),
		}
		return monitor
	}

	totalQuestions := w.QuestionCount(quizID)
	registrations := w.RegistrationsFor(quizID)
	rows := make([]viewmodel.ParticipantRow, 0, len(registrations))

	var inProgress, completed, answered int
	for _, r := range registrations {
		var current *Attempt
		for _, a := range r.Attempts {
			if a.SubmittedAt == nil {
				if current == nil || a.Number > current.Number {
					current = a
				}
			}
		}
		if current == nil {
			for _, a := range r.Attempts {
				if current == nil || a.Number > current.Number {
					current = a
				}
			}
		}

		state := viewmodel.ParticipationRegistered
		answeredForRow := 0
		var score *viewmodel.Score
		focusLoss := 0
		var startedAt, updatedAt *viewmodel.LocalTime

		if current != nil {
			startedAt = viewmodel.Ptr(current.StartedAt)
			updatedAt = viewmodel.Ptr(current.StartedAt)
			focusLoss = current.FocusLoss
			if current.SubmittedAt == nil {
				state = viewmodel.ParticipationInProgress
				inProgress++
				answeredForRow = len(current.Answers)
				if answeredForRow > totalQuestions {
					answeredForRow = totalQuestions
				}
			} else {
				state = viewmodel.ParticipationCompleted
				completed++
				answeredForRow = totalQuestions
				score = current.Final
			}
		}

		answered += answeredForRow
		rows = append(rows, viewmodel.ParticipantRow{
			RegistrationID: r.ID,
			AttemptID:      attemptID(current),
			StudentName:    w.DisplayName(r),
			Initials:       viewmodel.Initials(w.DisplayName(r)),
			ClassLabel:     w.RegistrationClass(r),
			State:          state,
			Answered:       answeredForRow,
			Total:          totalQuestions,
			Score:          score,
			FocusLoss:      focusLoss,
			StartedAt:      startedAt,
			UpdatedAt:      updatedAt,
			Anonymous:      r.Anonymous,
		})
	}

	// In-progress first, then completed, then not started; name breaks ties.
	rank := func(state viewmodel.ParticipationState) int {
		switch state {
		case viewmodel.ParticipationInProgress:
			return 0
		case viewmodel.ParticipationCompleted:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rank(rows[i].State) != rank(rows[j].State) {
			return rank(rows[i].State) < rank(rows[j].State)
		}
		return rows[i].StudentName < rows[j].StudentName
	})

	monitor.StreamHref = quizPath(quizID) + "/stream"
	monitor.Rows = rows
	monitor.Progress = viewmodel.MonitorProgress{
		Registered: len(registrations),
		InProgress: inProgress,
		Completed:  completed,
		Answered:   answered,
		Total:      len(registrations) * totalQuestions,
	}
	monitor.Empty = viewmodel.EmptyState{
		Title:  "Belum ada peserta",
		Body:   "Peserta yang bergabung akan muncul di sini secara langsung.",
		Icon:   "users",
		Action: actionPtr(viewmodel.Outline("Kembali ke kuis", quizPath(quizID))),
	}
	return monitor
}

// teacherQuizCard is the list's viewer-dependent card. The template adds the
// per-state action forms; these two actions cover the primary navigation.
func teacherQuizCard(w *World, q *Quiz) viewmodel.QuizCard {
	card := w.Card(q)
	card.Href = quizPath(q.ID)
	switch q.State {
	case viewmodel.QuizDraft:
		card.Primary = viewmodel.Primary("Kelola", card.Href)
	case viewmodel.QuizOpen:
		card.Primary = viewmodel.Primary("Monitor", monitorPath(q.ID))
		secondary := viewmodel.Outline("Hasil", resultsPath(q.ID))
		card.Secondary = &secondary
	case viewmodel.QuizClosed:
		card.Primary = viewmodel.Primary("Hasil", resultsPath(q.ID))
	}
	return card
}

func quizPath(id int64) string {
	return "/teacher/quizzes/" + strconv.FormatInt(id, 10)
}

func resultsPath(id int64) string {
	return quizPath(id) + "/results"
}

func monitorPath(id int64) string {
	return quizPath(id) + "/monitor"
}

func attemptID(a *Attempt) *int64 {
	if a == nil {
		return nil
	}
	return &a.ID
}

func actionPtr(a viewmodel.CardAction) *viewmodel.CardAction { return &a }

func normalizeQuizTab(state string) string {
	switch state {
	case "draft", "open", "closed":
		return state
	default:
		return "all"
	}
}

func normalizeResultsTab(state string) string {
	switch state {
	case "in_progress", "completed":
		return state
	default:
		return "all"
	}
}

func normalizeSortOrder(order string) viewmodel.SortOrder {
	switch viewmodel.SortOrder(order) {
	case viewmodel.SortOldest, viewmodel.SortNameAsc, viewmodel.SortNameDesc:
		return viewmodel.SortOrder(order)
	default:
		return viewmodel.SortNewest
	}
}

func quizTabMatches(tab string, state viewmodel.QuizState) bool {
	switch tab {
	case "draft":
		return state == viewmodel.QuizDraft
	case "open":
		return state == viewmodel.QuizOpen
	case "closed":
		return state == viewmodel.QuizClosed
	default:
		return true
	}
}

func quizListHref(state, query string, order viewmodel.SortOrder) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if state != "" && state != "all" {
		values.Set("state", state)
	}
	if order != "" && order != viewmodel.SortNewest {
		values.Set("sort", string(order))
	}
	return withQuery("/teacher/quizzes", values)
}

func resultsHref(quizID int64, state, query string) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if state != "" && state != "all" {
		values.Set("state", state)
	}
	return withQuery(resultsPath(quizID), values)
}

func withQuery(base string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return base + "?" + encoded
	}
	return base
}
