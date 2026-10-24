package identity

import (
	"context"
	"database/sql"
	"errors"

	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// Credentials is the login projection: exactly what credential verification
// needs, plus the role/status the route branches on. The hash never leaves this
// package's callers.
type Credentials struct {
	ID                 int64
	FullName           string
	Email              string
	PasswordHash       string
	Role               viewmodel.Role
	Status             viewmodel.AccountStatus
	MustChangePassword bool
}

// FindCredentialsByEmail selects the one live account for a normalised email.
// A soft-deleted row is invisible, which is what makes its email reusable
// (IDN-054, INV-19). nil means "no such account", not an error.
func FindCredentialsByEmail(ctx context.Context, q store.Querier, email string) (*Credentials, error) {
	var (
		c      Credentials
		role   string
		status string
		must   int
	)
	err := q.QueryRowContext(ctx, `
		SELECT id, full_name, email, password_hash, role, status, must_change_password
		FROM accounts
		WHERE email = ? AND deleted_at = ?`,
		email, store.LiveAt,
	).Scan(&c.ID, &c.FullName, &c.Email, &c.PasswordHash, &role, &status, &must)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Role = viewmodel.Role(role)
	c.Status = viewmodel.AccountStatus(status)
	c.MustChangePassword = must == 1
	return &c, nil
}

// AccountBrief is the pending page's read model: the minimum needed to choose
// waiting/approved/expired without exposing anything else (IDN-C12).
type AccountBrief struct {
	ID       int64
	FullName string
	Role     viewmodel.Role
	Status   viewmodel.AccountStatus
}

// FindAccountBrief loads one live account by id. A missing or soft-deleted
// account is nil, not an error.
func FindAccountBrief(ctx context.Context, q store.Querier, accountID int64) (*AccountBrief, error) {
	var (
		a      AccountBrief
		role   string
		status string
	)
	err := q.QueryRowContext(ctx, `
		SELECT id, full_name, role, status
		FROM accounts
		WHERE id = ? AND deleted_at = ?`,
		accountID, store.LiveAt,
	).Scan(&a.ID, &a.FullName, &role, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Role = viewmodel.Role(role)
	a.Status = viewmodel.AccountStatus(status)
	return &a, nil
}

// InsertTeacher creates the pending teacher account (IDN-C05). It never writes
// a profile, a session, or last_login_at. A duplicate live email is
// ErrEmailTaken so the route can answer `422 email_taken`.
func InsertTeacher(ctx context.Context, q store.Querier, fullName, email, passwordHash string) (int64, error) {
	res, err := q.ExecContext(ctx, `
		INSERT INTO accounts
			(full_name, email, password_hash, role, status,
			 approved_by_account_id, approved_at, must_change_password)
		VALUES (?, ?, ?, 'teacher', 'pending', NULL, NULL, 0)`,
		fullName, email, passwordHash)
	if store.IsDuplicate(err) {
		return 0, ErrEmailTaken
	}
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LockMajor verifies that a major is live and locks its row for the caller's
// transaction. It is the serialization point for a registration that must not
// reference a major being soft-deleted concurrently (IDN-C07, IDN-082).
func LockMajor(ctx context.Context, q store.Querier, majorID int64) (bool, error) {
	var id int64
	err := q.QueryRowContext(ctx, `
		SELECT id FROM majors
		WHERE id = ? AND deleted_at = ?
		FOR UPDATE`, majorID, store.LiveAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CreateStudent inserts the active student account and its profile in one
// transaction (IDN-C07, INV-02). The caller supplies the transaction; any error
// rolls both rows back. A duplicate email is ErrEmailTaken; a missing major is
// ErrMajorRequired.
func CreateStudent(ctx context.Context, q store.Querier, fullName, email, passwordHash string, gradeLevel int, majorID int64) (accountID, studentID int64, err error) {
	live, err := LockMajor(ctx, q, majorID)
	if err != nil {
		return 0, 0, err
	}
	if !live {
		return 0, 0, ErrMajorRequired
	}

	res, err := q.ExecContext(ctx, `
		INSERT INTO accounts
			(full_name, email, password_hash, role, status,
			 approved_by_account_id, approved_at, must_change_password)
		VALUES (?, ?, ?, 'student', 'active', NULL, NULL, 0)`,
		fullName, email, passwordHash)
	if store.IsDuplicate(err) {
		return 0, 0, ErrEmailTaken
	}
	if err != nil {
		return 0, 0, err
	}
	accountID, err = res.LastInsertId()
	if err != nil {
		return 0, 0, err
	}

	res, err = q.ExecContext(ctx, `
		INSERT INTO students (account_id, grade_level, major_id)
		VALUES (?, ?, ?)`,
		accountID, gradeLevel, majorID)
	if err != nil {
		return accountID, 0, err
	}
	studentID, err = res.LastInsertId()
	if err != nil {
		return accountID, 0, err
	}
	return accountID, studentID, nil
}

// UpdateProfile changes only the student's own name and email (IDN-C15). It
// locks the account row first so a concurrent delete cannot commit a write
// attributed to a dead session (IDN-112, IDN-113). A missing row is
// ErrSessionGone; a duplicate email is ErrEmailTaken.
func UpdateProfile(ctx context.Context, q store.Querier, accountID int64, fullName, email string) error {
	var id int64
	err := q.QueryRowContext(ctx, `
		SELECT id FROM accounts
		WHERE id = ? AND deleted_at = ?
		FOR UPDATE`, accountID, store.LiveAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSessionGone
	}
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		UPDATE accounts
		SET full_name = ?, email = ?
		WHERE id = ? AND deleted_at = ?`,
		fullName, email, accountID, store.LiveAt)
	if store.IsDuplicate(err) {
		return ErrEmailTaken
	}
	return err
}

// TouchLastLogin stamps the successful active login (IDN-073). It is part of
// the login transaction, so a session and its login time commit together.
func TouchLastLogin(ctx context.Context, q store.Querier, accountID int64) error {
	_, err := q.ExecContext(ctx, `
		UPDATE accounts
		SET last_login_at = UTC_TIMESTAMP()
		WHERE id = ? AND deleted_at = ?`,
		accountID, store.LiveAt)
	return err
}

// ChangePassword verifies the current password under the account row lock,
// rejects a new password equal to the current one, writes the new hash, clears
// must_change_password, and revokes every other session (IDN-C11, IDN-045).
//
// newPlain is needed only for the equality check; newHash is written. The
// caller hashes outside the lock so bcrypt does not extend the critical
// section more than the two required comparisons.
func ChangePassword(ctx context.Context, q store.Querier, accountID, currentSessionID int64, currentPlain, newPlain, newHash string) error {
	var stored string
	err := q.QueryRowContext(ctx, `
		SELECT password_hash FROM accounts
		WHERE id = ? AND deleted_at = ?
		FOR UPDATE`, accountID, store.LiveAt).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSessionGone
	}
	if err != nil {
		return err
	}
	if !secure.VerifyPassword(stored, currentPlain) {
		return ErrWrongPassword
	}
	if secure.VerifyPassword(stored, newPlain) {
		return ErrSamePassword
	}

	if _, err := q.ExecContext(ctx, `
		UPDATE accounts
		SET password_hash = ?, must_change_password = 0
		WHERE id = ? AND deleted_at = ?`,
		newHash, accountID, store.LiveAt); err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		UPDATE sessions
		SET deleted_at = UTC_TIMESTAMP()
		WHERE account_id = ? AND id <> ? AND deleted_at = ?`,
		accountID, currentSessionID, store.LiveAt)
	return err
}

// Profile is the student's own account read model (IDN-098). Grade and major
// come from the live profile, never from the session's cached display values.
type Profile struct {
	FullName   string
	Email      string
	GradeLevel int
	MajorID    int64
}

// LoadProfile reads the session account's own live account, profile, and major
// id. A missing account or profile is a data fault (INV-02); the caller maps
// nil to a 500 rather than inventing a grade.
func LoadProfile(ctx context.Context, q store.Querier, accountID int64) (*Profile, error) {
	var p Profile
	err := q.QueryRowContext(ctx, `
		SELECT a.full_name, a.email, s.grade_level, s.major_id
		FROM accounts a
		JOIN students s ON s.account_id = a.id AND s.deleted_at = ?
		WHERE a.id = ? AND a.deleted_at = ? AND a.role = 'student'`,
		store.LiveAt, accountID, store.LiveAt,
	).Scan(&p.FullName, &p.Email, &p.GradeLevel, &p.MajorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// LiveMajors lists the selectable majors in the pinned order: name ASC, id ASC
// (IDN-C06, IDN-095). A soft-deleted major is never selectable.
func LiveMajors(ctx context.Context, q store.Querier) ([]viewmodel.MajorRef, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, name FROM majors
		WHERE deleted_at = ?
		ORDER BY name ASC, id ASC`, store.LiveAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []viewmodel.MajorRef{}
	for rows.Next() {
		var m viewmodel.MajorRef
		if err := rows.Scan(&m.ID, &m.Name); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
