package attempts

import (
	"errors"
	"fmt"
)

// The sentinel errors one outcome maps to (ATT-020, ATT-107, ATT-160). They
// are compared with errors.Is; the HTTP layer maps each to its one status and
// never to a second candidate.
var (
	// ErrNotFound is a missing, soft-deleted, or foreign resource. Every
	// ATT route answers the 404 page for it, never 403 (ATT-020, INV-26).
	ErrNotFound = errors.New("attempts: not found")
	// ErrNotOpen means a new attempt may not start because the quiz is not
	// open (R-10). A closed quiz with an in-progress attempt resumes instead.
	ErrNotOpen = errors.New("attempts: quiz not open")
	// ErrAttemptLimit means the registration has used its quota
	// (EffectiveMaxAttempts, ATT-102/ATT-106).
	ErrAttemptLimit = errors.New("attempts: attempt limit reached")
	// ErrNoQuestions means a start found no live question (ATT-106).
	ErrNoQuestions = errors.New("attempts: quiz has no questions")
	// ErrAnswerLocked means the attempt is submitted, so no answer write is
	// possible (ATT-126, INV-08). It redirects to the result.
	ErrAnswerLocked = errors.New("attempts: attempt submitted")
	// ErrNotSubmitted means a result was requested for an unsubmitted attempt
	// (ATT-151).
	ErrNotSubmitted = errors.New("attempts: attempt not submitted")
	// ErrGradeConflict means the essay changed between rendering the form and
	// submitting it; the edit is rejected instead of silently overwriting
	// (grading-history spec §4).
	ErrGradeConflict = errors.New("attempts: grade changed")
)

// The closed flash keys this domain introduces (AT-37). Every key is
// registered in viewmodel.FlashFor so a redirect carries a signed one-shot
// cookie instead of echoing the key into the URL.
const (
	FlashNotOpen       = "not_open"
	FlashAttemptLimit  = "attempt_limit"
	FlashNoQuestions   = "no_questions"
	FlashAnswerLocked  = "answer_locked"
	FlashNotSubmitted  = "not_submitted"
	FlashGradeConflict = "grade_conflict"
)

// StartFlashKey maps a start error to the flash key PAR must redirect with
// (ATT-107). ErrNotFound has no key: PAR renders the 404 page.
func StartFlashKey(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrNotOpen):
		return FlashNotOpen, true
	case errors.Is(err, ErrAttemptLimit):
		return FlashAttemptLimit, true
	case errors.Is(err, ErrNoQuestions):
		return FlashNoQuestions, true
	default:
		return "", false
	}
}

// Conflict is one legal-but-refused state transition (ATT-020): it carries the
// closed flash key the route must redirect with, and a cause anchored at one
// sentinel for errors.Is. A conflict is never a 500 and never 422.
type Conflict struct {
	Flash string
	Err   error
}

// Error implements error.
func (e *Conflict) Error() string {
	if e.Err != nil {
		return "attempts: conflict " + e.Flash + ": " + e.Err.Error()
	}
	return "attempts: conflict " + e.Flash
}

// Unwrap anchors the conflict at its sentinel so errors.Is keeps working
// through the transaction helper.
func (e *Conflict) Unwrap() error { return e.Err }

// ConflictKey reports the flash key of a state conflict (ATT-020). The key is
// what the route adapter carries to the 303; the sentinel is what tests and
// callers branch on.
func ConflictKey(err error) (string, bool) {
	var c *Conflict
	if errors.As(err, &c) {
		return c.Flash, true
	}
	return "", false
}

// RedirectSignal is a GET outcome that must answer 303 instead of rendering:
// a submitted attempt's question and review reads, or an unsubmitted result
// request (ATT-111, ATT-131, ATT-151). It carries the relative target and an
// optional closed flash key. The route adapter maps it to a 303.
type RedirectSignal struct {
	Path  string
	Flash string
}

// Error implements error.
func (e *RedirectSignal) Error() string { return "attempts: redirect to " + e.Path }

// Redirect builds a 303 signal without a flash.
func Redirect(path string) error { return &RedirectSignal{Path: path} }

// RedirectFlash builds a 303 signal that renders one flash on arrival.
func RedirectFlash(path, flash string) error {
	return &RedirectSignal{Path: path, Flash: flash}
}

// AsRedirect reports a redirect signal and its fields.
func AsRedirect(err error) (RedirectSignal, bool) {
	var s *RedirectSignal
	if errors.As(err, &s) {
		return RedirectSignal{Path: s.Path, Flash: s.Flash}, true
	}
	return RedirectSignal{}, false
}

// errIntegrity marks a 500 data-integrity violation (ATT-033, ATT-281): a
// missing slot, an attempt under a dead registration, a live attempt whose
// registration is gone. The cause names the exact invariant for the log; the
// viewer sees the standard 500 page.
type errIntegrity struct{ msg string }

// Error implements error.
func (e *errIntegrity) Error() string { return "attempts: data integrity: " + e.msg }

// integrity builds one integrity signal with a formatted cause.
func integrity(format string, args ...any) error {
	return &errIntegrity{msg: fmt.Sprintf(format, args...)}
}
