// Package apperr is the one error vocabulary every handler returns. It maps a
// failure to exactly one HTTP status and one Indonesian message, so no handler
// invents a status code and no template invents copy.
//
// The taxonomy is fixed by business-logic spec OPS-200…OPS-216 and contract
// §3 R-19. A handler returns one of these; the HTTP layer renders it. Anything
// that is not an *Error is treated as an internal failure (500) and its text
// is never shown to a user.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind is the failure class. There are exactly four user-visible outcomes plus
// the internal one; there is no 401, 405, or 409 in this system.
type Kind uint8

const (
	// NotFound is missing, soft-deleted, wrong-role, wrong-owner, or otherwise
	// invisible. All of them share one body (OPS-204).
	NotFound Kind = iota + 1
	// Forbidden is CSRF failure or a pending teacher with correct credentials.
	// It is the only 403 in the system (IDN-013).
	Forbidden
	// Validation re-renders the submitting form at 422 with field errors.
	Validation
	// RateLimited answers 429 with Retry-After and no form (OPS-205).
	RateLimited
	// Internal is any database, invariant, or render failure (OPS-206).
	Internal
)

// String names the kind for log lines.
func (k Kind) String() string {
	switch k {
	case NotFound:
		return "not_found"
	case Forbidden:
		return "forbidden"
	case Validation:
		return "validation"
	case RateLimited:
		return "rate_limited"
	default:
		return "internal"
	}
}

// Status returns the HTTP status this kind renders as.
func (k Kind) Status() int {
	switch k {
	case NotFound:
		return http.StatusNotFound
	case Forbidden:
		return http.StatusForbidden
	case Validation:
		return http.StatusUnprocessableEntity
	case RateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// Error is the value handlers return. Message is safe to render: it is either
// one of the constants below or a domain-owned string that has been through
// review. Cause and Rule are for logs only and never reach a response.
type Error struct {
	kind       Kind
	message    string
	fields     map[string]string
	retryAfter int
	rule       string
	cause      error
}

// Kind reports the failure class.
func (e *Error) Kind() Kind { return e.kind }

// Message returns the renderable Indonesian copy.
func (e *Error) Message() string { return e.message }

// Fields returns the field-keyed validation messages for a 422.
func (e *Error) Fields() map[string]string { return e.fields }

// RetryAfter returns the whole seconds a 429 should advertise.
func (e *Error) RetryAfter() int { return e.retryAfter }

// Rule returns the spec rule or invariant ID bound to this failure, when one
// was attached. It is what structured logs cite (OPS-219).
func (e *Error) Rule() string { return e.rule }

// Error implements error. It never includes the cause, because this string can
// end up in a log line and must not carry SQL, tokens, or user text.
func (e *Error) Error() string {
	if e.rule != "" {
		return fmt.Sprintf("apperr[%s]: %s (rule %s)", e.kind, e.message, e.rule)
	}
	return fmt.Sprintf("apperr[%s]: %s", e.kind, e.message)
}

// Unwrap exposes the internal cause to errors.Is/As for logging only.
func (e *Error) Unwrap() error { return e.cause }

// Status returns the HTTP status for this error.
func (e *Error) Status() int { return e.kind.Status() }

// WithRule attaches a spec rule ID for logs and returns the same error.
func (e *Error) WithRule(rule string) *Error {
	e.rule = rule
	return e
}

// NotFoundf returns the single 404 outcome. Callers do not pick copy: the
// error page is identical for a missing row, a foreign owner, and a wrong role.
func NotFoundf() *Error { return &Error{kind: NotFound, message: MsgNotFound} }

// Forbiddenf returns a 403 with the given message. An empty message selects the
// CSRF copy, the common case.
func Forbiddenf(msg string) *Error {
	if msg == "" {
		msg = MsgCSRF
	}
	return &Error{kind: Forbidden, message: msg}
}

// Validationf returns a 422 carrying field errors keyed by input name.
func Validationf(fields map[string]string) *Error {
	return &Error{kind: Validation, message: MsgValidation, fields: fields}
}

// Field returns a 422 for exactly one field.
func Field(name, msg string) *Error {
	return Validationf(map[string]string{name: msg})
}

// RateLimitedf returns a 429 that must advertise at least one second.
func RateLimitedf(retryAfter int) *Error {
	if retryAfter < 1 {
		retryAfter = 1
	}
	return &Error{kind: RateLimited, message: MsgRateLimited, retryAfter: retryAfter}
}

// Internalf wraps an unexpected failure. cause is logged, never rendered.
func Internalf(cause error) *Error {
	return &Error{kind: Internal, message: MsgServerError, cause: cause}
}

// From normalises any error into an *Error. A nil input is nil; an already
// classified error is returned unchanged; everything else becomes a 500.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Internalf(err)
}

// IsNotFound reports whether err is a 404.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.kind == NotFound
}

// Validation message keys shared across domains (contract §3 R-19). Domain
// specs add their own strings; these are the ones more than one domain needs.
const (
	MsgValidation = "Periksa kembali isian Anda."

	MsgNotFound    = "Alamat yang Anda tuju tidak tersedia atau sudah dipindahkan."
	MsgCSRF        = "Sesi formulir tidak valid. Muat ulang halaman."
	MsgRateLimited = "Terlalu banyak percobaan. Coba lagi nanti."
	MsgServerError = "Permintaan Anda belum dapat diproses. Coba muat ulang beberapa saat lagi."
)

// The following are the R-19 global copy keys. They are duplicated here as
// constants because the same sentence is needed by both the HTTP layer (which
// renders 403/429/500) and by handlers (which set flash keys and field errors).
const (
	MsgInvalidCredentials = "Email atau kata sandi salah."
	MsgEmailTaken         = "Email sudah terdaftar."
	MsgPasswordPolicy     = "Kata sandi minimal 8 karakter, tanpa spasi, mengandung angka dan huruf besar."
	MsgPasswordTooLong    = "Kata sandi maksimal 72 karakter."
	MsgPasswordMismatch   = "Konfirmasi kata sandi tidak cocok."
	MsgCurrentPassword    = "Kata sandi saat ini salah."
	MsgSamePassword       = "Kata sandi baru harus berbeda dari kata sandi saat ini."
	MsgInvalidToken       = "Token tidak valid atau sudah kedaluwarsa."
	MsgTokenUsed          = "Token sudah digunakan."
	MsgSessionExpired     = "Sesi Anda berakhir. Silakan masuk lagi."
	MsgPendingTeacher     = "Menunggu persetujuan guru lain."
	MsgMustChangePassword = "Ubah kata sandi Anda sebelum melanjutkan."
	MsgEmailRequired      = "Email wajib diisi."
	MsgEmailInvalid       = "Format email tidak valid."
	MsgFieldRequired      = "Wajib diisi."
)
