package attempts

import (
	"context"
	"database/sql"
	"errors"
	"regexp"

	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// actor.go resolves the viewer of one attempt-scope request (ATT-024): a
// student session wins over an identity cookie (session rank, ATT-024 via the
// PAR contract), and an identity cookie means exactly one live
// anonymous_participants row keyed by SHA-256 of the presented value.
// Resolution takes a Querier so every command resolves inside the transaction
// that needs the row; an identity change between requests cannot race the
// ownership check.
//
// The cookie contract is PAR's (cookie name quiz_participant, value
// "v1." + 43 unpadded base64url characters). attempts cannot import the
// participation package (domain isolation), so the two constants here mirror
// PAR-005 exactly; internal/secure.HashToken is the shared hash.

// attemptIdentityCookie is the raw cookie value the browser presents; PAR
// mints it and owns its grammar.
const attemptIdentityCookie = "quiz_participant"

// attemptIdentityShape mirrors PAR's token grammar: "v1." plus 43 unpadded
// base64url characters. A value that does not match is treated as a visitor
// (PAR-006), never an error.
var attemptIdentityShape = regexp.MustCompile(`^v1\.[A-Za-z0-9_-]{43}$`)

// ActorKind is the resolved viewer class for one request. A teacher session is
// deliberately a visitor for attempt-scope routes: ATT-021 says a teacher can
// never own an attempt, and ownership verification answers 404 for them
// without a second identity branch.
type ActorKind uint8

const (
	// ActorVisitor has no attempt-scoped identity (plain visitor or teacher).
	ActorVisitor ActorKind = iota
	// ActorStudent is an authenticated student with a live profile.
	ActorStudent
	// ActorAnonymous is a live anonymous identity from the cookie.
	ActorAnonymous
)

// ActorInput is the raw identity material the route adapter supplies: the
// resolved session plus the raw participant cookie. Nothing is trusted beyond
// the session itself (authn already validated it); the domain re-derives the
// student id and the identity row (ATT-024).
type ActorInput struct {
	AccountID     int64
	Role          viewmodel.Role
	IdentityToken string
}

// AttemptActor is a resolved attempt-scoped viewer.
type AttemptActor struct {
	Kind ActorKind

	// StudentID is the students.id row when Kind is ActorStudent.
	StudentID uint32
	// ParticipantID is the ownership key supplied by the registration:
	// student_id for a student, anonymous_participant_id for an anonymous
	// identity. registrations stores its participant as exactly one non-null
	// column, so this is also the value the ownership check compares.
	ParticipantID uint32
	// AccountID is the acting account; for grading routes and the beacon this
	// is what the session contributes.
	AccountID int64
}

// IsStudent reports a live student session.
func (a AttemptActor) IsStudent() bool { return a.Kind == ActorStudent }

// IsAnonymous reports a live anonymous identity.
func (a AttemptActor) IsAnonymous() bool { return a.Kind == ActorAnonymous }

// ResolveActor maps the adapter's raw inputs to an AttemptActor. Role comes
// from the already-validated session; the domain reads the students row itself
// because authn.Session carries no student id. A student session without a
// live profile is a data-integrity 500 (authn.Resolve already proved it live);
// a stale or corrupt cookie degrades to visitor, never an error (PAR-006).
func ResolveActor(ctx context.Context, q store.Querier, in ActorInput) (AttemptActor, error) {
	switch in.Role {
	case viewmodel.RoleStudent:
		var id int64
		err := q.QueryRowContext(ctx, `
			SELECT st.id
			FROM students st
			WHERE st.account_id = ? AND st.deleted_at = ?`,
			in.AccountID, store.LiveAt,
		).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return AttemptActor{}, integrity("authenticated student has no live profile (account %d)", in.AccountID)
		}
		if err != nil {
			return AttemptActor{}, internal(err)
		}
		return AttemptActor{
			Kind:          ActorStudent,
			StudentID:     uint32(id),
			ParticipantID: uint32(id),
			AccountID:     in.AccountID,
		}, nil

	case viewmodel.RoleTeacher:
		// ATT-021: a teacher is never a participant. Resolving them as a
		// visitor makes every attempt-scope ownership check answer 404.
		return AttemptActor{Kind: ActorVisitor, AccountID: in.AccountID}, nil

	default:
		// No session: visitor or anonymous participant.
		if validAttemptToken(in.IdentityToken) {
			var id int64
			err := q.QueryRowContext(ctx, `
				SELECT ap.id
				FROM anonymous_participants ap
				WHERE ap.cookie_token_hash = ? AND ap.deleted_at = ?`,
				secure.HashToken(in.IdentityToken), store.LiveAt,
			).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				return AttemptActor{Kind: ActorVisitor}, nil
			}
			if err != nil {
				return AttemptActor{}, internal(err)
			}
			return AttemptActor{
				Kind:          ActorAnonymous,
				ParticipantID: uint32(id),
			}, nil
		}
		return AttemptActor{Kind: ActorVisitor}, nil
	}
}

// validAttemptToken mirrors PAR-005's grammar check. A malformed cookie
// candidate is a visitor (PAR-006), never an error.
func validAttemptToken(raw string) bool {
	return attemptIdentityShape.MatchString(raw)
}
