// Package participation implements the PAR domain: anonymous identity, join
// and invitation entry, registrations, cancellation, eligibility, the student
// dashboard/history read models, anonymous-participant management, and the
// major catalogue.
//
// The package owns every write to registrations, anonymous_participants, and
// majors. It reads quizzes/questions/attempts because the schema has no
// cross-domain read seam; it never writes those tables (PAR-025, PAR-200,
// PAR-202).
package participation

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// Service binds the participation domain to one database pool and the raw
// session signing key. The key is used for the join-ticket HMAC (PAR-015) and
// the derived anonymous identity token (PAR-019); it never leaves the process.
// A nil pool is tolerated at construction so the demo build can still register
// the route table; every method that needs the database answers 500 instead of
// panicking.
type Service struct {
	db *store.DB
	// key is the SESSION_SIGNING_KEY material for PAR-015/PAR-019.
	key []byte
	// AttemptStart is ATT's attempt-start rule (PAR-202). When nil, a first
	// start answers 500 with the unimplemented-seam error instead of
	// inserting anything PAR is not allowed to insert.
	AttemptStart AttemptStart
	// Logger receives the domain's ERROR/WARN lines (corrupt quiz rows, the
	// fail-closed quota normalization). Nil loggers are skipped.
	Logger *slog.Logger
	// nowFn is the app clock (not a DB stamp): ticket windows only (PAR-015).
	nowFn func() time.Time
}

// New returns the participation service.
func New(db *store.DB, signingKey []byte) *Service {
	return &Service{
		db:    db,
		key:   signingKey,
		nowFn: func() time.Time { return time.Now().UTC() },
	}
}

// now is the app clock. Token windows are app-clock by design (PAR-015); row
// stamps keep using UTC_TIMESTAMP through nowT.
func (s *Service) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn().UTC()
	}
	return time.Now().UTC()
}

// warn logs one warn-level domain line when a logger is attached.
func (s *Service) warn(rule, msg string, err error) {
	if s.Logger != nil {
		s.Logger.Warn(msg, "rule", rule, "error", err)
	}
}

// oops logs one error-level domain line when a logger is attached.
func (s *Service) oops(rule, msg string, err error) {
	if s.Logger != nil {
		s.Logger.Error(msg, "rule", rule, "error", err)
	}
}

// Result is a legal command outcome that ends in a 303: a target path and an
// optional closed flash key. A blocked transition is a Result, not an error
// (PAR-067, PAR-076, PAR-081).
type Result struct {
	Path  string
	Flash string
}

// Redirect is a GET outcome that must answer 303 instead of rendering. The
// route adapter maps it to Ctx.SeeOther (PAR-045, PAR-052, PAR-090).
type Redirect struct {
	Path  string
	Flash string
}

// Error implements error.
func (r *Redirect) Error() string { return "participation: redirect to " + r.Path }

// Cookies is the cookie delta one command wants applied. The adapter writes
// them only after the command succeeded (PAR-058, §9.1.5): a 422, 404 or 429
// never touches a cookie, so a blocked retry keeps its ticket (§8.7.3).
type Cookies struct {
	// IdentityToken (re)sets quiz_participant with a fresh 365-day slide.
	IdentityToken string
	// IdentityClear expires quiz_participant (stale on GET, or the explicit
	// identity switch PAR-052).
	IdentityClear bool
	// TicketValue (re)sets quiz_join_ticket (PAR-014).
	TicketValue string
	// TicketClear expires quiz_join_ticket (PAR-017, PAR-020).
	TicketClear bool
}

// Out is one POST command's combined outcome: the 303 plus the cookie delta.
type Out struct {
	Result  Result
	Cookies Cookies
}

// ActorKind is the resolved viewer class for one request.
type ActorKind uint8

const (
	// ActorVisitor has neither a session nor a live identity cookie.
	ActorVisitor ActorKind = iota
	// ActorStudent is an authenticated student with a live profile.
	ActorStudent
	// ActorAnonymous is a live anonymous identity with no session.
	ActorAnonymous
	// ActorTeacher is an authenticated teacher.
	ActorTeacher
)

// ActorInput is the raw identity material the adapter supplies: the resolved
// session account/role/name plus the raw participant and join-ticket cookies.
// The domain resolves it inside the transaction that needs it so a profile or
// identity change cannot race the check (PAR-056, PAR-198).
type ActorInput struct {
	AccountID     int64
	Role          viewmodel.Role
	FullName      string
	IdentityToken string
	TicketValue   string
}

// Actor is a resolved viewer. A session always outranks an identity cookie
// (PAR-199): when Role is student or teacher, the identity fields are empty.
type Actor struct {
	Kind    ActorKind
	Account uint32

	StudentID  uint32
	GradeLevel uint8
	MajorID    uint32
	MajorName  string

	IdentityID        uint32
	IdentityToken     string
	DisplayName       string
	IdentityGrade     uint8
	IdentityMajor     uint32
	IdentityMajorName string

	// IdentityStale reports that a cookie was presented but no live row
	// matched. The GET adapters clear it (PAR-006).
	IdentityStale bool
}

// IsStudent reports a live student session.
func (a Actor) IsStudent() bool { return a.Kind == ActorStudent }

// IsAnonymous reports a live anonymous identity.
func (a Actor) IsAnonymous() bool { return a.Kind == ActorAnonymous }

// IsTeacher reports an active teacher session.
func (a Actor) IsTeacher() bool { return a.Kind == ActorTeacher }

// IsVisitor reports no session and no live identity.
func (a Actor) IsVisitor() bool { return a.Kind == ActorVisitor }

// ParticipantID is the student id or anonymous participant id, whichever the
// actor is; zero for visitors and teachers.
func (a Actor) ParticipantID() uint32 {
	if a.IsStudent() {
		return a.StudentID
	}
	return a.IdentityID
}

// AttemptStart is ATT's attempt-start rule seam (PAR-202): PAR locks the
// registration, decides the attempt number, and hands the plan to ATT inside
// the same transaction; ATT owns the attempts-row insert and returns its id.
type AttemptStart func(ctx context.Context, tx *sql.Tx, plan StartPlan) (uint32, error)

// StartPlan is what CheckStart returns when a new attempt may be created. The
// adapter passes it to ATT inside the same transaction (PAR-082).
type StartPlan struct {
	RegistrationID         uint32
	QuizID                 uint32
	AttemptNo              int
	StudentID              uint32
	AnonymousParticipantID uint32
}

// StartInput is the typed handoff to ATT's attempt-start rule. It names the
// locked registration and the next attempt number; ATT owns the insert
// (PAR-202).
type StartInput struct {
	RegistrationID         uint32
	QuizID                 uint32
	AttemptNo              int
	StudentID              uint32
	AnonymousParticipantID uint32
}

// IdentityForm is the submitted identify/edit body. Values are raw strings so
// the same validation can return field errors for both screens (PAR-055,
// PAR-098).
type IdentityForm struct {
	FullName   string
	GradeLevel string
	MajorID    string
}

// readTx runs a read-only repeatable-read transaction so every aggregate on a
// page comes from one snapshot (PAR-126, PAR-145, PAR-165).
func (s *Service) readTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.db == nil {
		return internal(errors.New("participation: no database"))
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return internal(err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return internal(err)
	}
	return nil
}

// writeTx is the one write transaction helper. Every command is idempotent by
// natural key or by the ticket-derived identity, so a deadlock is retried once
// (PAR-9.4, OPS-215).
func (s *Service) writeTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.db == nil {
		return internal(errors.New("participation: no database"))
	}
	err := s.db.TxRetry(ctx, fn)
	if err == nil {
		return nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return err
	}
	var redirect *Redirect
	if errors.As(err, &redirect) {
		return err
	}
	return internal(err)
}

// internal wraps an unexpected failure without leaking driver text.
func internal(err error) *apperr.Error {
	return apperr.Internalf(err).WithRule("OPS-215")
}

// nowT reads the transaction timestamp. Every write stamp comes from the
// database clock, never time.Now (OPS-103, R-15).
func nowT(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	var t time.Time
	if err := tx.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP()").Scan(&t); err != nil {
		return time.Time{}, internal(err)
	}
	return t.UTC(), nil
}

// idPath builds a canonical /quiz/<id> target.
func quizPath(id uint32) string {
	return "/quiz/" + itoa(uint64(id))
}

// attemptPath builds /attempt/<id>.
func attemptPath(id uint32) string {
	return "/attempt/" + itoa(uint64(id))
}

// attemptResultPath builds /attempt/<id>/result.
func attemptResultPath(id uint32) string {
	return "/attempt/" + itoa(uint64(id)) + "/result"
}

// historyPath builds /student/history/<quizID>.
func historyPath(quizID uint32) string {
	return "/student/history/" + itoa(uint64(quizID))
}

// anonymousPath builds /teacher/anonymous/<id>.
func anonymousPath(id uint32) string {
	return "/teacher/anonymous/" + itoa(uint64(id))
}

// itoa is a tiny unsigned formatter that avoids importing strconv in every
// file.
func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// localPtr converts a nullable database time to the display model.
func localPtr(t sql.NullTime) *viewmodel.LocalTime {
	if !t.Valid {
		return nil
	}
	v := viewmodel.NewLocalTime(t.Time)
	return &v
}

// int64Ptr converts a nullable id to the display model's int64 pointer.
func int64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

// boolInt converts a MySQL TINYINT(1) scan to bool.
func boolInt(v int) bool { return v == 1 }

// uint32OrZero unwraps a nullable id for comparisons.
func uint32OrZero(v sql.NullInt64) uint32 {
	if !v.Valid || v.Int64 <= 0 {
		return 0
	}
	return uint32(v.Int64)
}
