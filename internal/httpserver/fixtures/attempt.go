// This file builds the three attempt screens from the demo world: the
// one-question answer form, the pre-submit review and the post-submit result.
// Everything is read through the same attempt/answer rows the teacher monitor,
// results and recap use, so the student and teacher views never disagree.
package fixtures

import (
	"strconv"
	"strings"

	"quiz-v2/internal/viewmodel"
)

// defaultAttemptQuestion is the in-progress attempt the demo resumes when an
// unknown id is requested: Ayu's second Biologi attempt.
const defaultAttemptQuestion int64 = 1009

// defaultAttemptResult is the fully answered attempt behind the flagship
// result screen: Ayu's first Biologi attempt.
const defaultAttemptResult int64 = 1001

// AttemptQuestionPage is the page-local view of AttemptQuestion. The shared
// view model deliberately has no question id or form target, so the attempt
// route adds the two values the answer form must post.
type AttemptQuestionPage struct {
	viewmodel.AttemptQuestion
	QuestionID int64
	AnswerHref string
}

// AttemptQuestionData builds /attempt/:id for one 1-based question position.
// Position 0 means "first unanswered question", which keeps the page useful
// without JavaScript; an out-of-range position clamps into the quiz.
func AttemptQuestionData(attemptID int64, position int) AttemptQuestionPage {
	w := Default()
	attempt := w.Attempt(attemptID)
	if attempt == nil {
		attempt = w.Attempt(defaultAttemptQuestion)
	}
	if attempt == nil {
		return AttemptQuestionPage{}
	}

	reg := w.RegistrationOfAttempt(attempt)
	if reg == nil {
		return AttemptQuestionPage{}
	}
	quiz := w.Quiz(reg.QuizID)
	questions := w.QuestionsFor(reg.QuizID)
	total := len(questions)
	if total == 0 {
		return AttemptQuestionPage{
			AttemptQuestion: viewmodel.AttemptQuestion{
				Header:     w.AttemptHeader(attempt),
				SubmitHref: attemptPath(attempt.ID) + "/review",
				QuizHref:   quizHref(reg.QuizID),
			},
			AnswerHref: attemptPath(attempt.ID) + "/answer?q=1",
		}
	}

	if position < 1 {
		position = firstUnansweredPosition(attempt, questions)
	}
	if position > total {
		position = total
	}

	question := questions[position-1]
	answer := attemptAnswer(attempt, question.ID)
	answered := attemptAnswered(attempt)

	out := AttemptQuestionPage{
		AttemptQuestion: viewmodel.AttemptQuestion{
			Header:           w.AttemptHeader(attempt),
			Progress:         viewmodel.AttemptProgress{Answered: answered, Total: total},
			Position:         position,
			Total:            total,
			Type:             question.Type,
			Body:             question.Body,
			Image:            question.Image,
			Options:          attemptOptions(question, answer),
			Answer:           attemptDraft(question, answer),
			Nav:              attemptNav(attempt.ID, questions, attempt, position),
			Action:           attemptPath(attempt.ID) + "/answer?q=" + strconv.Itoa(position),
			SubmitHref:       attemptPath(attempt.ID) + "/review",
			QuizHref:         quizHref(reg.QuizID),
			IsLast:           position == total,
			RandomizeAnswers: quiz != nil && quiz.RandomizeAnswers,
		},
		QuestionID: question.ID,
		AnswerHref: attemptPath(attempt.ID) + "/answer?q=" + strconv.Itoa(position),
	}
	if position > 1 {
		out.PrevHref = attemptQuestionHref(attempt.ID, position-1)
	}
	if position < total {
		out.NextHref = attemptQuestionHref(attempt.ID, position+1)
	}
	if question.Type == viewmodel.QuestionEssay {
		value := ""
		if answer != nil {
			value = answer.Essay
		}
		out.Essay = &viewmodel.EssayView{Value: value, MaxChars: 2000, Rows: 8}
	}
	return out
}

// AttemptReviewData builds /attempt/:id/review: every question with the
// student's current answer and a link back to it. The screen always shows all
// questions; review_mode only governs the post-submit result.
func AttemptReviewData(attemptID int64) viewmodel.AttemptReview {
	w := Default()
	attempt := w.Attempt(attemptID)
	if attempt == nil {
		attempt = w.Attempt(defaultAttemptQuestion)
	}
	if attempt == nil {
		return viewmodel.AttemptReview{}
	}
	reg := w.RegistrationOfAttempt(attempt)
	if reg == nil {
		return viewmodel.AttemptReview{}
	}
	questions := w.QuestionsFor(reg.QuizID)

	rows := make([]viewmodel.ReviewRow, 0, len(questions))
	var unanswered []int
	answered := 0
	for i, question := range questions {
		answer := attemptAnswer(attempt, question.ID)
		missing := !answerHasContent(question, answer)
		if missing {
			unanswered = append(unanswered, i+1)
		} else {
			answered++
		}
		rows = append(rows, viewmodel.ReviewRow{
			Position:   i + 1,
			Type:       question.Type,
			Body:       question.Body,
			Answer:     answerText(question, answer),
			Unanswered: missing,
			Href:       attemptQuestionHref(attempt.ID, i+1),
		})
	}

	back := attemptQuestionHref(attempt.ID, 1)
	if len(questions) > 0 {
		back = attemptQuestionHref(attempt.ID, len(questions))
	}
	return viewmodel.AttemptReview{
		Header:     w.AttemptHeader(attempt),
		Progress:   viewmodel.AttemptProgress{Answered: answered, Total: len(questions)},
		Rows:       rows,
		Unanswered: unanswered,
		SubmitHref: attemptPath(attempt.ID) + "/submit",
		BackHref:   back,
	}
}

// AttemptResultData builds /attempt/:id/result. The quiz's review_mode and
// show_score drive the screen; the demo-only state key may preview the other
// modes without changing production behaviour (the caller passes "" then).
func AttemptResultData(attemptID int64, state string) viewmodel.AttemptResult {
	w := Default()
	attempt := w.Attempt(attemptID)
	if attempt == nil {
		attempt = w.Attempt(defaultAttemptResult)
	}
	if attempt == nil {
		return viewmodel.AttemptResult{}
	}
	reg := w.RegistrationOfAttempt(attempt)
	if reg == nil {
		return viewmodel.AttemptResult{}
	}
	quiz := w.Quiz(reg.QuizID)
	questions := w.QuestionsFor(reg.QuizID)
	if quiz == nil {
		return viewmodel.AttemptResult{}
	}

	mode := reviewModeForState(quiz.ReviewMode, state)
	showScore := quiz.ShowScore
	if state == "hidden-score" || state == "hidden_score" {
		showScore = false
	}
	waiting := attempt.Waiting || (attempt.SubmittedAt != nil && attempt.Final == nil)

	rows := make([]viewmodel.ResultRow, 0, len(questions))
	var unanswered []int
	for i, question := range questions {
		answer := attemptAnswer(attempt, question.ID)
		row := viewmodel.ResultRow{
			Position:      i + 1,
			Type:          question.Type,
			Body:          question.Body,
			Image:         question.Image,
			StudentAnswer: answerText(question, answer),
		}
		if !answerHasContent(question, answer) {
			unanswered = append(unanswered, i+1)
		}

		switch question.Type {
		case viewmodel.QuestionEssay:
			row.Essay = true
			row.Key = "Dinilai guru"
			switch {
			case answer != nil && answer.Graded:
				row.Graded = true
				row.Score = answer.Score
				row.Feedback = answer.Feedback
				if row.Feedback == "" {
					row.Feedback = "Tidak ada catatan dari guru."
				}
				if answerHasContent(question, answer) {
					row.Verdict = "correct"
				} else {
					row.Verdict = "unanswered"
					row.Score = nil
				}
			case answerHasContent(question, answer):
				row.Verdict = "ungraded"
			default:
				row.Verdict = "unanswered"
			}
		default:
			row.Key = CorrectLabel(question)
			switch {
			case !answerHasContent(question, answer):
				row.Verdict = "unanswered"
			case objectiveCorrect(question, answer):
				row.Verdict = "correct"
				row.Score = ScorePtr(100)
			default:
				row.Verdict = "incorrect"
				row.Score = ScorePtr(0)
			}
		}
		rows = append(rows, row)
	}

	used := len(reg.Attempts)
	inProgress := false
	for _, candidate := range reg.Attempts {
		if candidate.SubmittedAt == nil {
			inProgress = true
			break
		}
	}

	result := viewmodel.AttemptResult{
		Header:       w.AttemptHeader(attempt),
		SubmittedAt:  attempt.SubmittedAt,
		Score:        attempt.Final,
		ShowScore:    showScore,
		Graded:       !waiting,
		Waiting:      waiting,
		Mode:         mode,
		ModeLabel:    mode.Label(),
		Rows:         rows,
		Unanswered:   unanswered,
		QuizHref:     quizHref(reg.QuizID),
		ReviewHref:   attemptPath(attempt.ID) + "/review",
		HistoryHref:  "/student/history/" + strconv.FormatInt(reg.QuizID, 10),
		FocusLoss:    attempt.FocusLoss,
		AttemptsUsed: used,
		MaxAttempts:  quiz.MaxAttempts,
		CanRetry:     quiz.State == viewmodel.QuizOpen && quiz.AllowRetake && !inProgress && used < quiz.MaxAttempts,
		RetryHref:    quizHref(reg.QuizID),
	}
	if waiting || !showScore {
		// A hidden score must not leak through the model, and a provisional
		// total must never sit beside "Menunggu penilaian".
		result.Score = nil
	}
	return result
}

// reviewModeForState maps the demo preview keys to a review mode. Anything else
// keeps the quiz's configured mode.
func reviewModeForState(current viewmodel.ReviewMode, state string) viewmodel.ReviewMode {
	switch state {
	case "questions_and_key", "questions-and-key":
		return viewmodel.ReviewQuestionsAndKey
	case "key_only", "key-only":
		return viewmodel.ReviewKeyOnly
	case "questions_only", "questions-only":
		return viewmodel.ReviewQuestionsOnly
	case "answers_only", "answers-only":
		return viewmodel.ReviewAnswersOnly
	default:
		return current
	}
}

// ---- Answer helpers -------------------------------------------------------

// firstUnansweredPosition returns the 1-based position of the first question
// without a saved answer, falling back to the first question.
func firstUnansweredPosition(attempt *Attempt, questions []*Question) int {
	for i, question := range questions {
		if !answerHasContent(question, attemptAnswer(attempt, question.ID)) {
			return i + 1
		}
	}
	return 1
}

func attemptAnswer(attempt *Attempt, questionID int64) *Answer {
	if attempt == nil {
		return nil
	}
	for _, a := range attempt.Answers {
		if a.QuestionID == questionID {
			return a
		}
	}
	return nil
}

func answerHasContent(q *Question, a *Answer) bool {
	if q == nil || a == nil {
		return false
	}
	if q.Type == viewmodel.QuestionEssay {
		return strings.TrimSpace(a.Essay) != ""
	}
	return len(a.Selected) > 0
}

func attemptAnswered(attempt *Attempt) int {
	if attempt == nil {
		return 0
	}
	w := Default()
	count := 0
	for _, a := range attempt.Answers {
		if answerHasContent(w.Question(a.QuestionID), a) {
			count++
		}
	}
	return count
}

func attemptOptions(q *Question, a *Answer) []viewmodel.OptionView {
	selected := map[int]bool{}
	if a != nil {
		for _, index := range a.Selected {
			selected[index] = true
		}
	}
	out := make([]viewmodel.OptionView, 0, len(q.Options))
	for i, option := range q.Options {
		out = append(out, viewmodel.OptionView{
			ID:      option.ID,
			Letter:  viewmodel.OptionLetter(i),
			Body:    option.Body,
			Checked: selected[i],
		})
	}
	return out
}

func attemptDraft(q *Question, a *Answer) viewmodel.AnswerDraft {
	if a == nil {
		return viewmodel.AnswerDraft{}
	}
	draft := viewmodel.AnswerDraft{EssayText: a.Essay}
	for _, index := range a.Selected {
		if index >= 0 && index < len(q.Options) {
			draft.SelectedIDs = append(draft.SelectedIDs, q.Options[index].ID)
		}
	}
	return draft
}

func attemptNav(attemptID int64, questions []*Question, attempt *Attempt, current int) []viewmodel.QuestionNav {
	out := make([]viewmodel.QuestionNav, 0, len(questions))
	for i, question := range questions {
		answer := attemptAnswer(attempt, question.ID)
		out = append(out, viewmodel.QuestionNav{
			Position: i + 1,
			Href:     attemptQuestionHref(attemptID, i+1),
			Answered: answerHasContent(question, answer),
			Current:  i+1 == current,
		})
	}
	return out
}

// answerText renders an objective answer as "A. Teks; C. Teks" and an essay as
// its full text.
func answerText(q *Question, a *Answer) string {
	if q == nil || a == nil {
		return "Tidak dijawab"
	}
	if q.Type == viewmodel.QuestionEssay {
		if strings.TrimSpace(a.Essay) == "" {
			return "Tidak dijawab"
		}
		return a.Essay
	}
	return optionText(q, a.Selected)
}

// optionText renders a key or selection by option index.
func optionText(q *Question, indexes []int) string {
	if q == nil || len(indexes) == 0 {
		return "Tidak dijawab"
	}
	parts := make([]string, 0, len(indexes))
	for _, index := range indexes {
		if index < 0 || index >= len(q.Options) {
			continue
		}
		parts = append(parts, viewmodel.OptionLetter(index)+". "+q.Options[index].Body)
	}
	if len(parts) == 0 {
		return "Tidak dijawab"
	}
	return strings.Join(parts, "; ")
}

func attemptPath(id int64) string {
	return "/attempt/" + strconv.FormatInt(id, 10)
}

// attemptQuestionHref omits ?q=1 so the canonical first-question URL stays
// short; every other position carries its 1-based index.
func attemptQuestionHref(attemptID int64, position int) string {
	if position <= 1 {
		return attemptPath(attemptID)
	}
	return attemptPath(attemptID) + "?q=" + strconv.Itoa(position)
}

func quizHref(quizID int64) string {
	return "/quiz/" + strconv.FormatInt(quizID, 10)
}
