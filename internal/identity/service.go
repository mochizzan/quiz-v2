package identity

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strings"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authn"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// Credential failures that are not field errors. They are sentinels so the
// route can choose the form-level alert (login) or the 403 page (pending
// teacher) without parsing text.
var (
	// ErrInvalidCredentials is the single unknown-email/wrong-password answer
	// (IDN-083, IDN-I13).
	ErrInvalidCredentials = errors.New("identity: invalid credentials")
	// ErrPendingTeacher is a correct password on a live pending teacher
	// (IDN-C03, IDN-013).
	ErrPendingTeacher = errors.New("identity: pending teacher")
)

// spendDummyCompare is the one real bcrypt compare the login's unknown-email
// branches spend so they cost the same as a wrong password (IDN-060). It is a
// package variable so unit tests can count the compares without a database;
// production binds secure.SpendDummyCompare.
var spendDummyCompare = secure.SpendDummyCompare

// Service binds the identity domain to one pool, the application signer, and
// the session store. It is built per request by the HTTP layer and holds no
// request state.
type Service struct {
	db       *store.DB
	signer   *secure.Signer
	sessions *authn.Store
	now      func() time.Time
}

// New returns the identity service. A nil pool is tolerated at construction so
// a demo build can build the route table; every database method answers 500
// instead of panicking.
func New(db *store.DB, signer *secure.Signer, sessions *authn.Store) *Service {
	return &Service{
		db:       db,
		signer:   signer,
		sessions: sessions,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// readTx runs a read-only repeatable-read transaction so a page's rows, total,
// and option lists come from one snapshot (IDN-094, IDN-142).
func (s *Service) readTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.db == nil {
		return internalErr(errors.New("identity: no database"))
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return internalErr(err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return classified(err)
	}
	if err := tx.Commit(); err != nil {
		return internalErr(err)
	}
	return nil
}

// writeTx runs one write transaction with the single permitted deadlock retry.
// Every command in this package is safe to retry: the lock is re-taken and a
// consumed token or committed delete is observed on the second attempt
// (IDN-077, IDN-143).
func (s *Service) writeTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.db == nil {
		return internalErr(errors.New("identity: no database"))
	}
	return classified(s.db.TxRetry(ctx, fn))
}

// internalErr wraps an unexpected failure without leaking driver text.
func internalErr(err error) *apperr.Error {
	return apperr.Internalf(err).WithRule("OPS-215")
}

// classified preserves the domain outcomes a route must map and wraps
// everything else as a 500 (OPS-215). It never rewrites a business error into
// an internal one.
func classified(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return err
	}
	var form *FormError
	if errors.As(err, &form) {
		return err
	}
	switch {
	case errors.Is(err, ErrInvalidCredentials),
		errors.Is(err, ErrPendingTeacher),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrTokenInvalid),
		errors.Is(err, ErrTokenUsed),
		errors.Is(err, ErrWrongPassword),
		errors.Is(err, ErrSamePassword),
		errors.Is(err, ErrSessionGone),
		errors.Is(err, ErrEmailTaken),
		errors.Is(err, secure.ErrEmailRequired),
		errors.Is(err, secure.ErrEmailInvalid),
		errors.Is(err, secure.ErrPasswordPolicy),
		errors.Is(err, secure.ErrPasswordTooLong):
		return err
	}
	return internalErr(err)
}

// LoginResult is what the route needs after a successful credential check: the
// raw session token (never stored), the role home inputs, and the must-change
// flag that selects /change-password instead.
type LoginResult struct {
	Token              string
	SessionID          int64
	Role               viewmodel.Role
	MustChangePassword bool
}

// Login verifies credentials and, on success, mints the session and stamps
// last_login_at in one transaction (IDN-C03, IDN-073).
//
// The email is normalised for lookup. A malformed address spends exactly one
// dummy bcrypt compare, exactly like an unknown address, so the two paths are
// indistinguishable (IDN-060).
func (s *Service) Login(ctx context.Context, rawEmail, password string, ip net.IP, userAgent string) (*LoginResult, error) {
	if s.db == nil {
		return nil, internalErr(errors.New("identity: no database"))
	}
	email, err := secure.NormalizeEmail(rawEmail)
	if err != nil {
		spendDummyCompare(password)
		return nil, ErrInvalidCredentials
	}
	creds, err := FindCredentialsByEmail(ctx, s.db.Querier(), email)
	if err != nil {
		return nil, internalErr(err)
	}
	if creds == nil {
		spendDummyCompare(password)
		return nil, ErrInvalidCredentials
	}
	if !secure.VerifyPassword(creds.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if creds.Role == viewmodel.RoleTeacher && creds.Status == viewmodel.AccountPending {
		return nil, ErrPendingTeacher
	}
	if creds.Status != viewmodel.AccountActive ||
		(creds.Role != viewmodel.RoleTeacher && creds.Role != viewmodel.RoleStudent) {
		return nil, ErrInvalidCredentials
	}
	if s.sessions == nil {
		return nil, internalErr(errors.New("identity: no session store"))
	}

	result := &LoginResult{
		Role:               creds.Role,
		MustChangePassword: creds.MustChangePassword,
	}
	err = s.writeTx(ctx, func(tx *sql.Tx) error {
		token, sessionID, err := s.sessions.Issue(ctx, tx, creds.ID, ip, userAgent)
		if err != nil {
			return err
		}
		if err := TouchLastLogin(ctx, tx, creds.ID); err != nil {
			return err
		}
		result.Token = token
		result.SessionID = sessionID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RegisterTeacher validates and inserts one pending teacher (IDN-C05). No
// session, profile, or last_login_at is written. The pending token is minted
// after commit because it is stateless.
func (s *Service) RegisterTeacher(ctx context.Context, fullName, email, password, confirm string) (string, error) {
	name, normalized, err := ValidateRegister(fullName, email, password, confirm)
	if err != nil {
		return "", err
	}
	hash, err := secure.HashPassword(password)
	if err != nil {
		return "", internalErr(err)
	}
	var accountID int64
	err = s.writeTx(ctx, func(tx *sql.Tx) error {
		id, err := InsertTeacher(ctx, tx, name, normalized, hash)
		if err != nil {
			return err
		}
		accountID = id
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return "", fieldError("email", apperr.MsgEmailTaken)
		}
		return "", err
	}
	if s.signer == nil {
		return "", internalErr(errors.New("identity: no signer"))
	}
	token, err := NewPendingToken(s.signer, accountID, s.now())
	if err != nil {
		return "", internalErr(err)
	}
	return token, nil
}

// StudentRegistration is the signed-in result of a successful registration.
type StudentRegistration struct {
	Token     string
	SessionID int64
}

// RegisterStudent validates, inserts the account and profile, and issues the
// session plus last_login_at in one transaction (IDN-C07, INV-02). Any failure
// leaves no account, no profile, and no session.
func (s *Service) RegisterStudent(ctx context.Context, fullName, email, password, confirm, gradeRaw, majorRaw string, ip net.IP, userAgent string) (*StudentRegistration, error) {
	name, normalized, school, err := ValidateStudentRegister(fullName, email, password, confirm, gradeRaw, majorRaw)
	if err != nil {
		return nil, err
	}
	hash, err := secure.HashPassword(password)
	if err != nil {
		return nil, internalErr(err)
	}
	if s.sessions == nil {
		return nil, internalErr(errors.New("identity: no session store"))
	}
	result := &StudentRegistration{}
	err = s.writeTx(ctx, func(tx *sql.Tx) error {
		accountID, _, err := CreateStudent(ctx, tx, name, normalized, hash, school.GradeLevel, school.MajorID)
		if err != nil {
			return err
		}
		token, sessionID, err := s.sessions.Issue(ctx, tx, accountID, ip, userAgent)
		if err != nil {
			return err
		}
		if err := TouchLastLogin(ctx, tx, accountID); err != nil {
			return err
		}
		result.Token = token
		result.SessionID = sessionID
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailTaken):
			return nil, fieldError("email", apperr.MsgEmailTaken)
		case errors.Is(err, ErrMajorRequired):
			return nil, fieldError("major_id", MsgMajorRequired)
		}
		return nil, err
	}
	return result, nil
}

// Logout soft-deletes the current session if it still exists (IDN-C13). A
// missing or already-revoked row is success: logout is idempotent.
func (s *Service) Logout(ctx context.Context, sessionID int64) error {
	if sessionID == 0 {
		return nil
	}
	if s.db == nil {
		return internalErr(errors.New("identity: no database"))
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET deleted_at = UTC_TIMESTAMP()
		WHERE id = ? AND deleted_at = ?`, sessionID, store.LiveAt)
	return classified(err)
}

// ChangePassword validates and writes a new password (IDN-C11). The current
// session row is locked first so a concurrent revocation makes the command
// answer session_expired rather than commit against a dead session (IDN-112).
// Other sessions are revoked in the same transaction; the current one stays.
func (s *Service) ChangePassword(ctx context.Context, accountID, sessionID int64, in ChangePasswordInput) error {
	if err := ValidateChangePassword(in); err != nil {
		return err
	}
	hash, err := secure.HashPassword(in.New)
	if err != nil {
		return internalErr(err)
	}
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM sessions
			WHERE id = ? AND account_id = ? AND deleted_at = ?
			FOR UPDATE`, sessionID, accountID, store.LiveAt).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSessionGone
		}
		if err != nil {
			return err
		}
		return ChangePassword(ctx, tx, accountID, sessionID, in.Current, in.New, hash)
	})
}

// ResetPassword validates the token form and consumes the token (IDN-C09). The
// token row is the serialization point: two submissions serialize, and the
// loser sees used_at set. Consumption writes the hash, marks the token used,
// clears must_change_password, and revokes every session of the student.
func (s *Service) ResetPassword(ctx context.Context, token, password, confirm string) error {
	trimmed, err := ValidateResetPassword(token, password, confirm)
	if err != nil {
		return err
	}
	hash, err := secure.HashPassword(password)
	if err != nil {
		return internalErr(err)
	}
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		var (
			tokenID   int64
			accountID int64
			usedAt    sql.NullTime
			expired   bool
		)
		err := tx.QueryRowContext(ctx, `
			SELECT t.id, s.account_id, t.used_at, (t.expires_at <= UTC_TIMESTAMP())
			FROM password_reset_tokens t
			JOIN students s ON s.id = t.student_id
			JOIN accounts a ON a.id = s.account_id
			WHERE t.token_hash = ?
			  AND t.deleted_at = ?
			  AND s.deleted_at = ?
			  AND a.deleted_at = ?
			  AND a.role = 'student'
			FOR UPDATE`,
			secure.HashToken(trimmed), store.LiveAt, store.LiveAt, store.LiveAt,
		).Scan(&tokenID, &accountID, &usedAt, &expired)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTokenInvalid
		}
		if err != nil {
			return err
		}
		if usedAt.Valid {
			return ErrTokenUsed
		}
		if expired {
			return ErrTokenInvalid
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE accounts
			SET password_hash = ?, must_change_password = 0
			WHERE id = ? AND deleted_at = ?`,
			hash, accountID, store.LiveAt); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE password_reset_tokens SET used_at = UTC_TIMESTAMP()
			WHERE id = ? AND used_at IS NULL AND deleted_at = ?`,
			tokenID, store.LiveAt)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrTokenUsed
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE sessions SET deleted_at = UTC_TIMESTAMP()
			WHERE account_id = ? AND deleted_at = ?`,
			accountID, store.LiveAt)
		return err
	})
}

// PendingInfo is the derived pending-page state (IDN-C12). FullName is set only
// for waiting and approved; expired never carries a name.
type PendingInfo struct {
	State    viewmodel.PendingState
	FullName string
}

// Pending resolves a pending-teacher token into exactly one state. Every token
// failure is the same expired state; a missing account is expired too, but a
// database failure is a 500 so an outage is not reported as expiry.
func (s *Service) Pending(ctx context.Context, token string) (*PendingInfo, error) {
	expired := &PendingInfo{State: viewmodel.PendingExpired}
	if strings.TrimSpace(token) == "" || s.signer == nil {
		return expired, nil
	}
	accountID, err := ParsePendingToken(s.signer, token, s.now())
	if err != nil {
		return expired, nil
	}
	var brief *AccountBrief
	err = s.readTx(ctx, func(tx *sql.Tx) error {
		var loadErr error
		brief, loadErr = FindAccountBrief(ctx, tx, accountID)
		return loadErr
	})
	if err != nil {
		return nil, err
	}
	if brief == nil || brief.Role != viewmodel.RoleTeacher {
		return expired, nil
	}
	switch brief.Status {
	case viewmodel.AccountActive:
		return &PendingInfo{State: viewmodel.PendingApproved, FullName: brief.FullName}, nil
	case viewmodel.AccountPending:
		return &PendingInfo{State: viewmodel.PendingWaiting, FullName: brief.FullName}, nil
	default:
		return expired, nil
	}
}

// Majors lists the live majors for a student form (IDN-C06, IDN-095).
func (s *Service) Majors(ctx context.Context) ([]viewmodel.MajorRef, error) {
	out := []viewmodel.MajorRef{}
	err := s.readTx(ctx, func(tx *sql.Tx) error {
		var loadErr error
		out, loadErr = LiveMajors(ctx, tx)
		return loadErr
	})
	return out, err
}
