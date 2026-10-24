package attempts

// result.go completes the read side (ATT-151–ATT-155): the result-only data
// load — the per-answer score, teacher feedback, and option key are never part
// of a pre-submit read (ATT-118) — the ATT-153 text derivations, and the quiz
// path the result chrome links back to.

import (
	"context"
	"database/sql"
	"strings"

	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// quizHref is the public quiz page the participant came from (ATT-155).
func quizHref(quizID uint32) string { return "/quiz/" + u32path(quizID) }

// loadResultData is loadTakeData plus the columns only a submitted attempt may
// see: score, feedback, and the option key flag. Slot order and selection
// placement follow the pre-submit loader exactly, so a row's position is the
// same on every screen.
func loadResultData(ctx context.Context, tx *sql.Tx, a attemptView) ([]takeSlot, map[int64][]optionRow, map[int64][]int64, error) {
	slotRows := []takeSlot{}
	rows, err := tx.QueryContext(ctx, `
		SELECT aa.id, aa.question_id, qq.position, qq.type, qq.prompt,
		       qq.image_path, qq.image_alt, qq.image_bytes, aa.answer_text,
		       aa.score, aa.feedback
		FROM attempt_answers aa
		JOIN questions qq ON qq.id = aa.question_id AND qq.deleted_at = ?
		WHERE aa.attempt_id = ? AND aa.deleted_at = ?
		ORDER BY qq.position, qq.id`,
		store.LiveAt, a.attemptID, store.LiveAt,
	)
	if err != nil {
		return nil, nil, nil, internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var sl takeSlot
		if err := rows.Scan(
			&sl.slotID, &sl.questionID, &sl.position, &sl.typ, &sl.prompt,
			&sl.imagePath, &sl.imageAlt, &sl.imageBytes, &sl.answerText,
			&sl.score, &sl.feedback,
		); err != nil {
			return nil, nil, nil, internal(err)
		}
		slotRows = append(slotRows, sl)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, internal(err)
	}

	var opts map[int64][]optionRow
	if len(slotRows) > 0 {
		rows, err := tx.QueryContext(ctx, `
			SELECT o.id, o.question_id, o.position, o.body, o.is_correct
			FROM answer_options o
			WHERE o.question_id IN (`+slotIDList(slotRows)+`) AND o.deleted_at = ?
			ORDER BY o.question_id, o.position, o.id`,
			store.LiveAt,
		)
		if err != nil {
			return nil, nil, nil, internal(err)
		}
		defer rows.Close()
		opts = map[int64][]optionRow{}
		for rows.Next() {
			var o optionRow
			var qid int64
			if err := rows.Scan(&o.id, &qid, &o.position, &o.body, &o.isCorrect); err != nil {
				return nil, nil, nil, internal(err)
			}
			opts[qid] = append(opts[qid], o)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, nil, internal(err)
		}
	}

	sels := map[int64][]int64{}
	if len(slotRows) > 0 {
		rows, err := tx.QueryContext(ctx, `
			SELECT aa.question_id, sel.option_id
			FROM attempt_answer_selections sel
			JOIN attempt_answers aa ON aa.id = sel.attempt_answer_id AND aa.deleted_at = ?
			WHERE aa.attempt_id = ? AND sel.deleted_at = ?
			ORDER BY sel.option_id`,
			store.LiveAt, a.attemptID, store.LiveAt,
		)
		if err != nil {
			return nil, nil, nil, internal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var qid, optID int64
			if err := rows.Scan(&qid, &optID); err != nil {
				return nil, nil, nil, internal(err)
			}
			sels[qid] = append(sels[qid], optID)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, nil, internal(err)
		}
	}

	for i := range slotRows {
		slotRows[i].selections = sels[slotRows[i].questionID]
	}
	if a.randomizeQuestions {
		slotRows = deriveOrder(slotRows, a.seed, true, func(t takeSlot) int64 { return t.questionID })
	}
	return slotRows, opts, sels, nil
}

// resultOptionSets returns the selected set and the option rows in display
// order (ATT-153's DerivedOptionOrder). The caller only needs the ordered
// rows; the set keeps the helper symmetrical with the pre-submit loader.
func resultOptionSets(cur takeSlot, opts []optionRow, h attemptView, sel []int64) (map[int64]bool, []optionRow) {
	ordered := orderedOptions(opts, h.seed, h.randomizeOptions)
	if cur.essay() {
		return nil, ordered
	}
	set := map[int64]bool{}
	for _, id := range sel {
		set[id] = true
	}
	return set, ordered
}

// resultAnswerText is ATT-153's student answer: selected bodies in display
// order joined with ", ", the essay text, or "-".
func resultAnswerText(cur takeSlot, ordered []optionRow, sel []int64) string {
	if cur.essay() {
		if cur.answerText.Valid && strings.TrimSpace(cur.answerText.String) != "" {
			return cur.answerText.String
		}
		return "-"
	}
	set := map[int64]bool{}
	for _, id := range sel {
		set[id] = true
	}
	if text := bodiesOf(ordered, set); text != "" {
		return text
	}
	return "-"
}

// reviewAnswerText is the pre-submit counterpart: the same derivation over
// the attempt's derived option order, without key or verdict columns.
func reviewAnswerText(cur takeSlot, opts []optionRow, sel []int64, h attemptView) string {
	return resultAnswerText(cur, orderedOptions(opts, h.seed, h.randomizeOptions), sel)
}

// correctKeyText is ATT-153's key: the correct option bodies in display order
// joined with ", ". Essays have no key.
func correctKeyText(cur takeSlot, ordered []optionRow) string {
	if cur.essay() {
		return ""
	}
	set := map[int64]bool{}
	for _, o := range ordered {
		if o.isCorrect {
			set[o.id] = true
		}
	}
	return bodiesOf(ordered, set)
}

// resultVerdict derives the verdict from the stored score, never by
// recomputing anything (ATT-153), and applies the ATT-152 visibility matrix:
// only questions_and_key and questions_only carry a verdict (and with it the
// colour). A missing score on an answered objective row is treated as
// ungraded rather than inventing a zero.
func resultVerdict(cur takeSlot, mode viewmodel.ReviewMode) string {
	if mode != viewmodel.ReviewQuestionsAndKey && mode != viewmodel.ReviewQuestionsOnly {
		return ""
	}
	if !slotAnswered(cur) {
		return "unanswered"
	}
	if cur.essay() {
		if !cur.score.Valid {
			return "ungraded"
		}
		return "graded"
	}
	if !cur.score.Valid {
		return "ungraded"
	}
	if cur.score.Float64 >= 0.5 {
		return "correct"
	}
	return "incorrect"
}
