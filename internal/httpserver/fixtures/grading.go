package fixtures

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"quiz-v2/internal/viewmodel"
)

// Grading fixtures. The queue and the recap are projections of the one demo
// world: every row is a real essay answer attached to a real attempt, so the
// student, class, quiz title, timestamps and scores agree with the result and
// monitor screens. Nothing here mutates the world.

const (
	// suggestedEssayScore is the demo rubric hint shown beside the score field.
	suggestedEssayScore = 75

	// defaultRecapAttemptID is the recap shown when the route receives an
	// unknown attempt id.
	defaultRecapAttemptID int64 = 1001

	// gradingPerPage keeps the demo queue on one page while still exercising
	// the shared pagination partial.
	gradingPerPage = 10
)

// gradingNow is the fixed demo clock used for "Menunggu N jam" labels. It sits
// after every queued submission, so the labels are stable between renders.
var gradingNow = viewmodel.LocalDate(2026, time.October, 23, 10, 0)

// gradingItem joins one essay answer to the attempt, registration, quiz and
// question it belongs to. Building this once keeps the row and detail
// projections from drifting apart.
type gradingItem struct {
	attempt      *Attempt
	registration *Registration
	answer       *Answer
	question     *Question
	quiz         *Quiz
}

// GradingPage returns the queue/detail screen. selectedID is 0 for the queue
// route. An unknown selected id falls back to the first visible row; an empty
// queue leaves Selected nil and the template renders the placeholder.
//
// Supported query keys: q, state (pending|graded), quiz, class, page. The two
// selects named in the spec have no option/echo fields on the typed model, so
// quiz and class filtering is URL-supported but not rendered as controls.
func GradingPage(selectedID int64, query map[string]string) viewmodel.GradingPage {
	w := Default()
	all := gradingItems()
	pending := make([]gradingItem, 0, len(all))
	graded := make([]gradingItem, 0, len(all))
	for _, item := range all {
		if item.answer.Graded {
			graded = append(graded, item)
		} else {
			pending = append(pending, item)
		}
	}

	search := strings.TrimSpace(query["q"])
	state := gradingState(query["state"])
	selected := findGradingItem(all, selectedID)
	if selected != nil && state == "" {
		state = "pending"
		if selected.answer.Graded {
			state = "graded"
		}
	}
	if state == "" {
		state = "pending"
	}

	base := pending
	if state == "graded" {
		base = graded
	}
	rows := filterGradingItems(w, base, search, query["quiz"], query["class"])

	// The detail route always lands on something useful. When the id is
	// unknown, pick the first row of the current tab (or the first tab that
	// still has rows).
	if selected == nil && selectedID > 0 {
		switch {
		case len(rows) > 0:
			selected = &rows[0]
		case len(base) > 0:
			selected = &base[0]
		case len(all) > 0:
			selected = &all[0]
		}
	}

	queueRows := gradingRows(w, rows)
	list := viewmodel.NewList(queueRows, gradingPageNumber(query["page"]), gradingPerPage)

	paginationQuery := url.Values{}
	for key, value := range query {
		if value != "" {
			paginationQuery.Set(key, value)
		}
	}
	pagination := viewmodel.NewPagination("/teacher/grading", paginationQuery, list.Page, list.PerPage, list.Total)

	empty := viewmodel.EmptyState{
		Title: "Tidak ada esai yang menunggu penilaian.",
		Body:  "Esai yang dikirim peserta akan muncul di sini.",
		Icon:  "check",
	}
	if state == "graded" {
		empty = viewmodel.EmptyState{
			Title: "Belum ada esai yang dinilai.",
			Body:  "Nilai esai yang sudah disimpan akan muncul di sini.",
			Icon:  "grade",
		}
	} else if search != "" {
		empty.Body = "Coba kata kunci lain atau hapus pencarian."
	}

	tabs := []viewmodel.FilterTab{
		{Label: "Menunggu", Href: gradingListHref("pending", search), Active: state == "pending", Count: len(pending)},
		{Label: "Sudah dinilai", Href: gradingListHref("graded", search), Active: state == "graded", Count: len(graded)},
	}

	page := viewmodel.GradingPage{
		Queue: viewmodel.GradingQueue{
			Counts:     viewmodel.GradingCounts{Pending: len(pending), GradedToday: len(graded)},
			Rows:       list,
			Pagination: pagination,
			Tabs:       tabs,
			Query:      viewmodel.QueryState{Q: search, Tab: state},
			Empty:      empty,
		},
	}
	if selected != nil {
		nextPool := rows
		if !containsGradingItem(rows, selected.answer.ID) {
			nextPool = base
		}
		page.Selected = gradingDetail(w, *selected, nextPool)
	}
	return page
}

// RecapPage returns the teacher recap for one attempt. An unknown attempt id
// falls back to the demo recap (Ayu, Biologi Sel — Kuis 4, attempt 1).
func RecapPage(attemptID int64) viewmodel.Recap {
	w := Default()
	attempt := w.Attempt(attemptID)
	if attempt == nil {
		attempt = w.Attempt(defaultRecapAttemptID)
	}
	if attempt == nil {
		return viewmodel.Recap{}
	}
	return buildRecap(w, attempt)
}

// ---- Queue ----------------------------------------------------------------

func gradingItems() []gradingItem {
	w := Default()
	items := make([]gradingItem, 0, 10)
	for _, registration := range w.Registrations {
		for _, attempt := range registration.Attempts {
			if attempt.SubmittedAt == nil {
				continue
			}
			for _, answer := range attempt.Answers {
				question := w.Question(answer.QuestionID)
				if question == nil || question.Type != viewmodel.QuestionEssay {
					continue
				}
				items = append(items, gradingItem{
					attempt:      attempt,
					registration: registration,
					answer:       answer,
					question:     question,
					quiz:         w.Quiz(registration.QuizID),
				})
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i].attempt.SubmittedAt.Time(), items[j].attempt.SubmittedAt.Time()
		if left.Equal(right) {
			return items[i].answer.ID > items[j].answer.ID
		}
		return left.After(right)
	})
	return items
}

func gradingRows(w *World, items []gradingItem) []viewmodel.GradingRow {
	rows := make([]viewmodel.GradingRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, viewmodel.GradingRow{
			AnswerID:        item.answer.ID,
			StudentName:     w.DisplayName(item.registration),
			ClassLabel:      w.RegistrationClass(item.registration),
			QuizTitle:       item.quiz.Title,
			QuestionExcerpt: item.question.Body,
			WaitingSince:    *item.attempt.SubmittedAt,
			WaitingLabel:    gradingWaitingLabel(item),
			Href:            "/teacher/grading/" + strconv.FormatInt(item.answer.ID, 10),
		})
	}
	return rows
}

func gradingDetail(w *World, item gradingItem, nextPool []gradingItem) *viewmodel.GradingDetail {
	state := viewmodel.EssayUngraded
	if item.answer.Graded {
		state = viewmodel.EssayGraded
	}
	return &viewmodel.GradingDetail{
		Answer: viewmodel.EssayAnswer{
			ID:            item.answer.ID,
			QuestionID:    item.question.ID,
			StudentName:   w.DisplayName(item.registration),
			ClassLabel:    w.RegistrationClass(item.registration),
			QuizTitle:     item.quiz.Title,
			QuestionBody:  item.question.Body,
			QuestionImage: item.question.Image,
			Text:          item.answer.Essay,
			SubmittedAt:   *item.attempt.SubmittedAt,
			WaitingLabel:  gradingWaitingLabel(item),
			State:         state,
			Score:         item.answer.Score,
			Feedback:      item.answer.Feedback,
		},
		Attempt:   w.AttemptHeader(item.attempt),
		Suggested: ScorePtr(suggestedEssayScore),
		Score:     item.answer.Score,
		Feedback:  item.answer.Feedback,
		Context:   gradingContext(w, item.attempt, item.answer.ID),
		Action:    "/teacher/grading/" + strconv.FormatInt(item.answer.ID, 10),
		BackHref:  "/teacher/grading",
		NextHref:  nextGradingHref(item.answer.ID, nextPool),
	}
}

func gradingWaitingLabel(item gradingItem) string {
	if item.answer.Graded {
		if item.answer.Score != nil {
			return "Dinilai " + item.answer.Score.String()
		}
		return "Sudah dinilai"
	}
	return "Menunggu " + item.attempt.SubmittedAt.RelativeShort(gradingNow)
}

func gradingContext(w *World, attempt *Attempt, excludeAnswerID int64) []viewmodel.GradingContextRow {
	registration := w.RegistrationOfAttempt(attempt)
	if registration == nil {
		return nil
	}
	quiz := w.Quiz(registration.QuizID)
	if quiz == nil {
		return nil
	}

	var current *Answer
	for _, answer := range attempt.Answers {
		if answer.ID == excludeAnswerID {
			current = answer
			break
		}
	}

	questions := append([]*Question(nil), w.QuestionsFor(quiz.ID)...)
	sort.SliceStable(questions, func(i, j int) bool {
		return questions[i].Position < questions[j].Position
	})

	rows := make([]viewmodel.GradingContextRow, 0, len(questions))
	for _, question := range questions {
		if current != nil && question.ID == current.QuestionID {
			continue
		}
		var saved *Answer
		for _, answer := range attempt.Answers {
			if answer.QuestionID == question.ID {
				saved = answer
				break
			}
		}
		if saved == nil {
			continue
		}
		score, verdict := questionResult(question, saved)
		rows = append(rows, viewmodel.GradingContextRow{
			Position:      question.Position,
			Type:          question.Type,
			Body:          question.Body,
			StudentAnswer: AnswerLabel(question, saved),
			Key:           CorrectLabel(question),
			Score:         score,
			Verdict:       verdict,
		})
	}
	return rows
}

func nextGradingHref(answerID int64, items []gradingItem) string {
	for index, item := range items {
		if item.answer.ID != answerID || len(items) < 2 {
			continue
		}
		next := items[(index+1)%len(items)]
		return "/teacher/grading/" + strconv.FormatInt(next.answer.ID, 10)
	}
	return ""
}

func containsGradingItem(items []gradingItem, answerID int64) bool {
	for _, item := range items {
		if item.answer.ID == answerID {
			return true
		}
	}
	return false
}

func findGradingItem(items []gradingItem, answerID int64) *gradingItem {
	if answerID <= 0 {
		return nil
	}
	for i := range items {
		if items[i].answer.ID == answerID {
			return &items[i]
		}
	}
	return nil
}

func filterGradingItems(w *World, items []gradingItem, search, quizID, classLabel string) []gradingItem {
	search = strings.ToLower(strings.TrimSpace(search))
	quizID = strings.TrimSpace(quizID)
	classLabel = strings.ToLower(strings.TrimSpace(classLabel))

	out := make([]gradingItem, 0, len(items))
	for _, item := range items {
		if quizID != "" && strconv.FormatInt(item.quiz.ID, 10) != quizID {
			continue
		}
		if classLabel != "" && strings.ToLower(w.RegistrationClass(item.registration)) != classLabel {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				w.DisplayName(item.registration),
				w.RegistrationClass(item.registration),
				item.quiz.Title,
				item.question.Body,
				item.answer.Essay,
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func gradingState(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "graded", "dinilai", "sudah-dinilai":
		return "graded"
	case "pending", "menunggu", "waiting":
		return "pending"
	default:
		return ""
	}
}

func gradingListHref(state, search string) string {
	values := url.Values{}
	if search != "" {
		values.Set("q", search)
	}
	if state == "graded" {
		values.Set("state", "graded")
	}
	if len(values) == 0 {
		return "/teacher/grading"
	}
	return "/teacher/grading?" + values.Encode()
}

func gradingPageNumber(raw string) int {
	page, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

// ---- Recap ----------------------------------------------------------------

func buildRecap(w *World, attempt *Attempt) viewmodel.Recap {
	registration := w.RegistrationOfAttempt(attempt)
	if registration == nil {
		return viewmodel.Recap{}
	}
	quiz := w.Quiz(registration.QuizID)
	if quiz == nil {
		return viewmodel.Recap{}
	}

	waiting := attempt.Waiting || attempt.Final == nil
	recap := viewmodel.Recap{
		Attempt: viewmodel.RecapAttempt{
			ID:          attempt.ID,
			StudentName: w.DisplayName(registration),
			ClassLabel:  w.RegistrationClass(registration),
			QuizTitle:   quiz.Title,
			QuizID:      quiz.ID,
			AttemptNo:   attempt.Number,
			MaxAttempts: quiz.MaxAttempts,
			StartedAt:   viewmodel.Ptr(attempt.StartedAt),
			SubmittedAt: attempt.SubmittedAt,
			FocusLoss:   attempt.FocusLoss,
			Score:       attempt.Final,
			Waiting:     waiting,
		},
		Score:    attempt.Final,
		Waiting:  waiting,
		BackHref: "/teacher/quizzes/" + strconv.FormatInt(quiz.ID, 10) + "/results",
	}

	questions := append([]*Question(nil), w.QuestionsFor(quiz.ID)...)
	sort.SliceStable(questions, func(i, j int) bool {
		return questions[i].Position < questions[j].Position
	})

	for _, question := range questions {
		var answer *Answer
		for _, candidate := range attempt.Answers {
			if candidate.QuestionID == question.ID {
				answer = candidate
				break
			}
		}
		score, verdict := questionResult(question, answer)
		row := viewmodel.RecapRow{
			Position:      question.Position,
			Type:          question.Type,
			Body:          question.Body,
			StudentAnswer: AnswerLabel(question, answer),
			Key:           CorrectLabel(question),
			Score:         score,
			Verdict:       verdict,
			Essay:         question.Type == viewmodel.QuestionEssay,
			Graded:        question.Type.IsObjective() || (answer != nil && answer.Graded),
		}
		if answer != nil {
			row.Feedback = answer.Feedback
		}
		recap.Answers = append(recap.Answers, row)
	}
	return recap
}

// questionResult is the single auto-grading rule shared by the grading context
// and the recap: objective answers are worth 100 or 0, essays use the stored
// score, and blank answers are unanswered rather than waiting.
func questionResult(question *Question, answer *Answer) (*viewmodel.Score, string) {
	if question == nil {
		return nil, "—"
	}
	if question.Type.IsObjective() {
		if answer == nil || len(answer.Selected) == 0 {
			return ScorePtr(0), "Tidak dijawab"
		}
		if objectiveCorrect(question, answer) {
			return ScorePtr(100), "Benar"
		}
		return ScorePtr(0), "Salah"
	}
	if answer == nil || strings.TrimSpace(answer.Essay) == "" {
		return ScorePtr(0), "Tidak dijawab"
	}
	if answer.Graded {
		return answer.Score, "Sudah dinilai"
	}
	return nil, "Menunggu nilai"
}

func objectiveCorrect(question *Question, answer *Answer) bool {
	if question == nil || answer == nil || len(answer.Selected) != len(question.Correct) {
		return false
	}
	wanted := make(map[int]bool, len(question.Correct))
	for _, index := range question.Correct {
		wanted[index] = true
	}
	for _, index := range answer.Selected {
		if !wanted[index] {
			return false
		}
		delete(wanted, index)
	}
	return len(wanted) == 0
}
