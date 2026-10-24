package authoring

import (
	"errors"

	"quiz-v2/internal/viewmodel"
)

// errUnknownState is an integrity fault: the ENUM only allows three values, so
// anything else means the row was written outside the application.
var errUnknownState = errors.New("authoring: unknown quiz state")

// decision is the pure guard result for one lifecycle command.
type decision struct {
	Allowed  bool
	Conflict string
}

// contentDecision is the draft guard shared by every authoring write and the
// two content GETs (AUT-022).
func contentDecision(state string) (decision, error) {
	switch viewmodel.QuizState(state) {
	case viewmodel.QuizDraft:
		return decision{Allowed: true}, nil
	case viewmodel.QuizOpen, viewmodel.QuizClosed:
		return decision{Conflict: FlashErrNotDraft}, nil
	default:
		return decision{}, errUnknownState
	}
}

// publishDecision is draft -> open; open is an informational repeat and closed
// is terminal (AUT-006, AUT-030).
func publishDecision(state string) (decision, error) {
	switch viewmodel.QuizState(state) {
	case viewmodel.QuizDraft:
		return decision{Allowed: true}, nil
	case viewmodel.QuizOpen:
		return decision{Conflict: FlashAlreadyOpen}, nil
	case viewmodel.QuizClosed:
		return decision{Conflict: FlashErrClosedTerminal}, nil
	default:
		return decision{}, errUnknownState
	}
}

// closeDecision is open -> closed; a draft was never opened and closed is
// already terminal (AUT-007).
func closeDecision(state string) (decision, error) {
	switch viewmodel.QuizState(state) {
	case viewmodel.QuizOpen:
		return decision{Allowed: true}, nil
	case viewmodel.QuizClosed:
		return decision{Conflict: FlashClosed}, nil
	case viewmodel.QuizDraft:
		return decision{Conflict: FlashErrNotOpen}, nil
	default:
		return decision{}, errUnknownState
	}
}

// deleteDecision is draft-only (AUT-008).
func deleteDecision(state string) (decision, error) {
	switch viewmodel.QuizState(state) {
	case viewmodel.QuizDraft:
		return decision{Allowed: true}, nil
	case viewmodel.QuizOpen, viewmodel.QuizClosed:
		return decision{Conflict: FlashErrDeleteNotDraft}, nil
	default:
		return decision{}, errUnknownState
	}
}

// guard converts a decision into the command's error contract: nil to proceed,
// a conflict sentinel for the 303, or an internal fault.
func guard(d decision, err error) error {
	if err != nil {
		return internal(err)
	}
	if !d.Allowed {
		return conflict(d.Conflict)
	}
	return nil
}
