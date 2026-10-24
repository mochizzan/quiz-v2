package attempts

// lock.go holds the shared locked-row preamble of the write commands and the
// small helpers every attempt transaction needs. Every command takes its
// locks in the global order registration → quiz → attempt → slot → selection
// (ATT-100, ATT-270); the helpers below make that order hard to violate.
//
// FOR UPDATE goes only on the attempt row and the slot row. Quiz content,
// settings, questions, and options are immutable once a quiz leaves draft
// (ATT-286), so plain snapshot reads cannot destabilise a command and never
// introduce a reverse-order lock.

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// OwnerRef names the acting participant to the domain: the session material
// plus the raw identity cookie. The domain re-derives the actual identity
// inside the transaction that needs it (ATT-024) and never trusts an adapter's
// summary of it. For teacher commands the AccountID is all that is needed.
//
// It is exported so the HTTP adapter can build one; the package keeps the
// shorter ownerRef alias for its own signatures.
type OwnerRef struct {
	Role          viewmodel.Role
	AccountID     int64
	IdentityToken string
}

// ownerRef is the package-internal name for OwnerRef.
type ownerRef = OwnerRef

// resolveOwnerTx resolves the actor inside the caller's transaction.
func resolveOwnerTx(ctx context.Context, q store.Querier, ref ownerRef) (AttemptActor, error) {
	return ResolveActor(ctx, q, ActorInput{
		AccountID:     ref.AccountID,
		Role:          ref.Role,
		IdentityToken: ref.IdentityToken,
	})
}

// attemptLockRow is the locked attempt preamble shared by the write commands.
type attemptLockRow struct {
	attemptID      uint32
	quizID         uint32
	seed           uint32
	registrationID uint32
	submittedAt    sql.NullTime
}

// Submitted reports the observed submission state; a caller turns true into
// its own conflict path (answer_locked, submit idempotency).
func (a attemptLockRow) Submitted() bool { return a.submittedAt.Valid }

// lockAttempt locks the attempt row and verifies ATT-024 ownership plus the
// live-registration invariant (ATT-281): a foreign or soft-deleted attempt is
// 404, and a live attempt under a dead registration is a data-integrity 500.
// One locked join means ownership cannot race an identity change.
func lockAttempt(ctx context.Context, tx *sql.Tx, attemptID uint32, owner ownerRef) (attemptLockRow, error) {
	actor, err := resolveOwnerTx(ctx, tx, owner)
	if err != nil {
		return attemptLockRow{}, err
	}
	if actor.Kind == ActorVisitor {
		return attemptLockRow{}, ErrNotFound
	}

	var (
		row       attemptLockRow
		studentID sql.NullInt64
		anonID    sql.NullInt64
		regLive   int64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT r.quiz_id, att.shuffle_seed, att.registration_id,
		       att.submitted_at,
		       r.student_id, r.anonymous_participant_id,
		       r.deleted_at = ?
		FROM quiz_attempts att
		JOIN registrations r ON r.id = att.registration_id
		WHERE att.id = ? AND att.deleted_at = ?
		FOR UPDATE`,
		store.LiveAt, attemptID, store.LiveAt,
	).Scan(
		&row.quizID, &row.seed, &row.registrationID,
		&row.submittedAt,
		&studentID, &anonID, &regLive,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return attemptLockRow{}, ErrNotFound
	}
	if err != nil {
		return attemptLockRow{}, internal(err)
	}
	row.attemptID = attemptID

	if regLive != 1 {
		return attemptLockRow{}, integrity(
			"attempt %d is live under a dead registration %d (ATT-281)", attemptID, row.registrationID)
	}

	// Exactly one participant column is set (registration XOR rule, 0004);
	// the acting identity must be that participant (ATT-024, INV-27).
	switch {
	case studentID.Valid:
		if actor.Kind != ActorStudent || uint32(studentID.Int64) != actor.StudentID {
			return attemptLockRow{}, ErrNotFound
		}
	case anonID.Valid:
		if actor.Kind != ActorAnonymous || uint32(anonID.Int64) != actor.ParticipantID {
			return attemptLockRow{}, ErrNotFound
		}
	default:
		return attemptLockRow{}, integrity(
			"registration %d has no participant (attempt %d ownership)", row.registrationID, attemptID)
	}
	return row, nil
}

// u32path renders an id for a URL path segment.
func u32path(id uint32) string { return strconv.FormatUint(uint64(id), 10) }

// attemptPath is the canonical attempt/question URL.
func attemptPath(id uint32) string { return "/attempt/" + u32path(id) }

// attemptResultPath is the canonical result URL. viewmodel.ResultHref is the
// int64 twin; the packages must not bridge each other, so the uint32 variant
// lives here.
func attemptResultPath(id uint32) string { return attemptPath(id) + "/result" }

// qSuffix appends one display position to a URL.
func qSuffix(path string, q int) string { return path + "?q=" + strconv.Itoa(q) }

// escapeLike escapes % and _ so a search term stays literal (ATT-172, ATT-248).
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// intInList joins validated int64 ids as the value list of an IN (...) clause.
func intInList(ids []int64) string {
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(id, 10))
	}
	return b.String()
}

// AnsweredSlot fragments (ATT-116, ATT-241): the predicate of a live slot that
// counts as answered. The sentinel literal is store.LiveAt spelled out because
// fragments cannot carry parameters. A selection whose option is soft-deleted
// still counts as a selection (ATT-241).
const essayAnsweredSQL = "(qq.type = 'essay' AND aa.answer_text IS NOT NULL AND TRIM(aa.answer_text) <> '')"

const objectiveAnsweredSQL = "(qq.type <> 'essay' AND EXISTS (" +
	"SELECT 1 FROM attempt_answer_selections sel" +
	" WHERE sel.attempt_answer_id = aa.id" +
	" AND sel.deleted_at = '" + store.LiveAt + "'))"
