package attempts

// beacon.go implements ATT-156..ATT-160: the focus-loss and progress beacon.
// It is the only ATT route without a CSRF token (navigator.sendBeacon cannot
// set one), which is why the HTTP layer enforces same-origin instead (ATT-159).
//
// The beacon is informational only (ATT-039): progress touches updated_at and
// focus appends one event row. It never changes an answer, a score, an attempt
// state, or submission eligibility. A beacon that arrives after submit is a
// 204 no-op and must not consume rate-limit budget (ATT-160).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mime"
	"strconv"
	"strings"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
)

// BeaconKind is the closed input vocabulary of ATT-157.
type BeaconKind string

const (
	// BeaconProgress is the periodic "still here" touch.
	BeaconProgress BeaconKind = "progress"
	// BeaconBlur is a window blur.
	BeaconBlur BeaconKind = "blur"
	// BeaconHidden is a visibilitychange to hidden.
	BeaconHidden BeaconKind = "hidden"
	// BeaconFullscreenExit is a fullscreen exit.
	BeaconFullscreenExit BeaconKind = "fullscreen_exit"
)

// The sliding windows and limits are fixed by ATT-158. Focus kinds share one
// bucket; progress has its own.
const (
	BeaconWindow        = time.Minute
	BeaconProgressLimit = 30
	BeaconFocusLimit    = 10
)

// ErrBeaconKind is the domain guard behind the parser. The HTTP adapter maps
// it to the beacon's empty 400 (ATT-157).
var ErrBeaconKind = errors.New("attempts: unknown beacon kind")

// Valid reports whether k is one of the four accepted kinds.
func (k BeaconKind) Valid() bool {
	switch k {
	case BeaconProgress, BeaconBlur, BeaconHidden, BeaconFullscreenExit:
		return true
	default:
		return false
	}
}

// bucket returns the ATT-158 bucket and its per-minute limit.
func (k BeaconKind) bucket() (string, int, bool) {
	switch k {
	case BeaconProgress:
		return "progress", BeaconProgressLimit, true
	case BeaconBlur, BeaconHidden, BeaconFullscreenExit:
		return "focus", BeaconFocusLimit, true
	default:
		return "", 0, false
	}
}

// ParseBeacon reads the ATT-157 input forms: text/plain whose trimmed body is
// the kind, or application/json with a kind field. Unknown JSON fields are
// ignored. Every malformed shape is (zero, false); the caller answers an empty
// 400 and never a 422 page.
func ParseBeacon(contentType string, body []byte) (BeaconKind, bool) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", false
	}
	switch mediaType {
	case "text/plain":
		kind := BeaconKind(strings.TrimSpace(string(body)))
		if !kind.Valid() {
			return "", false
		}
		return kind, true
	case "application/json":
		var payload struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", false
		}
		kind := BeaconKind(strings.TrimSpace(payload.Kind))
		if !kind.Valid() {
			return "", false
		}
		return kind, true
	default:
		return "", false
	}
}

// BeaconInput is POST /attempt/:id/beacon material. Owner is re-resolved inside
// the write transaction, so the ownership check cannot race an identity change
// (ATT-024).
type BeaconInput struct {
	AttemptID uint32
	Owner     OwnerRef
	Kind      BeaconKind
}

// BeaconResult reports the one benign no-op: the attempt was already submitted
// (or a concurrent submit won the race). The adapter answers 204 either way.
type BeaconResult struct {
	Noop bool
}

// WithLimiter attaches the process-local limiter. The HTTP layer passes its
// shared instance; a nil limiter fails closed with a 500 rather than silently
// disabling ATT-158.
func (s *Service) WithLimiter(l *secure.Limiter) *Service {
	s.limiter = l
	return s
}

// Beacon applies one ATT-156 beacon in a write transaction. The submitted check
// precedes the limiter on purpose: ATT-160 says a queued beacon that lands
// after submit neither writes nor consumes budget.
func (s *Service) Beacon(ctx context.Context, in BeaconInput) (BeaconResult, error) {
	var res BeaconResult
	if s.limiter == nil {
		return res, internal(errors.New("attempts: no beacon limiter"))
	}
	if in.AttemptID == 0 {
		return res, ErrNotFound
	}
	bucket, limit, ok := in.Kind.bucket()
	if !ok {
		return res, ErrBeaconKind
	}
	key := "beacon:" + strconv.FormatUint(uint64(in.AttemptID), 10) + ":" + bucket

	err := s.writeTx(ctx, func(tx *sql.Tx) error {
		actor, err := resolveOwnerTx(ctx, tx, in.Owner)
		if err != nil {
			return err
		}
		if actor.Kind == ActorVisitor {
			return ErrNotFound
		}

		var (
			submitted sql.NullTime
			studentID sql.NullInt64
			anonID    sql.NullInt64
			regLive   int64
		)
		err = tx.QueryRowContext(ctx, `
			SELECT att.submitted_at, r.student_id, r.anonymous_participant_id,
			       r.deleted_at = ?
			FROM quiz_attempts att
			JOIN registrations r ON r.id = att.registration_id
			WHERE att.id = ? AND att.deleted_at = ?`,
			store.LiveAt, in.AttemptID, store.LiveAt,
		).Scan(&submitted, &studentID, &anonID, &regLive)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return internal(err)
		}
		if regLive != 1 {
			return integrity("beacon on attempt %d under a dead registration (ATT-281)", in.AttemptID)
		}

		switch {
		case studentID.Valid:
			if actor.Kind != ActorStudent || uint32(studentID.Int64) != actor.StudentID {
				return ErrNotFound
			}
		case anonID.Valid:
			if actor.Kind != ActorAnonymous || uint32(anonID.Int64) != actor.ParticipantID {
				return ErrNotFound
			}
		default:
			return integrity("beacon registration has no participant (attempt %d)", in.AttemptID)
		}

		if submitted.Valid {
			res.Noop = true
			return nil
		}

		allowed, retry := s.limiter.Allow(key, limit, BeaconWindow)
		if !allowed {
			return apperr.RateLimitedf(retry).WithRule("ATT-158")
		}

		switch in.Kind {
		case BeaconProgress:
			// One statement; the submitted guard closes the read-to-write race
			// with a concurrent submit (ATT-160).
			result, err := tx.ExecContext(ctx, `
				UPDATE quiz_attempts
				SET updated_at = UTC_TIMESTAMP()
				WHERE id = ? AND deleted_at = ? AND submitted_at IS NULL`,
				in.AttemptID, store.LiveAt)
			if err != nil {
				return internal(err)
			}
			n, err := result.RowsAffected()
			if err != nil {
				return internal(err)
			}
			if n == 0 {
				res.Noop = true
			}
		default:
			// One statement; occurred_at is the server clock, never the
			// untrusted client `at` (ATT-157, ATT-160).
			result, err := tx.ExecContext(ctx, `
				INSERT INTO focus_loss_events (attempt_id, kind, occurred_at)
				SELECT att.id, ?, UTC_TIMESTAMP()
				FROM quiz_attempts att
				WHERE att.id = ? AND att.deleted_at = ? AND att.submitted_at IS NULL`,
				string(in.Kind), in.AttemptID, store.LiveAt)
			if err != nil {
				return internal(err)
			}
			n, err := result.RowsAffected()
			if err != nil {
				return internal(err)
			}
			if n == 0 {
				res.Noop = true
			}
		}
		return nil
	})
	return res, err
}
