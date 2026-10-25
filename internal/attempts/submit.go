package attempts

// submit.go implements the submit command ATT-141…ATT-147: lock the attempt,
// stamp every live objective slot with its ATT-142 score, set submitted_at
// once together with one graded_at, and maintain final_score (ATT-143) in the
// same transaction. A second submit observes the stamp and redirects without
// writing or recomputing (ATT-147); essay slots stay untouched until grading
// (ATT-146). A concurrent answer save either happens before the slot locks
// (and is scored) or waits behind them (ATT-274).

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"quiz-v2/internal/store"
)

// SubmitInput is the actor material for POST /attempt/:id/submit. The form
// carries nothing but csrf_token (ATT-141), so there is no payload to
// validate and no 422 exists on this route.
type SubmitInput struct {
	AttemptID uint32
	Owner     ownerRef
}

// FlashSubmitted is the success flash key of ATT-141's redirect. It is outside
// the conflict set and already registered in viewmodel.FlashFor
// ("Jawaban terkirim.").
const FlashSubmitted = "submitted"

// Submit runs one submit in one write transaction (ATT-144 step 8). A committed
// submission is the nil return: the adapter builds ATT-141's result redirect
// itself. Only the idempotent repeat carries a signal, because that path writes
// nothing and the rollback is therefore free (ATT-147).
func (s *Service) Submit(ctx context.Context, in SubmitInput) error {
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		return submitTx(ctx, tx, in)
	})
}

// submitSlot is one locked live slot of the submit pass, in id order.
type submitSlot struct {
	id         int64
	questionID int64
}

// submitTx is the transaction body of ATT-144, steps 1–7. It returns nil once
// every write is done: a non-nil return rolls the whole submission back in
// store.Tx, and the success redirect is the adapter's job.
func submitTx(ctx context.Context, tx *sql.Tx, in SubmitInput) error {
	// (1) Lock the attempt; re-check ownership and liveness in the same
	// transaction (ATT-024, ATT-281). The quiz may be open or closed (R-10):
	// no state check here on purpose.
	lock, err := lockAttempt(ctx, tx, in.AttemptID, in.Owner)
	if err != nil {
		return err
	}
	if lock.Submitted() {
		// ATT-147: a concurrent or retried submit writes nothing, computes
		// nothing, and redirects to the result without a flash.
		return Redirect(attemptResultPath(lock.attemptID))
	}

	// (2) Lock every live slot ordered by id — the same relative order grading
	// uses (attempt → answer, ATT-144 step 2).
	rows, err := tx.QueryContext(ctx, `
		SELECT aa.id, aa.question_id
		FROM attempt_answers aa
		WHERE aa.attempt_id = ? AND aa.deleted_at = ?
		ORDER BY aa.id
		FOR UPDATE`,
		lock.attemptID, store.LiveAt,
	)
	if err != nil {
		return internal(err)
	}
	var slots []submitSlot
	for rows.Next() {
		var sl submitSlot
		if err := rows.Scan(&sl.id, &sl.questionID); err != nil {
			rows.Close()
			return internal(err)
		}
		slots = append(slots, sl)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return internal(err)
	}
	rows.Close()

	// Snapshot read of the slot question types (immutable facts, ATT-286 —
	// plain reads, never locks). A live slot whose question is missing or
	// soft-deleted is the data-integrity 500 path, mirroring ATT-105.
	types, err := slotTypes(ctx, tx, slots)
	if err != nil {
		return err
	}

	// (3) Score every objective slot from the locked selections and the live
	// options only (ATT-142).
	units, ok, err := scoreSlots(ctx, tx, lock.attemptID, slots, types)
	if err != nil {
		return err
	}

	// (4)(5)(7) One now serves submitted_at and every objective graded_at.
	now, err := nowT(ctx, tx)
	if err != nil {
		return err
	}
	for i, sl := range slots {
		if types[sl.questionID] == "essay" {
			// ATT-144 step 5: essay scores, feedback, and grading columns are
			// left untouched, including the row's own updated_at.
			continue
		}
		// score ∈ {0,1} with graded_at set satisfies chk_answers_grading;
		// graded_by_account_id stays NULL (system scoring).
		if _, err := tx.ExecContext(ctx, `
			UPDATE attempt_answers
			SET score = ?, graded_at = ?, graded_by_account_id = NULL, updated_at = ?
			WHERE id = ?`,
			UnitsDecimal(units[i]), now, now, sl.id,
		); err != nil {
			return internal(err)
		}
	}

	// (6) final_score is defined iff every live slot is scored and n > 0
	// (ATT-143, ATT-INV-06); a graded essay row carried by the row store would
	// already surface through ok below.
	var final sql.NullString
	if allScored := ok && len(slots) > 0; allScored {
		if u, done := FinalScoreUnits(units, len(slots)); done {
			final = sql.NullString{String: UnitsDecimal(u), Valid: true}
		}
	}

	// (7) Stamp the attempt exactly once (ATT-144 step 7). submitted_at is
	// immutable from here on (ATT-146); this is the update the transaction
	// commits with.
	if _, err := tx.ExecContext(ctx, `
		UPDATE quiz_attempts
		SET submitted_at = ?, final_score = ?, updated_at = ?
		WHERE id = ?`,
		now, final, now, lock.attemptID,
	); err != nil {
		return internal(err)
	}

	// Redirect (ATT-141): the first commit carries the success flash. The
	// return is nil, not a redirect signal: store.Tx rolls back on a non-nil
	// error, so a signal here would discard submitted_at and final_score while
	// still sending the student to /result — where ATT-151 would bounce them
	// back to the question form. routes_attempt.go builds the redirect.
	return nil
}

// slotTypes snapshot-reads the type of every slotted question. A missing row
// is an integrity violation, never a silent rescore.
func slotTypes(ctx context.Context, tx *sql.Tx, slots []submitSlot) (map[int64]string, error) {
	out := make(map[int64]string, len(slots))
	if len(slots) == 0 {
		return out, nil
	}
	ids := slotQuestionList(slots)
	rows, err := tx.QueryContext(ctx, `
		SELECT id, type
		FROM questions
		WHERE id IN (`+ids+`)`,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var typ string
		if err := rows.Scan(&id, &typ); err != nil {
			return nil, internal(err)
		}
		out[id] = typ
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	for _, sl := range slots {
		if out[sl.questionID] == "" {
			return nil, integrity(
				"attempt slot %d has no live question %d (ATT-105)", sl.id, sl.questionID)
		}
	}
	return out, nil
}

// scoreSlots computes ATT-142 for every slot and reports whether the full
// set is scored (every live slot, counting pre-scored essay rows). The units
// list is aligned with slots; ok is false when any essay is unscored.
func scoreSlots(ctx context.Context, tx *sql.Tx, attemptID uint32, slots []submitSlot, types map[int64]string) ([]int, bool, error) {
	units := make([]int, len(slots))
	ok := len(slots) > 0

	// Essay rows normally carry NULL (grading happens only after submit);
	// read the stored score so ATT-143's every-slot-scored precondition
	// is observed even in that defensive case.
	for i, sl := range slots {
		if types[sl.questionID] != "essay" {
			continue
		}
		var stored sql.NullFloat64
		if err := tx.QueryRowContext(ctx, `
			SELECT score
			FROM attempt_answers
			WHERE id = ?`,
			sl.id,
		).Scan(&stored); err != nil {
			return nil, false, internal(err)
		}
		if stored.Valid {
			units[i] = ScoreUnits(stored.Float64)
			continue
		}
		// ATT-142: essays are never scored at submit.
		ok = false
		continue
	}

	// Live options of the slotted questions: id → (live, correct).
	options := map[int64][]int64{}
	correct := map[int64]map[int64]bool{}
	rows, err := tx.QueryContext(ctx, `
		SELECT question_id, id, is_correct
		FROM answer_options
		WHERE question_id IN (`+slotQuestionList(slots)+`) AND deleted_at = ?`,
		store.LiveAt,
	)
	if err != nil {
		return nil, false, internal(err)
	}
	for rows.Next() {
		var qid, id int64
		var isCorrect bool
		if err := rows.Scan(&qid, &id, &isCorrect); err != nil {
			rows.Close()
			return nil, false, internal(err)
		}
		options[qid] = append(options[qid], id)
		if isCorrect {
			if correct[qid] == nil {
				correct[qid] = map[int64]bool{}
			}
			correct[qid][id] = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, false, internal(err)
	}
	rows.Close()

	// Live selections of this attempt's slots.
	selections := map[int64][]int64{}
	rows, err = tx.QueryContext(ctx, `
		SELECT sel.attempt_answer_id, sel.option_id
		FROM attempt_answer_selections sel
		JOIN attempt_answers aa ON aa.id = sel.attempt_answer_id
		WHERE aa.attempt_id = ? AND sel.deleted_at = ? AND aa.deleted_at = ?`,
		attemptID, store.LiveAt, store.LiveAt,
	)
	if err != nil {
		return nil, false, internal(err)
	}
	for rows.Next() {
		var slotID, optionID int64
		if err := rows.Scan(&slotID, &optionID); err != nil {
			rows.Close()
			return nil, false, internal(err)
		}
		selections[slotID] = append(selections[slotID], optionID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, false, internal(err)
	}
	rows.Close()

	for i, sl := range slots {
		qtype := types[sl.questionID]
		if qtype == "essay" {
			continue
		}
		// Only live selections of live options count (ATT-142).
		selected := selections[sl.id]
		liveSelected := make([]int64, 0, len(selected))
		for _, id := range selected {
			live, _ := containsID(options[sl.questionID], id)
			if live {
				liveSelected = append(liveSelected, id)
			}
		}
		if objectiveScore(qtype, liveSelected, correct[sl.questionID]) {
			units[i] = scoreScale
		}
	}
	return units, ok, nil
}

// objectiveScore applies the ATT-142 matrix for one objective slot:
// single — exactly one live selection and it is correct; multiple — the live
// selected set equals the live correct set, no extra and no missing.
// Blanks and wrong answers score 0.
func objectiveScore(qtype string, selected []int64, correct map[int64]bool) bool {
	switch qtype {
	case "single":
		if len(selected) != 1 {
			return false
		}
		return correct[selected[0]]
	case "multiple":
		if len(selected) != len(correct) {
			return false
		}
		for _, id := range selected {
			if !correct[id] {
				return false
			}
		}
		return true
	default:
		// slotTypes proved the row is a live question; an unknown type value
		// is a store-level violation. Score 0 and never crash the submit.
		return false
	}
}

// containsID reports whether id is in ids. The option lists are small, so a
// linear scan beats building a set per question for the common single case.
func containsID(ids []int64, id int64) (bool, int) {
	for i, v := range ids {
		if v == id {
			return true, i
		}
	}
	return false, -1
}

// slotQuestionList renders the IN-list for the slot pass: distinct question
// ids in ascending order, safe because attempt_answers keys slots by
// (attempt_id, question_id).
func slotQuestionList(slots []submitSlot) string {
	if len(slots) == 0 {
		return "NULL"
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, sl := range slots {
		if !seen[sl.questionID] {
			seen[sl.questionID] = true
			ids = append(ids, sl.questionID)
		}
	}
	// Ascending keeps the SQL stable across runs.
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(id, 10))
	}
	return b.String()
}
