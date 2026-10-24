package authoring

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"sort"

	"quiz-v2/internal/store"
)

// imageState is the all-or-none image row a question write stores. A partial
// row is never produced by this package (AUT-029/INV-24).
type imageState struct {
	path  sql.NullString
	mime  sql.NullString
	bytes sql.NullInt64
	sha   []byte
	alt   sql.NullString
}

// CreateQuestion executes POST /teacher/quizzes/:id/questions (AUT-010). The
// image file is staged before the transaction and unlinked if the transaction
// fails; a validation failure never writes a file.
func (s *Service) CreateQuestion(ctx context.Context, ownerID, quizID int64, in QuestionInput) (string, error) {
	values, appErr := ValidateQuestion(in)
	if appErr != nil {
		return "", appErr
	}

	newName := ""
	if values.Image != nil {
		name, err := stageImage(s.uploads, values.Image)
		if err != nil {
			return "", internal(err)
		}
		newName = name
	}

	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, true)
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

		args := []any{quizID, count + 1, string(values.Type), values.Body, ownerID}
		args = append(args, createImageArgs(newName, values.Image, values.AltText)...)
		result, err := tx.ExecContext(ctx, `INSERT INTO questions
			(quiz_id, position, type, prompt, updated_by_account_id,
			 image_path, image_mime, image_bytes, image_sha256, image_alt)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
		if err != nil {
			return internal(err)
		}
		questionID, err := result.LastInsertId()
		if err != nil {
			return internal(err)
		}
		return insertOptions(ctx, tx, questionID, values.Options)
	})
	if err != nil {
		s.discardImage(newName)
		return "", err
	}
	return FlashQuestionSaved, nil
}

// UpdateQuestion executes POST /teacher/quizzes/:id/questions/:qid (AUT-012).
// The option set is replaced wholesale, the old image path comes from the
// locked row, and files are unlinked only after a successful commit.
func (s *Service) UpdateQuestion(ctx context.Context, ownerID, quizID, questionID int64, in QuestionInput) (string, error) {
	values, appErr := ValidateQuestion(in)
	if appErr != nil {
		return "", appErr
	}

	newName := ""
	if values.Image != nil {
		name, err := stageImage(s.uploads, values.Image)
		if err != nil {
			return "", internal(err)
		}
		newName = name
	}

	var (
		oldPath   string
		removeNew bool
		removeOld bool
	)

	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, true)
		if err != nil {
			return err
		}
		if err := guard(contentDecision(row.State)); err != nil {
			return err
		}
		item, err := readQuestion(ctx, tx, quizID, questionID, true)
		if err != nil {
			return err
		}
		if err := assertNoAttemptAnswers(ctx, tx, item.ID); err != nil {
			return err
		}
		if item.ImagePath.Valid && item.ImagePath.String != "" {
			oldPath = item.ImagePath.String
		}

		current := imageFromLive(item)
		desired := current
		switch {
		case values.Image != nil:
			alt := textNull(values.AltText)
			if sameStoredImage(item, values.Image) && nullStringEqual(item.ImageAlt, alt) {
				// A byte-identical re-upload keeps the live path and discards
				// the redundant new file after commit.
				removeNew = true
			} else {
				desired = imageState{
					path:  sql.NullString{String: newName, Valid: true},
					mime:  sql.NullString{String: values.Image.MIME, Valid: true},
					bytes: sql.NullInt64{Int64: int64(values.Image.Size), Valid: true},
					sha:   values.Image.SHA[:],
					alt:   alt,
				}
			}
		case values.RemoveImage:
			desired = imageState{}
		default:
			// No new file and no removal: metadata is kept. Alt is updated only
			// when the field was submitted and is always NULL without an image.
			if item.ImagePath.Valid && item.ImagePath.String != "" && in.AltPresent {
				desired.alt = textNull(values.AltText)
			}
		}

		if item.Kind == string(values.Type) && item.Prompt == values.Body &&
			imageStatesEqual(current, desired) &&
			optionsEqual(item.Options, values.Options) {
			return nil
		}

		if oldPath != "" && (!desired.path.Valid || desired.path.String != oldPath) {
			removeOld = true
		}

		t, err := nowT(ctx, tx)
		if err != nil {
			return err
		}

		args := []any{string(values.Type), values.Body}
		args = append(args, imageStateArgs(desired)...)
		args = append(args, ownerID, item.ID, quizID, store.LiveAt)
		if _, err := tx.ExecContext(ctx, `UPDATE questions
			SET type = ?, prompt = ?, image_path = ?, image_mime = ?, image_bytes = ?,
				image_sha256 = ?, image_alt = ?, updated_by_account_id = ?
			WHERE id = ? AND quiz_id = ? AND deleted_at = ?`, args...); err != nil {
			return internal(err)
		}

		if optionsEqual(item.Options, values.Options) {
			return nil
		}
		if len(item.Options) > 0 {
			optMax, err := maxDeleted(ctx, tx,
				"SELECT MAX(deleted_at) FROM answer_options WHERE question_id = ? AND deleted_at <> ?",
				item.ID, store.LiveAt)
			if err != nil {
				return internal(err)
			}
			tOpt, ok := NextStamp(t, []sql.NullTime{optMax})
			if !ok {
				return internal(errors.New("authoring: option stamp exceeds DATETIME"))
			}
			if _, err := tx.ExecContext(ctx,
				"UPDATE answer_options SET deleted_at = ? WHERE question_id = ? AND deleted_at = ?",
				tOpt, item.ID, store.LiveAt); err != nil {
				return internal(err)
			}
		}
		return insertOptions(ctx, tx, item.ID, values.Options)
	})
	if err != nil {
		s.discardImage(newName)
		return "", err
	}
	if removeNew {
		s.discardImage(newName)
	}
	if removeOld {
		s.discardImage(oldPath)
	}
	return FlashQuestionSaved, nil
}

// DeleteQuestion executes POST /teacher/quizzes/:id/questions/:qid/delete
// (AUT-013). One T_del stamps the question and its options; the remaining live
// questions are renumbered through the two-phase rewrite.
func (s *Service) DeleteQuestion(ctx context.Context, ownerID, quizID, questionID int64) (string, error) {
	imagePath := ""
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, true)
		if err != nil {
			return err
		}
		if err := guard(contentDecision(row.State)); err != nil {
			return err
		}
		item, err := readQuestion(ctx, tx, quizID, questionID, true)
		if err != nil {
			return err
		}
		if err := assertNoAttemptAnswers(ctx, tx, item.ID); err != nil {
			return err
		}
		if item.ImagePath.Valid && item.ImagePath.String != "" {
			imagePath = item.ImagePath.String
		}

		t, err := nowT(ctx, tx)
		if err != nil {
			return err
		}
		positionMax, err := maxDeleted(ctx, tx,
			"SELECT MAX(deleted_at) FROM questions WHERE quiz_id = ? AND position = ? AND deleted_at <> ?",
			quizID, item.Position, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		optionMax, err := maxDeleted(ctx, tx,
			"SELECT MAX(deleted_at) FROM answer_options WHERE question_id = ? AND deleted_at <> ?",
			item.ID, store.LiveAt)
		if err != nil {
			return internal(err)
		}
		tDel, ok := NextStamp(t, []sql.NullTime{positionMax, optionMax})
		if !ok {
			return internal(errors.New("authoring: delete stamp exceeds DATETIME"))
		}

		if _, err := tx.ExecContext(ctx,
			"UPDATE answer_options SET deleted_at = ? WHERE question_id = ? AND deleted_at = ?",
			tDel, item.ID, store.LiveAt); err != nil {
			return internal(err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE questions
			SET deleted_at = ?, deleted_by_account_id = ?
			WHERE id = ? AND quiz_id = ? AND deleted_at = ?`,
			tDel, ownerID, item.ID, quizID, store.LiveAt); err != nil {
			return internal(err)
		}

		remaining, err := readLivePositions(ctx, tx, quizID, false)
		if err != nil {
			return err
		}
		if len(remaining) == 0 {
			return nil
		}
		maxPos := 0
		for _, item := range remaining {
			if item.Position > maxPos {
				maxPos = item.Position
			}
		}
		sort.SliceStable(remaining, func(i, j int) bool {
			if remaining[i].Position != remaining[j].Position {
				return remaining[i].Position < remaining[j].Position
			}
			return remaining[i].ID < remaining[j].ID
		})
		ids := make([]int64, len(remaining))
		for i, item := range remaining {
			ids[i] = item.ID
		}
		return rewritePositions(ctx, tx, quizID, maxPos, ids)
	})
	if err != nil {
		return "", err
	}
	s.discardImage(imagePath)
	return FlashQuestionDeleted, nil
}

// ReorderQuestions executes POST /teacher/quizzes/:id/questions/reorder
// (AUT-014). A malformed or stale exact-set is a 303 err-reorder, never a 422.
func (s *Service) ReorderQuestions(ctx context.Context, ownerID, quizID int64, order string) (string, error) {
	ids, err := ParseReorder(order)
	if err != nil {
		return "", conflict(FlashErrReorder)
	}
	err = s.writeTx(ctx, func(tx *sql.Tx) error {
		row, err := readOwnedQuiz(ctx, tx, quizID, ownerID, true)
		if err != nil {
			return err
		}
		if err := guard(contentDecision(row.State)); err != nil {
			return err
		}

		rows, err := readLivePositions(ctx, tx, quizID, true)
		if err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return conflict(FlashErrReorder)
		}
		live := make(map[int64]bool, len(rows))
		maxPos := 0
		for _, item := range rows {
			live[item.ID] = true
			if item.Position > maxPos {
				maxPos = item.Position
			}
		}
		for _, id := range ids {
			if !live[id] {
				return conflict(FlashErrReorder)
			}
		}

		current := append([]positionRow(nil), rows...)
		sort.SliceStable(current, func(i, j int) bool {
			if current[i].Position != current[j].Position {
				return current[i].Position < current[j].Position
			}
			return current[i].ID < current[j].ID
		})
		unchanged := true
		for i := range current {
			if current[i].ID != ids[i] {
				unchanged = false
				break
			}
		}
		if unchanged {
			return nil
		}
		return rewritePositions(ctx, tx, quizID, maxPos, ids)
	})
	if err != nil {
		return "", err
	}
	return FlashReordered, nil
}

// positionRow is one live row in a position rewrite.
type positionRow struct {
	ID       int64
	Position int
}

// readLivePositions reads the quiz's live questions. lock orders by id, the
// deterministic lock order AUT-014 requires; callers that need display order
// sort the result themselves.
func readLivePositions(ctx context.Context, q store.Querier, quizID int64, lock bool) ([]positionRow, error) {
	query := "SELECT id, position FROM questions WHERE quiz_id = ? AND deleted_at = ? ORDER BY id"
	if lock {
		query += " FOR UPDATE"
	}
	rows, err := q.QueryContext(ctx, query, quizID, store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()

	var out []positionRow
	for rows.Next() {
		var item positionRow
		if err := rows.Scan(&item.ID, &item.Position); err != nil {
			return nil, internal(err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// rewritePositions writes the final dense order with the positive two-phase
// offset. The offset is derived from the current maximum position, so no
// intermediate value can collide with a live row (AUT-036).
func rewritePositions(ctx context.Context, tx *sql.Tx, quizID int64, maxPos int, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	offset, ok := PlanShift(maxPos, len(ids))
	if !ok {
		return internal(errors.New("authoring: position offset exceeds SMALLINT UNSIGNED"))
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE questions SET position = position + ? WHERE quiz_id = ? AND deleted_at = ?",
		offset, quizID, store.LiveAt); err != nil {
		return internal(err)
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx,
			"UPDATE questions SET position = ? WHERE id = ? AND quiz_id = ? AND deleted_at = ?",
			i+1, id, quizID, store.LiveAt); err != nil {
			return internal(err)
		}
	}
	return nil
}

// insertOptions writes the new option set at positions 1..M.
func insertOptions(ctx context.Context, tx *sql.Tx, questionID int64, options []OptionValues) error {
	for i, opt := range options {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO answer_options (question_id, position, body, is_correct) VALUES (?, ?, ?, ?)",
			questionID, i+1, opt.Body, opt.Correct); err != nil {
			return internal(err)
		}
	}
	return nil
}

// assertNoAttemptAnswers enforces OPS-056: a draft question can never have live
// answer data, so a hit is an integrity fault rather than a cascade.
func assertNoAttemptAnswers(ctx context.Context, q store.Querier, questionID int64) error {
	var exists bool
	if err := q.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM attempt_answers WHERE question_id = ? AND deleted_at = ?)",
		questionID, store.LiveAt).Scan(&exists); err != nil {
		return internal(err)
	}
	if exists {
		return internal(errors.New("authoring: draft question has a live attempt answer"))
	}
	return nil
}

// createImageArgs expands the all-NULL or all-present image columns.
func createImageArgs(name string, blob *imageBlob, alt string) []any {
	if name == "" || blob == nil {
		return []any{nil, nil, nil, nil, nil}
	}
	var altArg any
	if alt != "" {
		altArg = alt
	}
	return []any{name, blob.MIME, blob.Size, blob.SHA[:], altArg}
}

// imageStateArgs expands a stored image row for an UPDATE.
func imageStateArgs(img imageState) []any {
	var sha any
	if len(img.sha) > 0 {
		sha = img.sha
	}
	return []any{stringArg(img.path), stringArg(img.mime), int64Arg(img.bytes), sha, stringArg(img.alt)}
}

// imageFromLive copies the locked row's image columns.
func imageFromLive(item *liveQuestion) imageState {
	return imageState{
		path:  item.ImagePath,
		mime:  item.ImageMime,
		bytes: item.ImageBytes,
		sha:   item.ImageSHA,
		alt:   item.ImageAlt,
	}
}

// sameStoredImage reports whether a new upload is byte-identical to the stored
// image metadata.
func sameStoredImage(item *liveQuestion, blob *imageBlob) bool {
	if !item.ImagePath.Valid || !item.ImageMime.Valid || !item.ImageBytes.Valid || len(item.ImageSHA) != 32 {
		return false
	}
	return item.ImageMime.String == blob.MIME &&
		item.ImageBytes.Int64 == int64(blob.Size) &&
		bytes.Equal(item.ImageSHA, blob.SHA[:])
}

// imageStatesEqual compares every image column, including NULL-ness.
func imageStatesEqual(a, b imageState) bool {
	return nullStringEqual(a.path, b.path) &&
		nullStringEqual(a.mime, b.mime) &&
		nullInt64Equal(a.bytes, b.bytes) &&
		bytes.Equal(a.sha, b.sha) &&
		nullStringEqual(a.alt, b.alt)
}

// optionsEqual compares the live option set with the normalised submission.
func optionsEqual(current []liveOption, want []OptionValues) bool {
	if len(current) != len(want) {
		return false
	}
	for i := range current {
		if current[i].Body != want[i].Body || current[i].Correct != want[i].Correct {
			return false
		}
	}
	return true
}

// textNull converts trimmed text to a nullable column value.
func textNull(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullStringEqual compares two nullable strings.
func nullStringEqual(a, b sql.NullString) bool {
	return a.Valid == b.Valid && (!a.Valid || a.String == b.String)
}

// nullInt64Equal compares two nullable integers.
func nullInt64Equal(a, b sql.NullInt64) bool {
	return a.Valid == b.Valid && (!a.Valid || a.Int64 == b.Int64)
}

// int64Arg turns a nullable integer into a driver argument.
func int64Arg(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}
