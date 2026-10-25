package attempts

// start.go implements the start command ATT-101…ATT-110, the one ATT rule PAR
// calls from POST /quiz/:id/attempts. PAR owns the route, the registration,
// and the transaction retry (their TxRetry); ATT is handed the open
// transaction, locks the registration row first (ATT-104), and owns the
// attempts row and slot inserts. All errors are typed: ErrNotFound is the 404
// page, the three start sentinels map through StartFlashKey, anything else is
// 500 with a full rollback. A failed start writes nothing, not even a seed.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"

	"quiz-v2/internal/store"
)

// StartInput is the typed handoff from PAR (ATT-101, ATT-280). AttemptNo is
// advisory: PAR re-derives it for its own logging, but ATT derives the value
// itself from the locked registration's live attempts (ATT-104 step 4) and
// never trusts a caller-supplied number.
type StartInput struct {
	RegistrationID         uint32
	QuizID                 uint32
	AttemptNo              int
	StudentID              uint32
	AnonymousParticipantID uint32
}

// StartResult reports one completed start.
type StartResult struct {
	// AttemptID is the same id a resume, a retry, or a second concurrent
	// start returns (ATT-108, ATT-273).
	AttemptID uint32
	// Created is true for a fresh attempt, false for a resume (which wrote
	// nothing, ATT-109).
	Created bool
}

// Start runs the rule inside PAR's transaction and returns the attempt. It
// never commits or rolls back its caller's transaction.
func Start(ctx context.Context, tx *sql.Tx, in StartInput) (StartResult, error) {
	// (2) Lock the registration row for update. A row that is absent,
	// soft-deleted, or pointed at another quiz rolls back with 404 (ATT-104
	// step 2). Exactly one participant column is set (chk_reg_participant), so
	// both are read as nullable and the ownership check below branches on
	// which one is present.
	var (
		regStudentID sql.NullInt64
		regAnonID    sql.NullInt64
		regQuizID    int64
	)
	err := tx.QueryRowContext(ctx, `
		SELECT student_id, anonymous_participant_id, quiz_id
		FROM registrations
		WHERE id = ? AND deleted_at = ?
		FOR UPDATE`,
		in.RegistrationID, store.LiveAt,
	).Scan(&regStudentID, &regAnonID, &regQuizID)
	if errors.Is(err, sql.ErrNoRows) {
		return StartResult{}, ErrNotFound
	}
	if err != nil {
		return StartResult{}, internal(err)
	}
	if uint32(regQuizID) != in.QuizID {
		return StartResult{}, ErrNotFound
	}

	// Ownership re-check (ATT-101, INV-27): the caller must be the
	// registration's participant. registrations stores exactly one participant
	// column, so exactly one input id must match.
	switch {
	case regStudentID.Valid:
		if in.StudentID == 0 || uint32(regStudentID.Int64) != in.StudentID {
			return StartResult{}, ErrNotFound
		}
	case regAnonID.Valid:
		if in.AnonymousParticipantID == 0 || uint32(regAnonID.Int64) != in.AnonymousParticipantID {
			return StartResult{}, ErrNotFound
		}
	default:
		return StartResult{}, integrity("registration %d has no participant", in.RegistrationID)
	}

	// (3) The quiz row must be live. It is read with a shared lock inside the
	// registration lock so a publish never applies and a close cannot race the
	// state check (ATT-104 step 3).
	var (
		state              string
		allowRetake        bool
		maxAttempts        int
		randomizeQuestions bool
	)
	err = tx.QueryRowContext(ctx, `
		SELECT state, allow_retake, max_attempts, randomize_questions
		FROM quizzes
		WHERE id = ? AND deleted_at = ?
		LOCK IN SHARE MODE`,
		in.QuizID, store.LiveAt,
	).Scan(&state, &allowRetake, &maxAttempts, &randomizeQuestions)
	if errors.Is(err, sql.ErrNoRows) {
		return StartResult{}, ErrNotFound
	}
	if err != nil {
		return StartResult{}, internal(err)
	}

	// (4) Read the live attempts once. Any in-progress attempt is resumed with
	// no writes, and the quiz state is deliberately not checked on this path:
	// a closed quiz still resumes and accepts answers (ATT-104 step 4, R-10,
	// AT-02). Resume does not touch updated_at, does not re-seed, and does not
	// re-evaluate the quota (ATT-109).
	attempts, err := liveAttempts(ctx, tx, in.RegistrationID)
	if err != nil {
		return StartResult{}, err
	}
	for _, a := range attempts {
		if a.inFlight {
			return StartResult{AttemptID: a.id, Created: false}, nil
		}
	}

	// (5) A new attempt requires state = open.
	if state != "open" {
		return StartResult{}, ErrNotOpen
	}

	// (6) At least one live question is required (ATT-104 step 6, ATT-106).
	var questionCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM questions
		WHERE quiz_id = ? AND deleted_at = ?`,
		in.QuizID, store.LiveAt,
	).Scan(&questionCount); err != nil {
		return StartResult{}, internal(err)
	}
	if questionCount == 0 {
		return StartResult{}, ErrNoQuestions
	}

	// (7) The quota is evaluated against the count of live attempts (ATT-102,
	// ATT-260). max_attempts bounds rows, not resumes (ATT-109).
	limit := EffectiveMaxAttempts(allowRetake, maxAttempts)
	if limit > 0 && len(attempts) >= limit {
		return StartResult{}, ErrAttemptLimit
	}

	// The seed is 4 crypto/rand bytes interpreted big-endian; it is not
	// derived from time, user id, quiz id, or attempt number (ATT-103).
	seed, err := secureSeed()
	if err != nil {
		return StartResult{}, internal(err)
	}

	// (8) Insert the attempt. attempt_no is 1 + count(live attempts); the
	// registration lock serializes the derivation and uq_attempt_number is
	// only a backstop (ATT-108, ATT-272).
	attemptNo := clampAttemptNo(len(attempts) + 1)
	res, err := tx.ExecContext(ctx, `
		INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed)
		VALUES (?, ?, ?)`,
		in.RegistrationID, attemptNo, seed,
	)
	if err != nil {
		// A duplicate here means a writer bypassed the registration lock. The
		// same-transaction re-read finds the winner's attempt and returns it
		// so the rule stays total (attention: PAR's TxRetry does not retry a
		// duplicate). No live row means the collision cannot be explained by
		// a live attempt; answer 500, it signals a defect (ATT-108).
		if store.IsDuplicate(err) {
			var winner uint32
			err2 := tx.QueryRowContext(ctx, `
				SELECT id
				FROM quiz_attempts
				WHERE registration_id = ? AND deleted_at = ? AND attempt_no = ?
				ORDER BY id
				LIMIT 1`,
				in.RegistrationID, store.LiveAt, attemptNo,
			).Scan(&winner)
			if errors.Is(err2, sql.ErrNoRows) {
				return StartResult{}, internal(err)
			}
			if err2 != nil {
				return StartResult{}, internal(err2)
			}
			return StartResult{AttemptID: winner, Created: true}, nil
		}
		return StartResult{}, internal(err)
	}
	lastID, err := res.LastInsertId()
	if err != nil {
		return StartResult{}, internal(err)
	}
	attemptID := uint32(lastID)

	// (9) One slot per live question, in the derived order the student walks
	// (ATT-105).
	ids, err := derivedQuestionIDs(ctx, tx, in.QuizID, seed, randomizeQuestions)
	if err != nil {
		return StartResult{}, err
	}
	if len(ids) != questionCount {
		return StartResult{}, integrity("start measured %d live questions and derived %d (quiz %d)", questionCount, len(ids), in.QuizID)
	}
	if err := insertSlots(ctx, tx, attemptID, ids); err != nil {
		return StartResult{}, err
	}

	return StartResult{AttemptID: attemptID, Created: true}, nil
}

// attemptRow is one live attempt of a registration.
type attemptRow struct {
	id       uint32
	inFlight bool // submitted_at IS NULL
}

// liveAttempts reads the live attempts of one registration ordered by
// attempt_no (the order the schema's unique key enforces).
func liveAttempts(ctx context.Context, tx *sql.Tx, registrationID uint32) ([]attemptRow, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, submitted_at IS NULL
		FROM quiz_attempts
		WHERE registration_id = ? AND deleted_at = ?
		ORDER BY attempt_no, id`,
		registrationID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []attemptRow{}
	for rows.Next() {
		var a attemptRow
		if err := rows.Scan(&a.id, &a.inFlight); err != nil {
			return nil, internal(err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return out, nil
}

// derivedQuestionIDs returns the live question ids of one quiz in the
// attempt's display order (ATT-112): position asc, id asc when randomization
// is off, the keyed digest order when on.
func derivedQuestionIDs(ctx context.Context, q store.Querier, quizID uint32, seed uint32, random bool) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id
		FROM questions
		WHERE quiz_id = ? AND deleted_at = ?
		ORDER BY position, id`,
		quizID, store.LiveAt,
	)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, internal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	return deriveOrder(ids, seed, random, func(id int64) int64 { return id }), nil
}

// insertSlots creates one attempt_answers row per live question, in the given
// display order (ATT-105). Slots are never created or deleted after start.
func insertSlots(ctx context.Context, tx *sql.Tx, attemptID uint32, questionIDs []int64) error {
	for _, qid := range questionIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO attempt_answers (attempt_id, question_id)
			VALUES (?, ?)`,
			attemptID, qid,
		); err != nil {
			return internal(err)
		}
	}
	return nil
}

// secureSeed draws 4 crypto/rand bytes (ATT-103). A read failure is a 500 and
// rolls the start back (ATT-107).
func secureSeed() (uint32, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b[:]), nil
}

// clampAttemptNo keeps attempt_no inside the CHECK range (1..255)
// defensively; the flow cannot reach 256 rows for one registration.
func clampAttemptNo(n int) int {
	if n < 1 {
		return 1
	}
	if n > 255 {
		return 255
	}
	return n
}

// EffectiveMaxAttempts implements ATT-102: allow_retake = false → 1;
// allow_retake = true and max_attempts = 0 → unlimited (0 as the sentinel;
// defensive, AUT never writes 0); > 10 clamps to 10; otherwise the stored
// number. It reads the quiz row from its caller, never a form value.
func EffectiveMaxAttempts(allowRetake bool, maxAttempts int) int {
	if !allowRetake {
		return 1
	}
	if maxAttempts <= 0 {
		return 0
	}
	if maxAttempts > 10 {
		return 10
	}
	return maxAttempts
}
