package authoring

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
)

// Service binds the authoring domain to one database pool and upload root. It
// is constructed per request by the HTTP layer; it holds no request state.
type Service struct {
	db      *store.DB
	uploads string
	logger  *slog.Logger
}

// New returns the authoring service. A nil pool is tolerated at construction
// for the handler tests; every method that needs the database answers 500
// instead of panicking.
func New(db *store.DB, uploadsDir string) *Service {
	return &Service{db: db, uploads: uploadsDir, logger: slog.Default()}
}

// WithLogger injects the process logger so domain-side operational events
// (upload_orphan, OPS-061) land in the same structured stream as the HTTP layer.
// A nil logger keeps the default.
func (s *Service) WithLogger(l *slog.Logger) *Service {
	if l != nil {
		s.logger = l
	}
	return s
}

// discardImage unlinks one generated image name after the commit (or after a
// failed transaction for a staged file). A failed unlink is logged as
// upload_orphan and tolerated: an orphan is never served (AUT-030/OPS-061).
func (s *Service) discardImage(name string) {
	if name == "" {
		return
	}
	if err := removeImage(s.uploads, name); err != nil {
		s.logger.Warn("upload_orphan", "rule", "OPS-061", "name", name, "error", err)
	}
}

// conflictError carries the closed flash key for a legal-but-refused state
// transition. Commands translate it to a 303 with that key; it is never a 500.
type conflictError struct{ key string }

func (e conflictError) Error() string { return "authoring: conflict " + e.key }

func conflict(key string) error { return conflictError{key: key} }

// ConflictKey reports the flash key of a state conflict.
func ConflictKey(err error) (string, bool) {
	var c conflictError
	if errors.As(err, &c) {
		return c.key, true
	}
	return "", false
}

// readTx runs a read-only repeatable-read transaction so the rows behind one
// page come from one snapshot (AUT-001, AUT-004).
func (s *Service) readTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.db == nil {
		return apperr.Internalf(errors.New("authoring: no database")).WithRule("OPS-215")
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

// writeTx is the one write transaction helper. AUT never retries a command
// automatically (CX-AUT-12), so it never uses TxRetry.
func (s *Service) writeTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.db == nil {
		return apperr.Internalf(errors.New("authoring: no database")).WithRule("OPS-215")
	}
	err := s.db.Tx(ctx, fn)
	if err == nil {
		return nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return err
	}
	if key, ok := ConflictKey(err); ok {
		return conflict(key)
	}
	return internal(err)
}

// internal wraps an unexpected failure without leaking driver text.
func internal(err error) *apperr.Error {
	return apperr.Internalf(err).WithRule("OPS-215")
}

// nowT reads the single transaction timestamp. Every command uses this instead
// of time.Now so lifecycle and soft-delete stamps come from the database clock
// (OPS-103).
func nowT(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	var t time.Time
	if err := tx.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP()").Scan(&t); err != nil {
		return time.Time{}, internal(err)
	}
	return t.UTC(), nil
}

// maxDeleted returns MAX(deleted_at) for a collision-family query. A NULL
// aggregate is simply an absent candidate.
func maxDeleted(ctx context.Context, q store.Querier, query string, args ...any) (sql.NullTime, error) {
	var max sql.NullTime
	err := q.QueryRowContext(ctx, query, args...).Scan(&max)
	return max, err
}

// quizPath is the canonical builder URL.
func quizPath(id int64) string { return "/teacher/quizzes/" + strconv.FormatInt(id, 10) }

// questionPath is the canonical editor URL.
func questionPath(quizID, questionID int64) string {
	return quizPath(quizID) + "/questions/" + strconv.FormatInt(questionID, 10)
}

// listPath is the quiz collection URL.
const listPath = "/teacher/quizzes"
