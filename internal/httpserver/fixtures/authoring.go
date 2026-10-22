package fixtures

import (
	"sort"
	"strconv"

	"quiz-v2/internal/viewmodel"
)

// This file builds the authoring screens from the single demo world: the create
// form, the quiz builder, and the question editor. Nothing here mutates the
// world; POST routes are UI-only redirects.

const questionEssayNote = "Esai dinilai manual oleh guru. Tulis pertanyaan terbuka yang mendorong penjelasan, bukan jawaban ya atau tidak."

// NewQuizFormData is GET /teacher/quizzes/new. A new quiz is public, has no
// schedule yet, and starts in the draft state after the first save.
func NewQuizFormData() viewmodel.QuizForm {
	return viewmodel.QuizForm{
		Visibility: viewmodel.VisibilityPublic,
		Majors:     Majors(),
		Action:     "/teacher/quizzes",
		CancelHref: "/teacher/quizzes",
	}
}

// QuizDetailData is GET /teacher/quizzes/:id. The builder is editable only
// while the quiz is a draft; publishing is blocked until the quiz has a
// question and, for private quizzes, a complete target pair.
func QuizDetailData(quizID int64) viewmodel.QuizDetail {
	w := Default()
	q := w.Quiz(quizID)
	if q == nil {
		return missingQuizDetail(quizID)
	}

	questions := w.QuestionsFor(quizID)
	rows := make([]viewmodel.QuestionRow, 0, len(questions))
	for _, question := range questions {
		rows = append(rows, viewmodel.QuestionRow{
			ID:           question.ID,
			Position:     question.Position,
			Type:         question.Type,
			Body:         question.Body,
			OptionCount:  len(question.Options),
			HasImage:     question.Image != nil,
			CorrectLabel: CorrectLabel(question),
			EditHref:     questionPath(quizID, question.ID),
			DeleteHref:   questionPath(quizID, question.ID) + "/delete",
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Position != rows[j].Position {
			return rows[i].Position < rows[j].Position
		}
		return rows[i].ID < rows[j].ID
	})

	detail := viewmodel.QuizDetail{
		Quiz:               w.QuizRef(quizID),
		Description:        q.Description,
		TeacherName:        w.TeacherName(q.TeacherID),
		State:              q.State,
		Visibility:         q.Visibility,
		Audience:           w.Audience(q),
		InviteURL:          "/invite/" + q.InviteToken,
		StartsAt:           q.StartsAt,
		EndsAt:             q.EndsAt,
		CreatedAt:          q.CreatedAt,
		QuestionCount:      len(questions),
		Questions:          rows,
		Editable:           q.State == viewmodel.QuizDraft,
		ResultsHref:        resultsPath(quizID),
		MonitorHref:        monitorPath(quizID),
		EditHref:           quizPath(quizID),
		AddQuestionHref:    newQuestionPath(quizID),
		GradingHref:        "/teacher/grading",
		RandomizeQuestions: q.RandomizeQuestions,
		RandomizeAnswers:   q.RandomizeAnswers,
		Settings: viewmodel.QuizSettings{
			MaxAttempts:        q.MaxAttempts,
			ReviewMode:         q.ReviewMode,
			ShowScore:          q.ShowScore,
			RandomizeQuestions: q.RandomizeQuestions,
			RandomizeAnswers:   q.RandomizeAnswers,
			AllowRetake:        q.AllowRetake,
		},
	}
	if q.Visibility == viewmodel.VisibilityPublic {
		detail.Code = q.Code
	}

	hasTarget := q.TargetGrade != 0 && q.TargetMajorID != 0
	detail.CanPublish = detail.Editable && len(questions) > 0 &&
		(q.Visibility == viewmodel.VisibilityPublic || hasTarget)
	detail.CanClose = q.State == viewmodel.QuizOpen

	if detail.Editable && !detail.CanPublish {
		switch {
		case len(questions) == 0:
			detail.PublishBlockedReason = "Tambahkan minimal satu soal sebelum membuka kuis."
		case q.Visibility == viewmodel.VisibilityPrivate && !hasTarget:
			detail.PublishBlockedReason = "Lengkapi kelas dan jurusan sasaran sebelum membuka kuis."
		}
	}

	if detail.Editable {
		publish := viewmodel.Primary("Buka kuis", quizPath(quizID)+"/publish")
		detail.PublishAction = &publish
		remove := viewmodel.Danger("Hapus kuis", quizPath(quizID)+"/delete")
		detail.DeleteAction = &remove
	}
	if detail.CanClose {
		closeAction := viewmodel.Outline("Tutup kuis", quizPath(quizID)+"/close")
		detail.CloseAction = &closeAction
	}

	detail.Empty = viewmodel.EmptyState{
		Title:  "Belum ada soal",
		Body:   "Tambahkan soal pertama. Kuis dapat dibuka setelah memiliki minimal satu soal.",
		Icon:   "quiz",
		Action: actionPtr(viewmodel.Primary("Tambah soal", detail.AddQuestionHref)),
	}
	return detail
}

// NewQuestionFormData is GET /teacher/quizzes/:id/questions/new. A new
// objective question starts with two blank options and the first one marked as
// the key, which matches the editor's minimum validation rule.
func NewQuestionFormData(quizID int64) viewmodel.QuestionForm {
	w := Default()
	form := viewmodel.QuestionForm{
		Quiz:     w.QuizRef(quizID),
		Position: w.QuestionCount(quizID) + 1,
		Type:     viewmodel.QuestionSingle,
		Options: []viewmodel.OptionDraft{
			{Letter: viewmodel.OptionLetter(0), Correct: true},
			{Letter: viewmodel.OptionLetter(1)},
		},
		EssayNote:  questionEssayNote,
		Action:     newQuestionPath(quizID),
		CancelHref: quizPath(quizID),
		IsNew:      true,
	}
	if q := w.Quiz(quizID); q != nil {
		form.RandomizeAnswers = q.RandomizeAnswers
	}
	return form
}

// EditQuestionFormData is GET /teacher/quizzes/:id/questions/:qid. The correct
// answers are kept as option indices, never option ids, so the editor maps
// them back to the option rows directly.
func EditQuestionFormData(quizID, questionID int64) viewmodel.QuestionForm {
	w := Default()
	form := NewQuestionFormData(quizID)
	form.IsNew = false
	form.ID = 0
	form.Body = ""
	form.Options = nil
	form.Action = questionPath(quizID, questionID)
	form.DeleteHref = form.Action + "/delete"

	question := w.Question(questionID)
	if question == nil || question.QuizID != quizID {
		form.Position = 0
		form.Type = viewmodel.QuestionSingle
		return form
	}

	form.ID = question.ID
	form.Position = question.Position
	form.Type = question.Type
	form.Body = question.Body
	form.Image = question.Image
	form.Options = make([]viewmodel.OptionDraft, 0, len(question.Options))
	for i, option := range question.Options {
		form.Options = append(form.Options, viewmodel.OptionDraft{
			ID:      option.ID,
			Letter:  viewmodel.OptionLetter(i),
			Body:    option.Body,
			Correct: indexIn(question.Correct, i),
		})
	}
	return form
}

func missingQuizDetail(quizID int64) viewmodel.QuizDetail {
	return viewmodel.QuizDetail{
		Quiz:            viewmodel.QuizRef{ID: quizID},
		State:           viewmodel.QuizClosed,
		Visibility:      viewmodel.VisibilityPublic,
		Audience:        "—",
		ResultsHref:     resultsPath(quizID),
		MonitorHref:     monitorPath(quizID),
		EditHref:        quizPath(quizID),
		AddQuestionHref: newQuestionPath(quizID),
		GradingHref:     "/teacher/grading",
		Empty: viewmodel.EmptyState{
			Title:  "Kuis tidak ditemukan",
			Body:   "Kuis ini mungkin sudah dihapus atau tautannya tidak lagi berlaku.",
			Icon:   "warn",
			Action: actionPtr(viewmodel.Outline("Kembali ke daftar kuis", "/teacher/quizzes")),
		},
	}
}

func newQuestionPath(quizID int64) string {
	return quizPath(quizID) + "/questions/new"
}

func questionPath(quizID, questionID int64) string {
	return quizPath(quizID) + "/questions/" + strconv.FormatInt(questionID, 10)
}

func indexIn(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
