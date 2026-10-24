// Package authn owns the database-backed session: issuing it at login, resolving
// it on every request, sliding its expiry, and revoking it. It answers exactly
// one question — "who is this request?" — and nothing about what that identity
// may do. Authorization lives in the HTTP layer's route classes; domain rules
// live in their own packages.
//
// Rules: IDN-003 (what authenticated means), IDN-041…IDN-047 (session
// lifecycle), IDN-077 (transaction style), and schema spec §6.13.
package authn

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// SessionTTL is the sliding window. It is refreshed on every authenticated
// request; there is no absolute lifetime beyond it (IDN-042, IDN-046).
const SessionTTL = 60 * time.Minute

// ErrProfileMissing is a data fault: a student account with no live profile.
// It must surface as a 500, never as a silent zero grade level, because every
// eligibility rule would then answer wrongly.
var ErrProfileMissing = errors.New("authn: student account has no live profile")

// Session is the resolved identity of one request.
type Session struct {
	ID                 int64
	AccountID          int64
	Role               viewmodel.Role
	FullName           string
	Email              string
	MustChangePassword bool
	GradeLevel         int
	MajorName          string
	ExpiresAt          time.Time
}

// CurrentUser projects the session into the chrome's user model.
func (s *Session) CurrentUser() *viewmodel.CurrentUser {
	if s == nil {
		return nil
	}
	return &viewmodel.CurrentUser{
		ID:         s.AccountID,
		FullName:   s.FullName,
		Email:      s.Email,
		Role:       s.Role,
		GradeLevel: s.GradeLevel,
		MajorName:  s.MajorName,
		Initials:   viewmodel.Initials(s.FullName),
	}
}

// IsTeacher reports the teacher role.
func (s *Session) IsTeacher() bool { return s != nil && s.Role == viewmodel.RoleTeacher }

// IsStudent reports the student role.
func (s *Session) IsStudent() bool { return s != nil && s.Role == viewmodel.RoleStudent }

// Store issues and resolves sessions against the database.
type Store struct {
	db *store.DB
}

// NewStore builds a session store over the application pool.
func NewStore(db *store.DB) *Store { return &Store{db: db} }

// Issue mints a session for accountID and returns the raw token. Only its
// SHA-256 is written, so the token exists solely in the response cookie.
// A nil q uses the pool; passing a transaction makes the session part of the
// same unit as the registration that created the account.
func (s *Store) Issue(ctx context.Context, q store.Querier, accountID int64, ip net.IP, userAgent string) (string, int64, error) {
	token, err := secure.NewToken()
	if err != nil {
		return "", 0, err
	}
	if q == nil {
		q = s.db.Querier()
	}
	res, err := q.ExecContext(ctx, `
		INSERT INTO sessions (account_id, token_hash, expires_at, ip, user_agent)
		VALUES (?, ?, UTC_TIMESTAMP() + INTERVAL 60 MINUTE, ?, ?)`,
		accountID, secure.HashToken(token), ipBytes(ip), TruncateUTF8(userAgent, 255))
	if err != nil {
		return "", 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return "", 0, err
	}
	return token, id, nil
}

// Resolve looks up a raw token and returns the live session behind it, or nil
// when the token is unknown, expired, revoked, or belongs to an account that is
// no longer live and active. It never writes (IDN-022).
func (s *Store) Resolve(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, nil
	}
	var (
		sess       Session
		role       string
		mustChange int
		grade      sql.NullInt64
		major      sql.NullString
		studentID  sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.account_id, s.expires_at, a.role, a.full_name, a.email,
		       a.must_change_password, st.id, st.grade_level, m.name
		FROM sessions s
		JOIN accounts a ON a.id = s.account_id
		LEFT JOIN students st ON st.account_id = a.id AND st.deleted_at = ?
		LEFT JOIN majors m ON m.id = st.major_id AND m.deleted_at = ?
		WHERE s.token_hash = ?
		  AND s.deleted_at = ?
		  AND s.expires_at > UTC_TIMESTAMP()
		  AND a.deleted_at = ?
		  AND a.status = 'active'`,
		store.LiveAt, store.LiveAt, secure.HashToken(token), store.LiveAt, store.LiveAt,
	).Scan(&sess.ID, &sess.AccountID, &sess.ExpiresAt, &role, &sess.FullName, &sess.Email,
		&mustChange, &studentID, &grade, &major)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess.Role = viewmodel.Role(role)
	sess.MustChangePassword = mustChange == 1
	if sess.Role == viewmodel.RoleStudent {
		if !studentID.Valid {
			return nil, ErrProfileMissing
		}
		sess.GradeLevel = int(grade.Int64)
		sess.MajorName = major.String
	}
	return &sess, nil
}

// Slide rewrites the expiry to now+60m. Concurrent requests may each slide; the
// last writer wins and the result is still now+60m (IDN-042).
func (s *Store) Slide(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET expires_at = UTC_TIMESTAMP() + INTERVAL 60 MINUTE
		WHERE id = ? AND deleted_at = ?`, id, store.LiveAt)
	return err
}

// Revoke soft-deletes one session. Revocation is a system event, so
// deleted_by_account_id stays NULL (schema §5.2).
func (s *Store) Revoke(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET deleted_at = UTC_TIMESTAMP()
		WHERE id = ? AND deleted_at = ?`, id, store.LiveAt)
	return err
}

// RevokeAll kills every live session of an account. Password reset uses it.
func (s *Store) RevokeAll(ctx context.Context, accountID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET deleted_at = UTC_TIMESTAMP()
		WHERE account_id = ? AND deleted_at = ?`, accountID, store.LiveAt)
	return err
}

// RevokeOthers kills every live session except keepID. Change-password uses it
// so the device that changed the password stays signed in (IDN-045).
func (s *Store) RevokeOthers(ctx context.Context, accountID, keepID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET deleted_at = UTC_TIMESTAMP()
		WHERE account_id = ? AND id <> ? AND deleted_at = ?`, accountID, keepID, store.LiveAt)
	return err
}

// ipBytes stores a peer address as 4 or 16 raw bytes, or NULL when unknown.
func ipBytes(ip net.IP) []byte {
	if ip == nil {
		return nil
	}
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip.To16()
}

// TruncateUTF8 cuts s to at most n bytes without splitting a rune, so a
// multibyte user agent never becomes invalid UTF-8 in the column.
func TruncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	b := []byte(s[:n])
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b)
}

// ClientIP returns the transport peer address. Proxy headers are never trusted
// (IDN-027), so this is the only source of a client IP.
func ClientIP(remote string) net.IP {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remote))
	if err != nil {
		host = remote
	}
	return net.ParseIP(host)
}
