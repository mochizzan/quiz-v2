package participation

// actor.go resolves the viewer of one request (§3.1): a session always wins
// over an identity cookie (PAR-199), and an identity cookie means exactly one
// live anonymous_participants row keyed by SHA-256 of the presented value
// (PAR-005, PAR-INV-01). A stale or corrupt cookie degrades to "visitor", never
// an error (PAR-006).
//
// Resolution takes a Querier, not the pool: commands call it inside the
// transaction that needs the row, so a concurrent profile edit cannot race the
// check (PAR-056).

import (
	"context"
	"database/sql"
	"errors"

	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// resolveActor maps the adapter's raw inputs to an Actor. Role comes from the
// already-validated session; the domain reads the students row itself because
// authn.Session carries no student id (PAR-197).
func resolveActor(ctx context.Context, q store.Querier, in ActorInput) (Actor, error) {
	switch {
	case in.Role == viewmodel.RoleTeacher:
		return Actor{Kind: ActorTeacher, Account: uint32(in.AccountID)}, nil

	case in.Role == viewmodel.RoleStudent:
		var (
			id        int64
			grade     uint8
			majorID   int64
			majorName string
		)
		// authn.Resolve already proved the profile is live (IDN-047); a miss
		// here is inconsistency and answers 500.
		err := q.QueryRowContext(ctx, `
			SELECT st.id, st.grade_level, st.major_id, m.name
			FROM students st
			JOIN majors m ON m.id = st.major_id AND m.deleted_at = ?
			WHERE st.account_id = ? AND st.deleted_at = ?`,
			store.LiveAt, in.AccountID, store.LiveAt,
		).Scan(&id, &grade, &majorID, &majorName)
		if errors.Is(err, sql.ErrNoRows) {
			return Actor{}, internal(errProfileInvalid)
		}
		if err != nil {
			return Actor{}, internal(err)
		}
		return Actor{
			Kind:       ActorStudent,
			Account:    uint32(in.AccountID),
			StudentID:  uint32(id),
			GradeLevel: grade,
			MajorID:    uint32(majorID),
			MajorName:  majorName,
		}, nil
	}

	// No session: the request is a visitor or an anonymous participant.
	if !validToken(in.IdentityToken) {
		return Actor{Kind: ActorVisitor, IdentityStale: in.IdentityToken != ""}, nil
	}
	identity, ok, err := readIdentityByToken(ctx, q, in.IdentityToken)
	if err != nil {
		return Actor{}, err
	}
	if !ok {
		// Unknown hash or soft-deleted row: the browser keeps the cookie but
		// acts as a visitor; GET adapters clear it (PAR-006).
		return Actor{Kind: ActorVisitor, IdentityStale: true}, nil
	}
	return identity.Actor, nil
}

// errProfileInvalid names the impossible student-profile miss for the caller's
// log, never for the response (OPS-204).
var errProfileInvalid = errors.New("participation: authenticated student has no live profile")

// identityRow is the selected anonymous identity plus the raw token the browser
// holds. Parity with Actor keeps callers short.
type identityRow struct {
	Actor
	Token string
}

// readIdentityByToken resolves one presented cookie to its row (PAR-005). ok is
// false for a hash with no live match; there is no partial identity.
func readIdentityByToken(ctx context.Context, q store.Querier, token string) (identityRow, bool, error) {
	row := identityRow{}
	var (
		id          int64
		displayName string
		grade       uint8
		majorID     int64
		majorName   string
	)
	err := q.QueryRowContext(ctx, `
		SELECT ap.id, ap.display_name, ap.grade_level, ap.major_id, m.name
		FROM anonymous_participants ap
		JOIN majors m ON m.id = ap.major_id AND m.deleted_at = ?
		WHERE ap.cookie_token_hash = ? AND ap.deleted_at = ?`,
		store.LiveAt, secure.HashToken(token), store.LiveAt,
	).Scan(&id, &displayName, &grade, &majorID, &majorName)
	if errors.Is(err, sql.ErrNoRows) {
		return identityRow{}, false, nil
	}
	if err != nil {
		return identityRow{}, false, internal(err)
	}
	row.Actor = Actor{
		Kind:              ActorAnonymous,
		IdentityID:        uint32(id),
		IdentityToken:     token,
		DisplayName:       displayName,
		IdentityGrade:     grade,
		IdentityMajor:     uint32(majorID),
		IdentityMajorName: majorName,
	}
	row.Token = token
	return row, true, nil
}
