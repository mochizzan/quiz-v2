package authoring

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// createRetries is the code/token collision budget (AUT-038). The unique keys
// decide the winner; the loser regenerates both values inside its transaction.
const createRetries = 10

// CreateQuiz executes POST /teacher/quizzes (AUT-002). It returns the new quiz
// id and the success flash key.
func (s *Service) CreateQuiz(ctx context.Context, ownerID int64, in QuizInput) (int64, string, error) {
	values, appErr := ValidateQuizCreate(in)
	if appErr != nil {
		return 0, "", appErr
	}

	var id int64
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		if values.Visibility == viewmodel.VisibilityPrivate {
			live, err := lockMajor(ctx, tx, values.TargetMajorID)
			if err != nil {
				return err
			}
			if !live {
				return apperr.Field("target_major_id", MsgMajorUnavailable)
			}
		}

		for attempt := 0; attempt < createRetries; attempt++ {
			code := sql.NullString{}
			if values.Visibility == viewmodel.VisibilityPublic {
				value, err := publicCode()
				if err != nil {
					return internal(err)
				}
				code = sql.NullString{String: value, Valid: true}
			}
			token, err := inviteToken()
			if err != nil {
				return internal(err)
			}
			result, err := tx.ExecContext(ctx, `INSERT INTO quizzes
				(owner_account_id, updated_by_account_id, title, description, visibility, state,
				 code, invite_token, target_grade, target_major_id, starts_at, ends_at,
				 allow_retake, max_attempts, show_score, review_mode,
				 randomize_questions, randomize_options, published_at, closed_at)
				VALUES (?, ?, ?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?, 0, 1, 1, 'questions_no_answers', 0, 0, NULL, NULL)`,
				ownerID, ownerID, values.Title, values.Description, string(values.Visibility),
				stringArg(code), token, targetGradeArg(values.Visibility, values.TargetGrade),
				targetMajorArg(values.Visibility, values.TargetMajorID),
				nullableTime(values.StartsAt), nullableTime(values.EndsAt))
			if err == nil {
				id, err = result.LastInsertId()
				if err != nil {
					return internal(err)
				}
				return nil
			}
			if store.IsDuplicate(err) {
				continue
			}
			return internal(err)
		}
		return internal(errors.New("authoring: exhausted code/token retries"))
	})
	if err != nil {
		return 0, "", err
	}
	return id, FlashQuizCreated, nil
}

// SaveMeta executes the form=meta branch of POST /teacher/quizzes/:id (AUT-005).
// Visibility, targets, code, token and settings are never read or written here.
func (s *Service) SaveMeta(ctx context.Context, ownerID, quizID int64, in QuizInput) (string, error) {
	values, appErr := ValidateQuizMeta(in)
	if appErr != nil {
		return "", appErr
	}
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, true)
		if err != nil {
			return err
		}
		if err := guard(contentDecision(row.State)); err != nil {
			return err
		}
		if row.Title == values.Title && row.Description == values.Description &&
			sameNullableTime(row.StartsAt, values.StartsAt) && sameNullableTime(row.EndsAt, values.EndsAt) {
			return nil
		}
		_, err = tx.ExecContext(ctx, `UPDATE quizzes
			SET title = ?, description = ?, starts_at = ?, ends_at = ?, updated_by_account_id = ?
			WHERE id = ? AND deleted_at = ?`,
			values.Title, values.Description, nullableTime(values.StartsAt), nullableTime(values.EndsAt),
			ownerID, quizID, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return FlashSaved, nil
}

// SaveSettings executes the form=settings branch of POST /teacher/quizzes/:id
// (AUT-005). Any field error rejects the whole branch; identical values commit
// no write.
func (s *Service) SaveSettings(ctx context.Context, ownerID, quizID int64, form url.Values) (string, error) {
	values, appErr := ValidateSettings(form)
	if appErr != nil {
		return "", appErr
	}
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, true)
		if err != nil {
			return err
		}
		if err := guard(contentDecision(row.State)); err != nil {
			return err
		}
		if row.RandomizeQuestions == values.RandomizeQuestions &&
			row.RandomizeOptions == values.RandomizeAnswers &&
			row.AllowRetake == values.AllowRetake &&
			row.MaxAttempts == values.MaxAttempts &&
			row.ReviewMode == string(values.ReviewMode) &&
			row.ShowScore == values.ShowScore {
			return nil
		}
		_, err = tx.ExecContext(ctx, `UPDATE quizzes
			SET randomize_questions = ?, randomize_options = ?, allow_retake = ?, max_attempts = ?,
				review_mode = ?, show_score = ?, updated_by_account_id = ?
			WHERE id = ? AND deleted_at = ?`,
			values.RandomizeQuestions, values.RandomizeAnswers, values.AllowRetake, values.MaxAttempts,
			string(values.ReviewMode), values.ShowScore, ownerID, quizID, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return FlashSaved, nil
}

// Publish executes POST /teacher/quizzes/:id/publish (AUT-006). The gate runs
// under the quiz lock with the same predicate the detail page uses.
func (s *Service) Publish(ctx context.Context, ownerID, quizID int64) (string, error) {
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, true)
		if err != nil {
			return err
		}
		if err := guard(publishDecision(row.State)); err != nil {
			return err
		}

		questions, err := readQuestions(ctx, tx, quizID)
		if err != nil {
			return err
		}
		majorLive := false
		if viewmodel.Visibility(row.Visibility) == viewmodel.VisibilityPrivate && row.TargetMajorID.Valid {
			majorLive, err = readMajorLive(ctx, tx, row.TargetMajorID.Int64)
			if err != nil {
				return err
			}
		}
		result := EvaluatePublish(PublishInput{
			Quiz: PublishQuiz{
				Visibility:         row.Visibility,
				Code:               row.Code,
				InviteToken:        row.InviteToken,
				TargetGrade:        row.TargetGrade,
				TargetMajorID:      row.TargetMajorID,
				MajorLive:          majorLive,
				AllowRetake:        row.AllowRetake,
				MaxAttempts:        row.MaxAttempts,
				ShowScore:          row.ShowScore,
				ReviewMode:         row.ReviewMode,
				RandomizeQuestions: row.RandomizeQuestions,
				RandomizeOptions:   row.RandomizeOptions,
			},
			Questions:  toPublishQuestions(questions),
			UploadsDir: s.uploads,
		})
		if result.Err != nil {
			return internal(result.Err)
		}
		if !result.OK {
			return conflict(result.Reason)
		}

		t, err := nowT(ctx, tx)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE quizzes
			SET state = 'open', published_at = ?, closed_at = NULL, updated_by_account_id = ?
			WHERE id = ? AND deleted_at = ?`, t, ownerID, quizID, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return FlashPublished, nil
}

// Close executes POST /teacher/quizzes/:id/close (AUT-007). Close is terminal.
func (s *Service) Close(ctx context.Context, ownerID, quizID int64) (string, error) {
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, true)
		if err != nil {
			return err
		}
		if err := guard(closeDecision(row.State)); err != nil {
			return err
		}

		t, err := nowT(ctx, tx)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE quizzes
			SET state = 'closed', closed_at = ?, updated_by_account_id = ?
			WHERE id = ? AND deleted_at = ?`, t, ownerID, quizID, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return FlashClosed, nil
}

// DeleteQuiz executes POST /teacher/quizzes/:id/delete (AUT-008). Only a draft
// is deletable; the whole aggregate is stamped in one transaction and the image
// files are unlinked only after the commit.
func (s *Service) DeleteQuiz(ctx context.Context, ownerID, quizID int64) (string, error) {
	var images []string
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, true)
		if err != nil {
			return err
		}
		if err := guard(deleteDecision(row.State)); err != nil {
			return err
		}
		if err := assertNoParticipation(ctx, tx, quizID); err != nil {
			return err
		}

		images, err = collectQuizImages(ctx, tx, quizID)
		if err != nil {
			return err
		}

		t, err := nowT(ctx, tx)
		if err != nil {
			return err
		}
		quizMax, err := maxDeleted(ctx, tx,
			"SELECT MAX(deleted_at) FROM quizzes WHERE (code = ? OR invite_token = ?) AND deleted_at <> ?",
			stringArg(row.Code), row.InviteToken, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		questionMax, err := maxDeleted(ctx, tx,
			"SELECT MAX(deleted_at) FROM questions WHERE quiz_id = ? AND deleted_at <> ?",
			quizID, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		optionMax, err := maxDeleted(ctx, tx,
			`SELECT MAX(o.deleted_at) FROM answer_options o
			 JOIN questions z ON z.id = o.question_id
			 WHERE z.quiz_id = ? AND o.deleted_at <> ?`, quizID, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		tDel, ok := NextStamp(t, []sql.NullTime{quizMax, questionMax, optionMax})
		if !ok {
			return internal(errors.New("authoring: delete stamp exceeds DATETIME"))
		}

		if _, err := tx.ExecContext(ctx, `UPDATE answer_options o
			JOIN questions z ON z.id = o.question_id
			SET o.deleted_at = ?
			WHERE z.quiz_id = ? AND z.deleted_at = ? AND o.deleted_at = ?`,
			tDel, quizID, store.LiveAt, store.LiveAt); err != nil {
			return internal(err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE questions
			SET deleted_at = ?, deleted_by_account_id = NULL
			WHERE quiz_id = ? AND deleted_at = ?`,
			tDel, quizID, store.LiveAt); err != nil {
			return internal(err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE quizzes
			SET deleted_at = ?, deleted_by_account_id = ?
			WHERE id = ? AND deleted_at = ?`,
			tDel, ownerID, quizID, store.LiveAt); err != nil {
			return internal(err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	for _, name := range images {
		s.discardImage(name)
	}
	return FlashQuizDeleted, nil
}

// assertNoParticipation enforces AUT-INV-12: a draft can never have attempt
// data, so a hit is an integrity fault rather than a cascade.
func assertNoParticipation(ctx context.Context, tx *sql.Tx, quizID int64) error {
	var exists bool
	if err := tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM registrations WHERE quiz_id = ? AND deleted_at = ?)",
		quizID, store.LiveAt).Scan(&exists); err != nil {
		return internal(err)
	}
	if exists {
		return internal(errors.New("authoring: draft quiz has a live registration"))
	}
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM quiz_attempts a
			JOIN registrations r ON r.id = a.registration_id
			WHERE r.quiz_id = ? AND a.deleted_at = ? AND r.deleted_at = ?)`,
		quizID, store.LiveAt, store.LiveAt).Scan(&exists); err != nil {
		return internal(err)
	}
	if exists {
		return internal(errors.New("authoring: draft quiz has a live attempt"))
	}
	return nil
}

// collectQuizImages returns every live question's image name under the lock.
func collectQuizImages(ctx context.Context, q store.Querier, quizID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT image_path FROM questions
		WHERE quiz_id = ? AND deleted_at = ? AND image_path IS NOT NULL`, quizID, store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, internal(err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// lockMajor takes the shared lock on a live target major (OPS-150, PAR-121).
func lockMajor(ctx context.Context, tx *sql.Tx, majorID int64) (bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM majors WHERE id = ? AND deleted_at = ? LOCK IN SHARE MODE",
		majorID, store.LiveAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, internal(err)
	}
	return true, nil
}

// publicCode mints the six-digit code, preserving leading zeros (AUT-038).
func publicCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// inviteToken mints 128 random bits as 22 unpadded base64url characters.
func inviteToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// targetGradeArg writes NULL for a public quiz, whatever the form carried.
func targetGradeArg(visibility viewmodel.Visibility, grade int) any {
	if visibility != viewmodel.VisibilityPrivate || grade == 0 {
		return nil
	}
	return grade
}

// targetMajorArg writes NULL for a public quiz.
func targetMajorArg(visibility viewmodel.Visibility, majorID int64) any {
	if visibility != viewmodel.VisibilityPrivate || majorID <= 0 {
		return nil
	}
	return majorID
}

// nullableTime converts an optional local instant into a driver argument.
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

// sameNullableTime compares a stored DATETIME with a submitted optional value.
func sameNullableTime(stored sql.NullTime, want *time.Time) bool {
	if !stored.Valid && want == nil {
		return true
	}
	if !stored.Valid || want == nil {
		return false
	}
	return stored.Time.UTC().Equal(want.UTC())
}
