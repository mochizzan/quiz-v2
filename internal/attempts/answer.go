package attempts

// answer.go implements the answer save ATT-121…ATT-127. The command never
// scores (ATT-034), replaces exactly one slot, is idempotent on equal content
// (ATT-127), and derives its redirect from the saved question's display
// position (ATT-125). Validation failures are 422 field errors; a submitted
// attempt is the answer_locked conflict (303 to the result); a foreign or
// missing attempt is 404.

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
)

// Field-error copy (ATT-122). The strings are the contract (AT-37).
const (
	FieldErrQuestion = "Soal tidak ditemukan."
	FieldErrOption   = "Pilihan jawaban tidak valid. Muat ulang halaman."
	FieldErrSingle   = "Pilih satu jawaban."
	FieldErrWrong    = "Jawaban tidak sesuai jenis soal."
	FieldErrTooLong  = "Jawaban terlalu panjang."
	FieldErrRequired = "Nilai wajib diisi."
	FieldErrRange    = "Nilai harus antara 0 dan 100."
	FieldErrFeedback = "Umpan balik maksimal 1000 karakter."
)

// AnswerInput is the raw form for POST /attempt/:id/answer. The HasX flags
// say whether the form carried the field at all: the wrong-field rule needs
// presence, not just non-empty content.
type AnswerInput struct {
	AttemptID   uint32
	QuestionRaw string   // verbatim question_id form value
	NavRaw      string   // verbatim nav value
	Options     []string // every selected_option_ids[] value, posted order, unparsed
	HasOptions  bool     // the field was present, even with zero values
	EssayRaw    string   // verbatim essay_text value
	HasEssay    bool     // the field was present

	// Owner is the actor material for the in-transaction ownership check.
	Owner ownerRef
}

// AnswerResult is a completed save: the 303 target plus the flash key ("" for
// prev/next/review/autosave, "saved" for save, per ATT-125). An autosave sets
// NoContent instead: the write is done and the client is not navigating.
type AnswerResult struct {
	Path      string
	Flash     string
	NoContent bool
}

// navKey is the closed nav vocabulary plus the legacy autosave (ATT-122).
type navKey string

const (
	navPrev     navKey = "prev"
	navSave     navKey = "save"
	navNext     navKey = "next"
	navReview   navKey = "review"
	navAutosave navKey = "autosave"
)

// resolveNav maps the posted nav onto the closed key; missing or unknown is
// save-and-stay, never a state error (ATT-122).
func resolveNav(raw string) navKey {
	switch strings.TrimSpace(raw) {
	case "prev":
		return navPrev
	case "next":
		return navNext
	case "review":
		return navReview
	case "autosave":
		return navAutosave
	default:
		return navSave
	}
}

// SaveAnswer runs one answer save in one write transaction (ATT-123) and
// returns the redirect target for the adapter.
func (s *Service) SaveAnswer(ctx context.Context, in AnswerInput) (AnswerResult, error) {
	var out AnswerResult
	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		res, err := saveAnswerTx(ctx, tx, in)
		out = res
		return err
	})
	return out, err
}

// saveAnswerTx is the transaction body of ATT-123, steps 1–7.
func saveAnswerTx(ctx context.Context, tx *sql.Tx, in AnswerInput) (AnswerResult, error) {
	// (1) Lock the attempt; re-check ownership, liveness, registration, and
	// submission in the same transaction.
	lock, err := lockAttempt(ctx, tx, in.AttemptID, in.Owner)
	if err != nil {
		return AnswerResult{}, err
	}
	if lock.Submitted() {
		// ATT-126: a submitted attempt means the answer_locked conflict, not a
		// validation error. The redirect is the result page.
		return AnswerResult{}, RedirectFlash(attemptResultPath(in.AttemptID), FlashAnswerLocked)
	}

	// Syntax validation (ATT-122): question id and option ids, no database.
	parsed, err := parseAnswer(in)
	if err != nil {
		return AnswerResult{}, err
	}

	// (2) Lock the slot row for update; then snapshot-read the question type
	// and the option randomization (immutable facts, ATT-286).
	slot, err := lockSlotAndQuestion(ctx, tx, in.AttemptID, lock.quizID, parsed.questionID)
	if err != nil {
		return AnswerResult{}, err
	}

	// Field/type coherence and the single-choice cardinality (ATT-122).
	switch {
	case slot.essay:
		if in.HasOptions && len(parsed.optionIDs) > 0 {
			return AnswerResult{}, apperr.Field("essay_text", FieldErrWrong)
		}
		if len(in.EssayRaw) > 65535 {
			return AnswerResult{}, apperr.Field("essay_text", FieldErrTooLong)
		}
	case slot.single && len(parsed.optionIDs) > 1:
		return AnswerResult{}, apperr.Field("selected_option_ids", FieldErrSingle)
	case len(parsed.optionIDs) > 0 && in.HasEssay && strings.TrimSpace(in.EssayRaw) != "":
		return AnswerResult{}, apperr.Field("selected_option_ids", FieldErrWrong)
	}

	// Membership: every posted id must be a live option of this question
	// (ATT-122). A stale form pointing at a removed option is 422, never a
	// silent drop (ATT-126).
	if len(parsed.optionIDs) > 0 {
		live, err := liveOptionIDsOf(ctx, tx, parsed.questionID, parsed.optionIDs)
		if err != nil {
			return AnswerResult{}, err
		}
		for _, id := range parsed.optionIDs {
			if !live[id] {
				return AnswerResult{}, apperr.Field("selected_option_ids", FieldErrOption)
			}
		}
	}

	// (7) The redirect is derived before the write, because the unchanged
	// path commits with no writes and carries the same target (ATT-123
	// step 3, ATT-127).
	redirect, err := answerRedirect(ctx, tx, lock, parsed.questionID, resolveNav(in.NavRaw))
	if err != nil {
		return AnswerResult{}, err
	}

	// (3) Compare posted content with the current content (ATT-127).
	equal, err := answerUnchanged(ctx, tx, slot.id, parsed, slot.essay)
	if err != nil {
		return AnswerResult{}, err
	}
	if equal {
		return redirect, nil
	}

	// (4) One timestamp for the whole replacement (ATT-123 step 4).
	now, err := nowT(ctx, tx)
	if err != nil {
		return AnswerResult{}, err
	}

	// Soft-delete every live selection of the slot with the one stamp; the
	// distinct timestamp keeps uq_selection usable again (ATT-272).
	if _, err := tx.ExecContext(ctx, `
		UPDATE attempt_answer_selections
		SET deleted_at = ?, updated_at = ?
		WHERE attempt_answer_id = ? AND deleted_at = ?`,
		now, now, slot.id, store.LiveAt,
	); err != nil {
		return AnswerResult{}, internal(err)
	}

	// (5) Write the new content: ids only for objective, text only for essay
	// (ATT-124); the exclusive write is what makes the slot state total.
	if !slot.essay {
		if _, err := tx.ExecContext(ctx, `
			UPDATE attempt_answers
			SET answer_text = NULL, updated_at = ?
			WHERE id = ?`,
			now, slot.id,
		); err != nil {
			return AnswerResult{}, internal(err)
		}
		for _, optID := range deriveOptionOrder(parsed.optionIDs, lock.seed, slot.randomizeOptions, func(id int64) int64 { return id }) {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO attempt_answer_selections (attempt_answer_id, option_id)
				VALUES (?, ?)`,
				slot.id, optID,
			); err != nil {
				return AnswerResult{}, internal(err)
			}
		}
	} else {
		var text sql.NullString
		if strings.TrimSpace(parsed.essay) != "" {
			text = sql.NullString{String: parsed.essay, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE attempt_answers
			SET answer_text = ?, updated_at = ?
			WHERE id = ?`,
			text, now, slot.id,
		); err != nil {
			return AnswerResult{}, internal(err)
		}
	}

	// (6) Stamp the attempt once: the save is the monitor's last_seen signal
	// (ATT-123 step 6). No other attempt column moves.
	if _, err := tx.ExecContext(ctx, `
		UPDATE quiz_attempts
		SET updated_at = ?
		WHERE id = ?`,
		now, in.AttemptID,
	); err != nil {
		return AnswerResult{}, internal(err)
	}

	return redirect, nil
}

// parsedAnswer is one syntactically-validated answer.
type parsedAnswer struct {
	questionID int64
	// essay carries the verbatim text ("" is the blank/NULL value).
	essay string
	// optionIDs holds the de-duplicated posted ids in first-seen order. The
	// set is what validation and comparison need; storage order comes from
	// the derived option order, not the posted order (ATT-123 step 5).
	optionIDs []int64
}

// parseAnswer validates the posted syntax without touching the database:
// question id is a positive integer, option ids are integers de-duplicated
// first-seen, essay text is carried verbatim. The wrong-field and
// single-choice rules need the question type, so they run in saveAnswerTx
// (ATT-122).
func parseAnswer(in AnswerInput) (parsedAnswer, error) {
	out := parsedAnswer{}
	qid, err := strconv.ParseInt(strings.TrimSpace(in.QuestionRaw), 10, 64)
	if err != nil || qid <= 0 {
		return out, apperr.Field("question_id", FieldErrQuestion)
	}
	out.questionID = qid
	out.essay = in.EssayRaw

	seen := map[int64]bool{}
	for _, raw := range in.Options {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		id, perr := strconv.ParseInt(trimmed, 10, 64)
		if perr != nil {
			return out, apperr.Field("selected_option_ids", FieldErrOption)
		}
		if !seen[id] {
			seen[id] = true
			out.optionIDs = append(out.optionIDs, id)
		}
	}
	return out, nil
}

// slotLockRow is the locked slot plus the immutable question facts.
type slotLockRow struct {
	id               int64
	essay            bool
	single           bool
	randomizeOptions bool
}

// lockSlotAndQuestion locks the slot row of (attempt, question) and reads the
// immutable question facts. The question must name a live question of this
// attempt's quiz: a foreign question or unknown id is 404 (ATT-253), and a
// live question of this quiz without a slot is the 500 integrity path
// (ATT-105, ATT-123 step 2).
func lockSlotAndQuestion(ctx context.Context, tx *sql.Tx, attemptID, quizID uint32, questionID int64) (slotLockRow, error) {
	var row slotLockRow
	err := tx.QueryRowContext(ctx, `
		SELECT aa.id
		FROM attempt_answers aa
		WHERE aa.attempt_id = ? AND aa.question_id = ? AND aa.deleted_at = ?
		FOR UPDATE`,
		attemptID, questionID, store.LiveAt,
	).Scan(&row.id)
	if errors.Is(err, sql.ErrNoRows) {
		// Not a slot of this attempt. Distinguish the two worlds: a live
		// question of this quiz is corruption (500); anything else (foreign
		// quiz, another attempt's question, unknown id, soft-deleted) is a
		// plain 404.
		var known int64
		err2 := tx.QueryRowContext(ctx, `
			SELECT qq.id
			FROM questions qq
			WHERE qq.id = ? AND qq.quiz_id = ? AND qq.deleted_at = ?`,
			questionID, quizID, store.LiveAt,
		).Scan(&known)
		if errors.Is(err2, sql.ErrNoRows) {
			return slotLockRow{}, ErrNotFound
		}
		if err2 != nil {
			return slotLockRow{}, internal(err2)
		}
		return slotLockRow{}, integrity(
			"attempt %d has no slot for live question %d (ATT-105)", attemptID, questionID)
	}
	if err != nil {
		return slotLockRow{}, internal(err)
	}

	// Snapshot reads of immutable facts (no locks, see lock.go).
	var qtype string
	if err := tx.QueryRowContext(ctx, `
		SELECT qq.type, zz.randomize_options
		FROM questions qq
		JOIN quizzes zz ON zz.id = qq.quiz_id AND zz.deleted_at = ?
		WHERE qq.id = ? AND qq.deleted_at = ?`,
		store.LiveAt, questionID, store.LiveAt,
	).Scan(&qtype, &row.randomizeOptions); err != nil {
		return slotLockRow{}, internal(err)
	}
	row.essay = qtype == "essay"
	row.single = qtype == "single"
	return row, nil
}

// liveOptionIDsOf answers "is every id a live option of this question" in one
// query (ATT-122). A question id that names a live question of this quiz has
// already been resolved, so an empty result simply fails membership.
func liveOptionIDsOf(ctx context.Context, tx *sql.Tx, questionID int64, ids []int64) (map[int64]bool, error) {
	if len(ids) == 0 {
		return map[int64]bool{}, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM answer_options
		WHERE question_id = ? AND deleted_at = ? AND id IN (`+intInList(ids)+`)`,
		questionID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	live := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, internal(err)
		}
		live[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return live, nil
}

// answerUnchanged compares the posted content against the current slot
// content: set equality of the live selections for objectives (answer_text
// must be absent), exact text equality with blank ↔ NULL normalisation for
// essays (ATT-122 storage, ATT-127 idempotency).
func answerUnchanged(ctx context.Context, tx *sql.Tx, slotID int64, p parsedAnswer, essay bool) (bool, error) {
	var text sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT answer_text
		FROM attempt_answers
		WHERE id = ?`,
		slotID,
	).Scan(&text); err != nil {
		return false, internal(err)
	}
	if essay {
		current := ""
		if text.Valid {
			current = text.String
		}
		if strings.TrimSpace(p.essay) == "" {
			return current == "", nil
		}
		return current == p.essay, nil
	}

	if text.Valid && strings.TrimSpace(text.String) != "" {
		return false, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT option_id
		FROM attempt_answer_selections
		WHERE attempt_answer_id = ? AND deleted_at = ?`,
		slotID, store.LiveAt,
	)
	if err != nil {
		return false, internal(err)
	}
	defer rows.Close()
	current := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return false, internal(err)
		}
		current[id] = true
	}
	if err := rows.Err(); err != nil {
		return false, internal(err)
	}
	if len(current) != len(p.optionIDs) {
		return false, nil
	}
	for _, id := range p.optionIDs {
		if !current[id] {
			return false, nil
		}
	}
	return true, nil
}

// answerRedirect builds the ATT-125 target for a save: q is the display
// position of the saved question in the attempt's derived order, n the count.
func answerRedirect(ctx context.Context, tx *sql.Tx, lock attemptLockRow, questionID int64, nav navKey) (AnswerResult, error) {
	// randomize_questions is an immutable setting; snapshot read.
	var random bool
	if err := tx.QueryRowContext(ctx, `
		SELECT randomize_questions
		FROM quizzes
		WHERE id = ? AND deleted_at = ?`,
		lock.quizID, store.LiveAt,
	).Scan(&random); err != nil {
		return AnswerResult{}, internal(err)
	}
	ids, err := derivedQuestionIDs(ctx, tx, lock.quizID, lock.seed, random)
	if err != nil {
		return AnswerResult{}, err
	}
	pos := indexOfID(ids, questionID)
	if pos == 0 {
		// Unreachable: lockSlotAndQuestion proved the question is live in this
		// attempt's quiz, and the same-transaction read derives it. Never
		// renumber silently.
		return AnswerResult{}, integrity(
			"answer redirect: question %d is not in the derived order of quiz %d", questionID, lock.quizID)
	}
	n := len(ids)
	base := attemptPath(lock.attemptID)
	review := base + "/review"
	switch nav {
	case navPrev:
		if pos <= 1 {
			return AnswerResult{Path: base}, nil
		}
		return AnswerResult{Path: qSuffix(base, pos-1)}, nil
	case navNext:
		if pos < n {
			return AnswerResult{Path: qSuffix(base, pos+1)}, nil
		}
		return AnswerResult{Path: review}, nil
	case navReview:
		return AnswerResult{Path: review}, nil
	case navAutosave:
		// Quiet commit: the answer is stored, nothing is rendered (ATT-125).
		return AnswerResult{NoContent: true}, nil
	default: // save
		return AnswerResult{Path: qSuffix(base, pos), Flash: "saved"}, nil
	}
}
