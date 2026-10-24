// Package identity owns the account, credential and access surface: who a
// request is (resolved by internal/authn), who may register, sign in, reset or
// change a password, how a student profile and the school-wide roster are
// managed, and how a pending teacher is approved.
//
// It implements the commands of
// docs/superpowers/specs/2026-10-23-business-logic-01-identity-and-access.md:
// IDN-C01…IDN-C22 plus the credential rules IDN-053…IDN-108 and the concurrency
// rules IDN-140…IDN-147. Every repository function takes a store.Querier rather
// than the pool, so a route adapter can run one multi-write command inside a
// single transaction (IDN-077, IDN-141) and pass the *sql.Tx down unchanged.
//
// Everything here is storage-shaped: no function renders, redirects, sets a
// cookie, or knows a route. The route files in internal/httpserver translate a
// returned error into apperr copy and a status.
package identity

import (
	"errors"
	"time"
)

// Domain copy owned by this spec (§5 IDN-080). The R-19 global keys live in
// internal/apperr and are cited, never re-spelled: MsgInvalidCredentials,
// MsgEmailTaken, MsgPasswordPolicy, MsgPasswordTooLong, MsgPasswordMismatch,
// MsgCurrentPassword, MsgSamePassword, MsgInvalidToken, MsgTokenUsed,
// MsgSessionExpired, MsgPendingTeacher, MsgEmailRequired, MsgEmailInvalid.
const (
	// MsgNameRequired is `name_required`.
	MsgNameRequired = "Nama lengkap wajib diisi."
	// MsgNameLong is `name_long`.
	MsgNameLong = "Nama lengkap maksimal 100 karakter."
	// MsgPasswordRequired is `password_required`.
	MsgPasswordRequired = "Kata sandi wajib diisi."
	// MsgConfirmRequired is `confirm_required`.
	MsgConfirmRequired = "Konfirmasi kata sandi wajib diisi."
	// MsgCurrentRequired is `current_required`.
	MsgCurrentRequired = "Kata sandi saat ini wajib diisi."
	// MsgGradeRequired is `grade_required`.
	MsgGradeRequired = "Pilih kelas."
	// MsgMajorRequired is `major_required`.
	MsgMajorRequired = "Pilih jurusan yang tersedia."
	// MsgTokenRequired is `token_required`.
	MsgTokenRequired = "Token wajib diisi."
	// MsgNoMajors is `no_majors`, the note shown when the school has no major
	// yet (IDN-C06, IDN-122).
	MsgNoMajors = "Belum ada jurusan. Hubungi guru untuk menambahkan jurusan."
)

// Fixed limits. PageSize is R-21's student roster size; the token windows are
// R-15/R-23/IDN-105.
const (
	// MinPasswordLength is the form's `minlength` and the policy floor.
	MinPasswordLength = 8
	// MaxTokenLength bounds the reset token a form may submit (IDN-128).
	MaxTokenLength = 128
	// PageSize is the student roster page size (R-21, IDN-094).
	PageSize = 10
	// PendingTokenTTL is the pending-teacher token window (R-23).
	PendingTokenTTL = 24 * time.Hour
	// RevealTokenTTL is the one-shot reset-token display window (IDN-105).
	RevealTokenTTL = 5 * time.Minute
	// RevealCookiePath scopes the reveal cookie to the students list.
	RevealCookiePath = "/teacher/students"
	// PendingCookiePath is the pending token cookie path.
	PendingCookiePath = "/"
)

// Purpose strings. They are domain separators inside the shared signer, so a
// pending token can never be replayed as a reveal cookie or vice versa (R-23,
// IDN-105).
const (
	pendingTokenPurpose = "pending"
	revealTokenPurpose  = "resetreveal"
)

// Repository outcomes the route adapter maps to apperr copy. They are plain
// sentinels, never user-visible text.
var (
	// ErrNotFound is a missing, soft-deleted, or wrong-role target. A route
	// answers 404 for it (IDN-012).
	ErrNotFound = errors.New("identity: not found")
	// ErrTokenInvalid is an unknown, revoked, or expired reset token
	// (`invalid_token`, IDN-C09).
	ErrTokenInvalid = errors.New("identity: reset token invalid")
	// ErrTokenUsed is a reset token already consumed (`token_used`, IDN-C09).
	ErrTokenUsed = errors.New("identity: reset token used")
	// ErrWrongPassword is a failed current-password check (IDN-C11).
	ErrWrongPassword = errors.New("identity: current password does not match")
	// ErrSamePassword is a new password equal to the stored one (IDN-C11).
	ErrSamePassword = errors.New("identity: new password equals current")
	// ErrSessionGone is the IDN-112 case: the session was revoked between
	// resolution and the write the command committed.
	ErrSessionGone = errors.New("identity: session revoked")
	// ErrEmailTaken is the unique-key outcome. The route maps it to the
	// `email_taken` field error (INV-01, IDN-054).
	ErrEmailTaken = errors.New("identity: email already registered")
)
