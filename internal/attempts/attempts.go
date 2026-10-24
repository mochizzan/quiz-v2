// Package attempts implements the ATT domain (business logic 04): attempt
// start/resume, answer persistence, deterministic shuffle, submit, objective
// scoring, essay grading, the final score, review visibility, result and recap
// read models, teacher aggregates, the live monitor, the SSE stream, and
// attempt beacons (spec ATT-001…ATT-311).
//
// Authority boundaries (ATT-011…ATT-018): the package owns every write to
// quiz_attempts, attempt_answers, attempt_answer_selections, and
// focus_loss_events. It never writes registrations, quizzes, questions, or
// options, never creates or cancels a registration, and never stores a 0–100
// value: every score is stored on the 0–1 scale in DECIMAL(5,4) (ATT-002).
// All database times are UTC; a viewer-facing page formats in WIB.
//
// The package takes *store.DB or an *sql.Tx, never a handler context: the HTTP
// layer resolves the actor material (session, identity cookie) and passes it
// in, the domain re-checks ownership inside the transaction that needs it
// (ATT-024), so a profile or identity change cannot race the check.
package attempts

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
)

// Service binds the attempts domain to one database pool. It is constructed
// per request by the HTTP layer; it holds no request state. A nil pool is
// tolerated at construction so a demo build can build the route table; every
// method that needs the database answers 500 instead of panicking.
type Service struct {
	db      *store.DB
	logger  *slog.Logger
	limiter *secure.Limiter
}

// New returns the attempts service.
func New(db *store.DB) *Service {
	return &Service{db: db, logger: slog.Default()}
}

// WithLogger injects the process logger so domain-side operational events
// (missing data-integrity rows, duplicate attempt numbers) land in the same
// structured stream as the HTTP layer. A nil logger keeps the default.
func (s *Service) WithLogger(l *slog.Logger) *Service {
	if l != nil {
		s.logger = l
	}
	return s
}

// warn logs one warn-level domain line when a logger is attached.
func (s *Service) warn(rule, msg string, err error) {
	if s.logger != nil {
		s.logger.Warn(msg, "rule", rule, "error", err)
	}
}

// readTx runs a read-only repeatable-read transaction so every aggregate on a
// page comes from one snapshot (ATT-100, ATT-161).
func (s *Service) readTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.db == nil {
		return internal(errors.New("attempts: no database"))
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

// writeTx is the retrying write helper for the flows OPS-102 declares
// idempotent: attempt start (resume), answer save (content compare), submit
// and beacon progress (state guards). A deadlock or lock wait timeout is
// retried once through store.TxRetry; a retry of a rolled-back command sees the
// same guards, which is what makes the retry safe. Typed domain errors pass
// through unchanged; TxRetry only inspects the MySQL error class.
func (s *Service) writeTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.db == nil {
		return internal(errors.New("attempts: no database"))
	}
	return s.db.TxRetry(ctx, fn)
}

// writeTxOnce is the non-retrying write helper OPS-102 requires for a grade: a
// deadlock or lock wait timeout aborts the transaction and becomes a 500,
// because a grade is not on the retry list even though its CAS predicate would
// make a replay safe.
func (s *Service) writeTxOnce(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.db == nil {
		return internal(errors.New("attempts: no database"))
	}
	if err := s.db.Tx(ctx, fn); err != nil {
		var appErr *apperr.Error
		if errors.As(err, &appErr) {
			return err
		}
		return internal(err)
	}
	return nil
}

// internal wraps an unexpected failure without leaking driver text. It is the
// 500 page's message in production (OPS-215).
func internal(err error) *apperr.Error {
	return apperr.Internalf(err).WithRule("OPS-215")
}

// nowT reads the single transaction timestamp. Every command and every sweep
// uses this instead of time.Now so lifecycle stamps, cascade stamps, and the
// focus-log timeline all come from one database clock (OPS-103, ATT-003).
func nowT(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	var t time.Time
	if err := tx.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP()").Scan(&t); err != nil {
		return time.Time{}, internal(err)
	}
	return t, nil
}
