package authoring

import (
	"context"
	"database/sql"
	"errors"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// NewQuestionForm builds GET /teacher/quizzes/:id/questions/new (AUT-009).
// The form starts with two blank option rows and no preselected key; the
// position is display-only.
func (s *Service) NewQuestionForm(ctx context.Context, ownerID, quizID int64) (viewmodel.QuestionForm, error) {
	var out viewmodel.QuestionForm
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, false)
		if err != nil {
			return err
		}
		if err := guard(contentDecision(row.State)); err != nil {
			return err
		}
		count, err := readQuestionCount(ctx, tx, quizID)
		if err != nil {
			return err
		}
		if count >= MaxQuestions {
			return conflict(FlashErrQuestionLimit)
		}

		out = viewmodel.QuestionForm{
			Quiz:     viewmodel.QuizRef{ID: row.ID, Title: row.Title},
			Position: count + 1,
			Type:     viewmodel.QuestionSingle,
			Options: []viewmodel.OptionDraft{
				{Letter: viewmodel.OptionLetter(0)},
				{Letter: viewmodel.OptionLetter(1)},
			},
			EssayNote:        QuestionEssayNote,
			RandomizeAnswers: row.RandomizeOptions,
			Action:           quizPath(quizID) + "/questions",
			CancelHref:       quizPath(quizID),
			IsNew:            true,
		}
		return nil
	})
	if err != nil {
		return viewmodel.QuestionForm{}, err
	}
	return out, nil
}

// EditQuestionForm builds GET /teacher/quizzes/:id/questions/:qid (AUT-011).
// Correctness is returned as row state, never as an option id, so the editor
// can replace the whole option set without reviving a stale id.
func (s *Service) EditQuestionForm(ctx context.Context, ownerID, quizID, questionID int64) (viewmodel.QuestionForm, error) {
	var out viewmodel.QuestionForm
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, false)
		if err != nil {
			return err
		}
		if err := guard(contentDecision(row.State)); err != nil {
			return err
		}
		item, err := readQuestion(ctx, tx, quizID, questionID, false)
		if err != nil {
			return err
		}

		out = viewmodel.QuestionForm{
			Quiz:             viewmodel.QuizRef{ID: row.ID, Title: row.Title},
			ID:               item.ID,
			Position:         item.Position,
			Type:             viewmodel.QuestionType(item.Kind),
			Body:             item.Prompt,
			Image:            questionImage(item),
			Options:          optionDrafts(item.Options),
			EssayNote:        QuestionEssayNote,
			RandomizeAnswers: row.RandomizeOptions,
			Action:           questionPath(quizID, questionID),
			CancelHref:       quizPath(quizID),
			DeleteHref:       questionPath(quizID, questionID) + "/delete",
		}
		return nil
	})
	if err != nil {
		return viewmodel.QuestionForm{}, err
	}
	return out, nil
}

// readQuestionCount returns the number of live questions under a quiz.
func readQuestionCount(ctx context.Context, q store.Querier, quizID int64) (int, error) {
	var count int
	if err := q.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM questions WHERE quiz_id = ? AND deleted_at = ?",
		quizID, store.LiveAt).Scan(&count); err != nil {
		return 0, internal(err)
	}
	return count, nil
}

// readQuestion loads one live question of a quiz and its live options. A missing,
// deleted, or cross-quiz row is the same 404 (AUT-020). lock selects FOR UPDATE
// for the edit/delete commands.
func readQuestion(ctx context.Context, q store.Querier, quizID, questionID int64, lock bool) (*liveQuestion, error) {
	query := `SELECT id, position, type, prompt, image_path, image_mime, image_bytes, image_sha256, image_alt
		FROM questions WHERE id = ? AND quiz_id = ? AND deleted_at = ?`
	if lock {
		query += " FOR UPDATE"
	}
	var item liveQuestion
	err := q.QueryRowContext(ctx, query, questionID, quizID, store.LiveAt).Scan(
		&item.ID, &item.Position, &item.Kind, &item.Prompt,
		&item.ImagePath, &item.ImageMime, &item.ImageBytes, &item.ImageSHA, &item.ImageAlt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFoundf()
	}
	if err != nil {
		return nil, internal(err)
	}

	optQuery := `SELECT position, body, is_correct FROM answer_options
		WHERE question_id = ? AND deleted_at = ? ORDER BY position, id`
	if lock {
		optQuery += " FOR UPDATE"
	}
	rows, err := q.QueryContext(ctx, optQuery, questionID, store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var opt liveOption
		if err := rows.Scan(&opt.Position, &opt.Body, &opt.Correct); err != nil {
			return nil, internal(err)
		}
		item.Options = append(item.Options, opt)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return &item, nil
}

// questionImage renders the editor preview. A partial image row still renders
// its URL rather than becoming a 404 (AUT-011); the publish gate is the place
// that rejects impossible metadata.
func questionImage(item *liveQuestion) *viewmodel.QuestionImage {
	if !item.ImagePath.Valid || item.ImagePath.String == "" {
		return nil
	}
	img := &viewmodel.QuestionImage{
		URL: "/uploads/" + item.ImagePath.String,
		Alt: item.ImageAlt.String,
	}
	if item.ImageBytes.Valid {
		img.SizeLabel = viewmodel.FileSize(item.ImageBytes.Int64)
	}
	return img
}

// optionDrafts maps stored option rows to the editor rows. Letters are derived
// from the display order, never stored.
func optionDrafts(options []liveOption) []viewmodel.OptionDraft {
	out := make([]viewmodel.OptionDraft, 0, len(options))
	for i, opt := range options {
		out = append(out, viewmodel.OptionDraft{
			Letter:  viewmodel.OptionLetter(i),
			Body:    opt.Body,
			Correct: opt.Correct,
		})
	}
	return out
}
