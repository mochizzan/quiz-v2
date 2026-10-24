package participation

// validate.go is the closed validation vocabulary of §5.0.3/§5.0.4: the exact
// field messages, the input shapes, the fail-closed attempt quota, and the
// query-parameter rules of PAR-023/PAR-029/PAR-055/PAR-110. Copy is never
// paraphrased and a query parameter never 422s (PAR-189/§5.0.5).

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/viewmodel"
)

// Field error copy, byte for byte from §5.0.3 (PAR-189 consumers cite it, the
// identity edit screen reuses it through PAR-098).
const (
	ErrNameRequired  = "Nama lengkap wajib diisi."
	ErrNameShort     = "Nama lengkap minimal 2 karakter."
	ErrNameLong      = "Nama lengkap maksimal 100 karakter."
	ErrNameControl   = "Nama lengkap mengandung karakter yang tidak didukung."
	ErrGrade         = "Pilih kelas yang valid."
	ErrMajor         = "Pilih jurusan yang valid."
	ErrMajorRequired = "Nama jurusan wajib diisi."
	ErrMajorShort    = "Nama jurusan minimal 2 karakter."
	ErrMajorLong     = "Nama jurusan maksimal 50 karakter."
	ErrMajorControl  = "Nama jurusan mengandung karakter yang tidak didukung."
	ErrMajorDup      = "Nama jurusan sudah dipakai."
)

// urlSafeCode matches exactly six ASCII digits, which is also the ticket
// secret's only accepted code shape (PAR-029, 8.5.1).
var urlSafeCode = regexp.MustCompile(`^[0-9]{6}$`)

// urlTokenShape matches 1–32 characters of the invite-token alphabet (PAR-037,
// 8.5.3). Tokens are exact and case-sensitive; nothing is normalised.
var urlTokenShape = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// trimCode trims exactly ASCII leading/trailing whitespace. Internal space and
// non-ASCII lookalikes survive and then fail the six-digit test (PAR-028,
// 8.5.2): they become `unknown_code`, never a 422.
func trimCode(raw string) string {
	return strings.Trim(raw, " \t\n\v\f\r")
}

// validCode reports whether the string is an entry-code shape.
func validCode(s string) bool { return urlSafeCode.MatchString(s) }

// validInvite reports whether the token has the invite shape.
func validInvite(s string) bool { return urlTokenShape.MatchString(s) }

// parseID is the path/query integer rule shared by every :id parameter
// (PAR-063: a non-integer or non-positive id is 404).
func parseID(raw string) (int64, bool) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// collapseName trims and collapses internal whitespace runs to one space
// (PAR-055). The result is what is validated, echoed, and stored.
func collapseName(raw string) string {
	return strings.Join(strings.FieldsFunc(raw, isSpaceRune), " ")
}

// isSpaceRune matches Unicode whitespace but keeps control characters for the
// dedicated control-character rejection below.
func isSpaceRune(r rune) bool {
	return unicode.IsSpace(r)
}

// hasControl reports whether any rune cannot be printed safely (5.0.3).
func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// validateName returns the stored form of full_name and its field message
// ("" when valid). Length is in runes (PAR-055).
func validateName(raw string) (string, string) {
	name := collapseName(raw)
	switch {
	case name == "":
		return name, ErrNameRequired
	case hasControl(name):
		return name, ErrNameControl
	case utf8.RuneCountInString(name) < 2:
		return name, ErrNameShort
	case utf8.RuneCountInString(name) > 100:
		return name, ErrNameLong
	default:
		return name, ""
	}
}

// validateGrade returns the grade level and its field message ("" when valid).
func validateGrade(raw string) (uint8, string) {
	switch raw {
	case "10":
		return 10, ""
	case "11":
		return 11, ""
	case "12":
		return 12, ""
	default:
		return 0, ErrGrade
	}
}

// validateMajorID returns the positive integer and its field message. The
// liveness of the major is checked in the transaction (PAR-055: a soft-deleted
// or unknown major is a field error too, delivered after the shared lock).
func validateMajorID(raw string) (int64, string) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n <= 0 {
		return 0, ErrMajor
	}
	return n, ""
}

// validateMajorName is the §5.14 form rule (PAR-110): trim/collapse, 2–50
// runes, at least one letter or digit, no control characters. The letterless
// and control cases share the unsupported-character message; the duplicate
// case is a separate check against the live table.
func validateMajorName(raw string) (string, string) {
	name := collapseName(raw)
	switch {
	case name == "":
		return name, ErrMajorRequired
	case hasControl(name):
		return name, ErrMajorControl
	case utf8.RuneCountInString(name) < 2:
		return name, ErrMajorShort
	case utf8.RuneCountInString(name) > 50:
		return name, ErrMajorLong
	}
	lettered := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			lettered = true
			break
		}
	}
	if !lettered {
		return name, ErrMajorControl
	}
	return name, ""
}

// effectiveAttempts normalises incoherent quiz rows (PAR-081, 8.6.1): a stored
// max_attempts of 0 is treated as 1 and an allow_retake=false row is treated as
// a one-attempt quiz. The boolean is the fail-closed WARN signal (PAR-081's
// "logging WARN"); the caller passes it to the service logger.
func effectiveAttempts(maxAttempts int, allowRetake bool) (limit int, normalised bool) {
	limit = maxAttempts
	if limit <= 0 {
		limit = 1
		normalised = true
	}
	if !allowRetake && limit > 1 {
		limit = 1
		normalised = true
	}
	return limit, normalised
}

// clampPage is the pagination rule of PAR-145/PAR-168 on a raw query value:
// non-numeric, zero or negative falls back to 1; beyond the end clamps to the
// last page. A bad page never 422s (§5.0.5).
func clampPage(raw string, perPage, total int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		n = 1
	}
	pages := (total + perPage - 1) / perPage
	if pages < 1 {
		pages = 1
	}
	if n > pages {
		n = pages
	}
	return n
}

// parseJoinError resolves the `error` query key of GET /join and GET
// /join/identify's not-open path (PAR-023, PAR-024): only unknown_code and
// not_open render on that screen; any other key is ignored (§5.0.5).
func parseJoinError(raw string) (viewmodel.JoinError, bool) {
	e := viewmodel.JoinError(raw)
	switch e {
	case viewmodel.JoinUnknownCode, viewmodel.JoinNotOpen:
		return e, true
	default:
		return "", false
	}
}

// JoinErrorFor exposes parseJoinError to the HTTP adapter: only the two code
// entry outcomes are renderable on GET /join (PAR-023); anything else is
// ignored rather than echoed (A§5.0.5).
func JoinErrorFor(raw string) *viewmodel.JoinError {
	if e, ok := parseJoinError(raw); ok {
		return &e
	}
	return nil
}

// truncateRunes cuts s to at most n runes without splitting one (PAR-165: the
// search q is capped, never rejected).
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	cuts := 0
	for i := range s {
		if cuts == n {
			return s[:i]
		}
		cuts++
	}
	return s
}

// FieldErrorsOf extracts a 422's field map from a domain error; ok is false
// when the failure is not field-scoped, so the adapter can fall through to its
// redirect or error branch. This is the PAR twin of identity.FieldErrorsOf.
func FieldErrorsOf(err error) (map[string]string, bool) {
	var appErr *apperr.Error
	if errors.As(err, &appErr) && appErr.Kind() == apperr.Validation && len(appErr.Fields()) > 0 {
		return appErr.Fields(), true
	}
	return nil, false
}

// likePattern turns a raw search term into an escaped LIKE operand (PAR-165).
// `%` and `_` and the escape character itself are quoted so the search box
// cannot widen the match; the case-insensitivity comes from the column
// collation, not from the pattern.
func likePattern(raw string) string {
	s := strings.ReplaceAll(raw, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return "%" + s + "%"
}
