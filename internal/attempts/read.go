package attempts

// read.go implements the three read-only screens: the one-question form
// (ATT-111…ATT-119), the pre-submit review (ATT-131…ATT-135), and the
// post-submit result (ATT-151…ATT-155). Each runs in one read-only
// transaction, resolves ownership without locks, and loads only the fields
// the screen shows: no key, score, verdict, or is_correct value reaches a
// pre-submit view model (ATT-118). Soft-deleted attempt/registration/quiz is
// 404 on reads (ATT-119, INV-18); the integrity-500 paths are reserved for
// the write commands (ATT-281).

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// Essay textarea constants shared with the demo fixtures (client-side aids;
// the storage bound is ATT-122's 65,535 bytes on the write path).
const (
	essayMaxChars = 2000
	essayRows     = 8
)

// QuestionPage is the page-local wrapper the attempt route renders. The
// shared view model carries no question id or form target, so the route adds
// the two values the hidden input and form need; ClosedNotice carries
// ATT-119's closed-quiz condition.
type QuestionPage struct {
	viewmodel.AttemptQuestion
	QuestionID   int64
	AnswerHref   string
	ClosedNotice bool
}

// QuestionInput is the GET /attempt/:id material.
type QuestionInput struct {
	AttemptID uint32
	QRaw      string // raw q query value; ATT-115 semantics apply
	Owner     ownerRef

	// Draft preserves the posted values of a failed answer save (ATT-126):
	// the question re-renders with the posted selections/text, stored values
	// everywhere else.
	Draft *QuestionDraft
}

// QuestionDraft is one question's posted values for the 422 re-render.
type QuestionDraft struct {
	QuestionID int64 // anything else → the draft is ignored
	Options    []int64
	Essay      string
}

// ReviewInput is GET /attempt/:id/review material.
type ReviewInput struct {
	AttemptID uint32
	Owner     ownerRef
}

// ResultInput is GET /attempt/:id/result material.
type ResultInput struct {
	AttemptID uint32
	Owner     ownerRef
}

// attemptView is the resolved header of one attempt read plus every fact the
// three screens need.
type attemptView struct {
	attemptID      uint32
	quizID         uint32
	registrationID uint32
	attemptNo      int
	seed           uint32
	submittedAt    sql.NullTime
	submitted      bool
	final          sql.NullFloat64
	createdAt      time.Time

	quizTitle          string
	state              viewmodel.QuizState
	allowRetake        bool
	maxAttempts        int
	effectiveMax       int
	showScore          bool
	reviewMode         viewmodel.ReviewMode
	randomizeQuestions bool
	randomizeOptions   bool

	studentID uint32 // registration participant columns
	anonID    uint32
	name      string
	class     string
}

// selectionView is one live option row.
type optionRow struct {
	id        int64
	position  int
	body      string
	isCorrect bool
}

// takeSlot is one live slot as the attempt screens see it. score/feedback and
// the option key column are part of the result's own query only (ATT-118).
type takeSlot struct {
	slotID     int64
	questionID int64
	position   int
	typ        viewmodel.QuestionType
	prompt     string
	imagePath  sql.NullString
	imageAlt   sql.NullString
	imageBytes sql.NullInt64
	answerText sql.NullString
	score      sql.NullFloat64
	feedback   sql.NullString
	selections []int64 // live option ids; set order is storage order
}

// essay reports the question's answer kind.
func (t takeSlot) essay() bool { return t.typ == viewmodel.QuestionEssay }

// slotAnswered implements the answered predicate ATT-116 for one slot: an
// objective slot with at least one live selection, or an essay with non-blank
// text. Ties the count semantics of ATT-241 exactly; the AnsweredCount named
// function below is the single derivation every screen shares.
func slotAnswered(t takeSlot) bool {
	if t.essay() {
		return t.answerText.Valid && strings.TrimSpace(t.answerText.String) != ""
	}
	return len(t.selections) > 0
}

// AnsweredCount is the named AnsweredCount derivation (ATT-019, ATT-241):
// the counted live slots of an attempt that satisfy ATT-116. Every screen
// and service read counts through it, never through a second dialect.
func AnsweredCount(slots []takeSlot) int {
	n := 0
	for i := range slots {
		if slotAnswered(slots[i]) {
			n++
		}
	}
	return n
}

// orderedOptions returns the requested question's options in the order it is
// displayed: DerivedOptionOrder when randomize_options is on, stored
// (position, id) order otherwise (ATT-113). The rows must already be in
// stored order, which the load query guarantees.
func orderedOptions(rows []optionRow, seed uint32, random bool) []optionRow {
	return deriveOptionOrder(rows, seed, random, func(r optionRow) int64 { return r.id })
}

// bodiesOf renders the bodies of the ordered rows whose ids are in set,
// joined with ", " (ATT-153).
func bodiesOf(rows []optionRow, set map[int64]bool) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		if set[r.id] {
			parts = append(parts, r.body)
		}
	}
	return strings.Join(parts, ", ")
}

// questionImage turns the question's image columns into the view model; a
// pathless question has no image.
func questionImage(path sql.NullString, alt sql.NullString, size sql.NullInt64) *viewmodel.QuestionImage {
	if !path.Valid || path.String == "" {
		return nil
	}
	img := &viewmodel.QuestionImage{URL: "/uploads/" + path.String, Alt: alt.String}
	if size.Valid {
		img.SizeLabel = viewmodel.FileSize(size.Int64)
	}
	return img
}

// readAttemptHeader loads the attempt header plus ownership in one plain
// read. A mismatched, soft-deleted, or foreign attempt/registration/quiz is
// ErrNotFound; the acting identity is resolved in the same transaction and
// compared against exactly one of the registration's participant columns.
func readAttemptHeader(ctx context.Context, tx *sql.Tx, attemptID uint32, owner ownerRef) (attemptView, error) {
	var v attemptView
	var (
		rStudentID  sql.NullInt64
		rAnonID     sql.NullInt64
		fullName    sql.NullString
		displayName sql.NullString
		grade       sql.NullInt64
		majorName   sql.NullString
	)
	err := tx.QueryRowContext(ctx, `
		SELECT att.id, r.quiz_id, att.attempt_no, att.shuffle_seed, att.registration_id,
		       att.submitted_at, att.final_score, att.created_at,
		       z.title, z.state, z.allow_retake, z.max_attempts, z.show_score,
		       z.review_mode, z.randomize_questions, z.randomize_options,
		       r.student_id, r.anonymous_participant_id,
		       a.full_name, ap.display_name,
		       COALESCE(COALESCE(s.grade_level, ap.grade_level), 0), mj.name
		FROM quiz_attempts att
		JOIN registrations r ON r.id = att.registration_id AND r.deleted_at = ?
		JOIN quizzes z ON z.id = r.quiz_id AND z.deleted_at = ?
		LEFT JOIN students s ON s.id = r.student_id AND s.deleted_at = ?
		LEFT JOIN accounts a ON a.id = s.account_id AND a.deleted_at = ?
		LEFT JOIN anonymous_participants ap ON ap.id = r.anonymous_participant_id AND ap.deleted_at = ?
		LEFT JOIN majors mj ON mj.id = COALESCE(s.major_id, ap.major_id) AND mj.deleted_at = ?
		WHERE att.id = ? AND att.deleted_at = ?`,
		store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt, store.LiveAt,
		attemptID, store.LiveAt,
	).Scan(
		&v.attemptID, &v.quizID, &v.attemptNo, &v.seed, &v.registrationID,
		&v.submittedAt, &v.final, &v.createdAt,
		&v.quizTitle, &v.state, &v.allowRetake, &v.maxAttempts, &v.showScore,
		&v.reviewMode, &v.randomizeQuestions, &v.randomizeOptions,
		&rStudentID, &rAnonID,
		&fullName, &displayName,
		&grade, &majorName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return attemptView{}, ErrNotFound
	}
	if err != nil {
		return attemptView{}, internal(err)
	}
	v.submitted = v.submittedAt.Valid
	v.effectiveMax = EffectiveMaxAttempts(v.allowRetake, v.maxAttempts)

	// Ownership (ATT-024, ATT-021): the resolved actor must be exactly the
	// registration's participant.
	actor, err := resolveOwnerTx(ctx, tx, owner)
	if err != nil {
		return attemptView{}, err
	}
	switch {
	case rStudentID.Valid:
		v.studentID = uint32(rStudentID.Int64)
		if actor.Kind != ActorStudent || actor.StudentID != v.studentID {
			return attemptView{}, ErrNotFound
		}
		if !fullName.Valid {
			return attemptView{}, integrity(
				"attempt %d student %d has no readable name (ATT-282)", attemptID, v.studentID)
		}
		v.name = fullName.String
	case rAnonID.Valid:
		v.anonID = uint32(rAnonID.Int64)
		if actor.Kind != ActorAnonymous || actor.ParticipantID != v.anonID {
			return attemptView{}, ErrNotFound
		}
		if !displayName.Valid {
			return attemptView{}, integrity(
				"attempt %d anonymous participant %d has no readable name (ATT-282)", attemptID, v.anonID)
		}
		v.name = displayName.String
	default:
		return attemptView{}, integrity(
			"attempt %d registration has no participant", attemptID)
	}
	v.class = viewmodel.ClassLabel(int(grade.Int64), majorName.String)
	return v, nil
}

// attemptHeaderOf renders the shared Att-117 header block.
func (a attemptView) attemptHeaderOf() viewmodel.AttemptHeader {
	return viewmodel.AttemptHeader{
		ID:          int64(a.attemptID),
		QuizID:      int64(a.quizID),
		QuizTitle:   a.quizTitle,
		StudentName: a.name,
		ClassLabel:  a.class,
		StartedAt:   viewmodel.NewLocalTime(a.createdAt),
		AttemptNo:   a.attemptNo,
		MaxAttempts: a.effectiveMax,
	}
}

// attemptHeaderOf is the LocalTime builder used everywhere in the package.
func timeOf(t time.Time) viewmodel.LocalTime {
	return viewmodel.NewLocalTime(t)
}

// loadTakeData reads the slots, options, and selections of a live attempt in
// the forms the pre-submit screens need. Slots arrive in stored question
// order (display order when randomize_questions is off); options arrive in
// stored option order grouped by question; selections arrive grouped by
// slot's question id. No key/score/feedback column is requested (ATT-118).
func loadTakeData(ctx context.Context, tx *sql.Tx, a attemptView) ([]takeSlot, map[int64][]optionRow, map[int64][]int64, error) {
	// Slots in stored order. The display order is derived only when the quiz
	// randomizes (ATT-112).
	slotRows := []takeSlot{}
	{
		rows, err := tx.QueryContext(ctx, `
			SELECT aa.id, aa.question_id, qq.position, qq.type, qq.prompt,
			       qq.image_path, qq.image_alt, qq.image_bytes, aa.answer_text
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
			); err != nil {
				return nil, nil, nil, internal(err)
			}
			slotRows = append(slotRows, sl)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, nil, internal(err)
		}
	}

	// Options of every slotted question, in stored order (ATT-113 input).
	var opts map[int64][]optionRow
	if len(slotRows) > 0 {
		rows, err := tx.QueryContext(ctx, `
			SELECT o.id, o.question_id, o.position, o.body
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
			if err := rows.Scan(&o.id, &qid, &o.position, &o.body); err != nil {
				return nil, nil, nil, internal(err)
			}
			opts[qid] = append(opts[qid], o)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, nil, internal(err)
		}
	}

	// Live selections grouped by the slot's question id.
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

	// Attach the selections to their slots: slotAnswered and the result
	// verdicts read them from the slot, never from a parallel map.
	for i := range slotRows {
		slotRows[i].selections = sels[slotRows[i].questionID]
	}

	// display order (ATT-112) is computed once, in memory.
	if a.randomizeQuestions {
		slotRows = deriveOrder(slotRows, a.seed, true, func(t takeSlot) int64 { return t.questionID })
	}
	return slotRows, opts, sels, nil
}

// slotIDList joins slot question ids for the options IN-list. Slot keys are
// unique per (attempt, question, live), so the list is both complete and
// duplicate-free.
func slotIDList(slots []takeSlot) string {
	ids := make([]int64, 0, len(slots))
	seen := map[int64]bool{}
	for _, t := range slots {
		if !seen[t.questionID] {
			seen[t.questionID] = true
			ids = append(ids, t.questionID)
		}
	}
	if len(ids) == 0 {
		return "0"
	}
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(id, 10))
	}
	return b.String()
}

// resolveTakePosition implements ATT-115's q semantics. Returned value is
// 1-based display position, clamped to [1, n] for numeric q; the fallback is
// the first unanswered position or 1 when everything is answered (or empty).
func resolveTakePosition(raw string, slots []takeSlot) int {
	n := len(slots)
	if n == 0 {
		return 1
	}
	if q, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		if q < 1 {
			return 1
		}
		if q > n {
			return n
		}
		return q
	}
	for i := range slots {
		if !slotAnswered(slots[i]) {
			return i + 1
		}
	}
	return 1
}

// optionsView renders one question's option rows: derived display order,
// letters, checked state from the shown selection ids.
func optionsView(questionID int64, rows []optionRow, seed uint32, random bool, selected []int64) []viewmodel.OptionView {
	ordered := orderedOptions(rows, seed, random)
	set := map[int64]bool{}
	for _, id := range selected {
		set[id] = true
	}
	out := make([]viewmodel.OptionView, len(ordered))
	for i, r := range ordered {
		out[i] = viewmodel.OptionView{
			ID:      r.id,
			Letter:  viewmodel.OptionLetter(i),
			Body:    r.body,
			Checked: set[r.id],
		}
	}
	return out
}

// essayViewOf is the essay textarea model.
func essayViewOf(current viewmodel.QuestionType, text string) *viewmodel.EssayView {
	if current.IsObjective() {
		return nil
	}
	return &viewmodel.EssayView{Value: text, MaxChars: essayMaxChars, Rows: essayRows}
}

// QuestionPage renders GET /attempt/:id (ATT-111–ATT-119).
func (s *Service) QuestionPage(ctx context.Context, in QuestionInput) (QuestionPage, error) {
	var out QuestionPage
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		h, err := readAttemptHeader(ctx, tx, in.AttemptID, in.Owner)
		if err != nil {
			return err
		}
		if h.submitted {
			// ATT-111: a submitted attempt answers 303 to the result, no
			// flash (the POST-side answer_locked flash is separate).
			return Redirect(attemptResultPath(in.AttemptID))
		}
		page, err := createQuestionPage(ctx, tx, h, in)
		if err != nil {
			return err
		}
		out = page
		return nil
	})
	if err != nil {
		return QuestionPage{}, err
	}
	return out, nil
}

// createQuestionPage assembles the form model; the 303 path already exits.
func createQuestionPage(ctx context.Context, tx *sql.Tx, h attemptView, in QuestionInput) (QuestionPage, error) {
	slots, opts, sels, err := loadTakeData(ctx, tx, h)
	if err != nil {
		return QuestionPage{}, err
	}
	_ = tx

	base := attemptPath(h.attemptID)
	pos := resolveTakePosition(in.QRaw, slots)
	nav := make([]viewmodel.QuestionNav, 0, len(slots))
	for i := range slots {
		nav = append(nav, viewmodel.QuestionNav{
			Position: i + 1,
			Href:     qSuffix(base, i+1),
			Answered: slotAnswered(slots[i]),
			Current:  i+1 == pos,
		})
	}

	page := QuestionPage{
		AttemptQuestion: viewmodel.AttemptQuestion{
			Header:     h.attemptHeaderOf(),
			Progress:   viewmodel.AttemptProgress{Answered: AnsweredCount(slots), Total: len(slots)},
			Position:   pos,
			Total:      len(slots),
			Nav:        nav,
			SubmitHref: base + "/review",
			QuizHref:   quizHref(h.quizID),
			IsLast:     pos == len(slots),
			Action:     base + "/answer",
		},
		AnswerHref:   base + "/answer",
		ClosedNotice: h.state == viewmodel.QuizClosed,
	}
	if len(slots) > 0 {
		cur := slots[pos-1]
		page.Type = cur.typ
		page.Body = cur.prompt
		page.Image = questionImage(cur.imagePath, cur.imageAlt, cur.imageBytes)
		page.QuestionID = cur.questionID
		page.RandomizeAnswers = h.randomizeOptions
		page.AnswerHref = base + "/answer?q=" + strconv.Itoa(pos)
		page.PrevHref = ""
		page.NextHref = base + "/review"
		if pos > 1 {
			page.PrevHref = qSuffix(base, pos-1)
		}
		if pos < len(slots) {
			page.NextHref = qSuffix(base, pos+1)
		}
		// Shown selections: the 422 draft wins at its own question.
		pick := sels[cur.questionID]
		essay := ""
		if cur.answerText.Valid {
			essay = cur.answerText.String
		}
		if in.Draft != nil && in.Draft.QuestionID == cur.questionID {
			pick = in.Draft.Options
			essay = in.Draft.Essay
		}
		page.Answer = viewmodel.AnswerDraft{SelectedIDs: pick, EssayText: essay}
		if cur.typ.IsObjective() {
			page.Options = optionsView(cur.questionID, opts[cur.questionID], h.seed, h.randomizeOptions, pick)
		} else {
			page.Essay = essayViewOf(cur.typ, essay)
		}
	}
	return page, nil
}

// ReviewPage renders GET /attempt/:id/review (ATT-131–ATT-135) — every
// question with the saved answer and a link back to it. Pre-submit review is
// independent of review_mode (ATT-132).
func (s *Service) ReviewPage(ctx context.Context, in ReviewInput) (viewmodel.AttemptReview, error) {
	var out viewmodel.AttemptReview
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		h, err := readAttemptHeader(ctx, tx, in.AttemptID, in.Owner)
		if err != nil {
			return err
		}
		if h.submitted {
			// ATT-131: a stale review of a submitted attempt redirects to the
			// result, no flash.
			return Redirect(attemptResultPath(in.AttemptID))
		}
		slots, opts, sels, err := loadTakeData(ctx, tx, h)
		if err != nil {
			return err
		}

		base := attemptPath(h.attemptID)
		rows := make([]viewmodel.ReviewRow, 0, len(slots))
		for i := range slots {
			cur := slots[i]
			rows = append(rows, viewmodel.ReviewRow{
				Position:   i + 1,
				Type:       cur.typ,
				Body:       cur.prompt,
				Answer:     reviewAnswerText(cur, opts[cur.questionID], sels[cur.questionID], h),
				Unanswered: !slotAnswered(cur),
				Href:       qSuffix(base, i+1),
			})
		}
		out.Header = h.attemptHeaderOf()
		out.Progress = viewmodel.AttemptProgress{Answered: AnsweredCount(slots), Total: len(slots)}
		out.Rows = rows
		out.Unanswered = unansweredPositions(slots)
		out.SubmitHref = base + "/submit"
		out.BackHref = base
		return nil
	})
	if err != nil {
		return viewmodel.AttemptReview{}, err
	}
	return out, nil
}

// resultPage holds the fields assembleResult returns together with its
// transaction-only extras.
type resultExtras struct {
	// rows hold per-slot verdicts; unused when the screen is hidden.
}

// ResultPage renders GET /attempt/:id/result (ATT-151–ATT-155). An
// unsubmitted attempt answers 303 to the form with the not_submitted flash;
// foreign or missing is 404. The review mode and show_score alone decide the
// rows (ATT-152).
func (s *Service) ResultPage(ctx context.Context, in ResultInput) (viewmodel.AttemptResult, error) {
	var out viewmodel.AttemptResult
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		h, err := readAttemptHeader(ctx, tx, in.AttemptID, in.Owner)
		if err != nil {
			return err
		}
		if !h.submitted {
			// ATT-151: not submitted → back to the form with the info flash.
			return RedirectFlash(attemptPath(h.attemptID), FlashNotSubmitted)
		}
		res, err := assembleResult(ctx, tx, h)
		if err != nil {
			return err
		}
		out = res
		return nil
	})
	if err != nil {
		return viewmodel.AttemptResult{}, err
	}
	return out, nil
}

// assembleResult builds the post-submit model inside the caller's
// read-only transaction: header facts, slots with stored verdicts, the
// option key, focus count, and remaining-attempt state.
func assembleResult(ctx context.Context, tx *sql.Tx, h attemptView) (viewmodel.AttemptResult, error) {
	slots, opts, sels, err := loadResultData(ctx, tx, h)
	if err != nil {
		return viewmodel.AttemptResult{}, err
	}

	// Count and focus are one row and one aggregate each.
	var attempts int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM quiz_attempts WHERE registration_id = ? AND deleted_at = ?`,
		h.registrationID, store.LiveAt,
	).Scan(&attempts); err != nil {
		return viewmodel.AttemptResult{}, internal(err)
	}
	var focus int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM focus_loss_events WHERE attempt_id = ? AND deleted_at = ?`,
		h.attemptID, store.LiveAt,
	).Scan(&focus); err != nil {
		return viewmodel.AttemptResult{}, internal(err)
	}

	mode := h.reviewMode
	showScore := h.showScore
	waiting := h.final.Valid == false
	var shownScore *viewmodel.Score
	if !waiting && showScore {
		shownScore = &viewmodel.Score{Percent: DisplayPercent(ScoreUnits(h.final.Float64))}
	}

	rows := make([]viewmodel.ResultRow, 0, len(slots))
	for i := range slots {
		cur := slots[i]
		if mode == viewmodel.ReviewKeyOnly && cur.essay() {
			// ATT-152 key_only: essay rows are omitted entirely; the key
			// list is the whole page.
			continue
		}
		sols, orderedOpts := resultOptionSets(cur, opts[cur.questionID], h, sels[cur.questionID])
		_ = sols
		row := viewmodel.ResultRow{
			Position:      i + 1,
			Type:          cur.typ,
			Body:          cur.prompt,
			Image:         questionImage(cur.imagePath, cur.imageAlt, cur.imageBytes),
			Essay:         cur.essay(),
			StudentAnswer: resultAnswerText(cur, orderedOpts, sels[cur.questionID]),
		}
		// The matrix (ATT-152): key and verdict visibility per mode.
		if mode.ShowsQuestions() {
			row.StudentAnswer = resultAnswerText(cur, orderedOpts, sels[cur.questionID])
		}
		if mode.ShowsKey() {
			row.Key = correctKeyText(cur, orderedOpts)
		}
		row.Verdict = resultVerdict(cur, mode)
		if mode == viewmodel.ReviewQuestionsAndKey && cur.essay() && cur.score.Valid {
			// Graded essay: score numeral gated on show_score (ATT-154).
			row.Graded = true
			if showScore {
				row.Score = &viewmodel.Score{Percent: DisplayPercent(ScoreUnits(cur.score.Float64))}
			}
			row.Feedback = cur.feedback.String
		}
		rows = append(rows, row)
	}

	out := viewmodel.AttemptResult{
		Header:       h.attemptHeaderOf(),
		SubmittedAt:  ptrLocalTimeOf(h.submittedAt),
		Score:        shownScore,
		ShowScore:    showScore,
		Graded:       !waiting,
		Waiting:      waiting,
		Mode:         mode,
		ModeLabel:    mode.Label(),
		Rows:         rows,
		Unanswered:   unansweredPositions(slots),
		QuizHref:     quizHref(h.quizID),
		ReviewHref:   "", // ATT-155: this page IS the review; dead anchor stays empty
		HistoryHref:  resultHistoryHref(h, h.quizID),
		FocusLoss:    focus,
		AttemptsUsed: attempts,
		MaxAttempts:  h.effectiveMax,
	}
	// CanRetry (ATT-155): only the remaining-capacity condition; ATT-101
	// re-checks every condition server-side.
	if h.state == viewmodel.QuizOpen && h.allowRetake && (h.effectiveMax == 0 || attempts < h.effectiveMax) {
		out.CanRetry = true
		out.RetryHref = quizHref(h.quizID)
	}
	return out, nil
}

// resultHistoryHref: authenticated students reach the per-quiz history;
// anonymous participants have no history (ATT-155) so the template's always
// rendered link points at the quiz page instead. Flagged template gap.
func resultHistoryHref(h attemptView, quizID uint32) string {
	if h.studentID != 0 {
		return "/student/history/" + strconv.FormatUint(uint64(quizID), 10)
	}
	return quizHref(quizID)
}

// ptrLocalTimeOf lifts the submitted_at column.
func ptrLocalTimeOf(t sql.NullTime) *viewmodel.LocalTime {
	if !t.Valid {
		return nil
	}
	n := timeOf(t.Time)
	return &n
}

// unansweredPositions is the ascending list of unanswered slot positions
// (ATT-133/ATT-154's count source).
func unansweredPositions(slots []takeSlot) []int {
	out := make([]int, 0, len(slots))
	for i := range slots {
		if !slotAnswered(slots[i]) {
			out = append(out, i+1)
		}
	}
	return out
}
