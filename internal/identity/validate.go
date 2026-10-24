package identity

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/viewmodel"
)

// Validation outcomes. They are sentinels, not user-visible text; FieldMessage
// translates each one into the closed copy set from IDN-080.
var (
	// ErrNameRequired is `name_required`.
	ErrNameRequired = errors.New("identity: name required")
	// ErrNameLong is `name_long`.
	ErrNameLong = errors.New("identity: name too long")
	// ErrPasswordRequired is `password_required`.
	ErrPasswordRequired = errors.New("identity: password required")
	// ErrConfirmRequired is `confirm_required`.
	ErrConfirmRequired = errors.New("identity: password confirmation required")
	// ErrPasswordMismatch is `password_mismatch`.
	ErrPasswordMismatch = errors.New("identity: password confirmation mismatch")
	// ErrCurrentRequired is `current_required`.
	ErrCurrentRequired = errors.New("identity: current password required")
	// ErrGradeRequired is `grade_required`.
	ErrGradeRequired = errors.New("identity: grade required")
	// ErrMajorRequired is `major_required`.
	ErrMajorRequired = errors.New("identity: major required")
	// ErrTokenRequired is `token_required`.
	ErrTokenRequired = errors.New("identity: reset token required")
)

// MaxNameRunes is the IDN-081 ceiling.
const MaxNameRunes = 100

// MaxSearchRunes is the IDN-091 ceiling for `q`. A longer value falls back to
// no search rather than becoming a query.
const MaxSearchRunes = 100

// ValidateName normalises a display name (IDN-081): C0/C1 controls are removed,
// internal whitespace collapses to one ASCII space, and the result must be
// 1…100 Unicode code points. Control characters that are whitespace become a
// space first so `De\tLa` collapses to `De La` rather than `DeLa`.
func ValidateName(raw string) (string, error) {
	clean := make([]rune, 0, len(raw))
	for _, r := range raw {
		if unicode.IsControl(r) {
			if unicode.IsSpace(r) {
				clean = append(clean, ' ')
			}
			continue
		}
		clean = append(clean, r)
	}
	name := secure.NormalizeName(string(clean))
	if name == "" {
		return "", ErrNameRequired
	}
	if utf8.RuneCountInString(name) > MaxNameRunes {
		return "", ErrNameLong
	}
	return name, nil
}

// ValidateEmail applies the single IDN-053 normalisation and maps its two
// outcomes to the identity copy keys.
func ValidateEmail(raw string) (string, error) {
	email, err := secure.NormalizeEmail(raw)
	switch {
	case errors.Is(err, secure.ErrEmailRequired):
		return "", secure.ErrEmailRequired
	case err != nil:
		return "", secure.ErrEmailInvalid
	default:
		return email, nil
	}
}

// ValidatePassword enforces presence and the IDN-057 policy on the raw bytes.
// It never trims: whitespace is a character the user typed and the policy
// rejects it.
func ValidatePassword(raw string) error {
	if raw == "" {
		return ErrPasswordRequired
	}
	return secure.ValidatePassword(raw)
}

// ValidateConfirm checks presence and byte equality of the confirmation. It
// does not normalise anything (IDN-061).
func ValidateConfirm(password, confirm string) error {
	if confirm == "" {
		return ErrConfirmRequired
	}
	if password != confirm {
		return ErrPasswordMismatch
	}
	return nil
}

// ValidateCurrentPassword only checks presence; policy is not applied to an
// existing secret.
func ValidateCurrentPassword(raw string) error {
	if raw == "" {
		return ErrCurrentRequired
	}
	return nil
}

// ParseGradeLevel accepts only the three form values `10`, `11`, `12`
// (IDN-082). Roman numerals are a list-filter convenience, never a form value.
func ParseGradeLevel(raw string) (int, error) {
	switch strings.TrimSpace(raw) {
	case "10":
		return 10, nil
	case "11":
		return 11, nil
	case "12":
		return 12, nil
	default:
		return 0, ErrGradeRequired
	}
}

// ParseGradeFilter is the lenient list-filter parser (IDN-091): `10`, `11`,
// `12`, `x`, `xi`, `xii`, and the `kelas …` spellings, case-insensitive. An
// unrecognised value means "all grades", never a 422.
func ParseGradeFilter(raw string) int {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.TrimSpace(strings.TrimPrefix(v, "kelas"))
	switch strings.TrimSpace(v) {
	case "10", "x":
		return 10
	case "11", "xi":
		return 11
	case "12", "xii":
		return 12
	default:
		return 0
	}
}

// ParseID parses a positive int64 identifier. A malformed, zero, negative, or
// overflowing value is not found (IDN-097, IDN-017).
func ParseID(raw string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// ParseSort clamps the closed list ordering set; an unknown value is `newest`
// (IDN-091, IDN-108).
func ParseSort(raw string) viewmodel.SortOrder {
	switch viewmodel.SortOrder(strings.TrimSpace(raw)) {
	case viewmodel.SortOldest:
		return viewmodel.SortOldest
	case viewmodel.SortNameAsc:
		return viewmodel.SortNameAsc
	case viewmodel.SortNameDesc:
		return viewmodel.SortNameDesc
	default:
		return viewmodel.SortNewest
	}
}

// ParsePage reads a 1-based page number, clamping anything invalid to 1.
func ParsePage(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// NormalizeSearch trims `q` and drops it when it exceeds the IDN-091 ceiling.
// A dropped search means "no filter", not an error.
func NormalizeSearch(raw string) string {
	q := strings.TrimSpace(raw)
	if q == "" || utf8.RuneCountInString(q) > MaxSearchRunes {
		return ""
	}
	return q
}

// EscapeLike escapes the three characters that give LIKE its wildcard
// behaviour. The caller pairs it with `ESCAPE '\\'` (IDN-091, AC-44).
func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// PageCount returns max(1, ceil(total/perPage)) (IDN-094).
func PageCount(total, perPage int) int {
	if perPage < 1 {
		perPage = PageSize
	}
	if total <= 0 {
		return 1
	}
	return (total + perPage - 1) / perPage
}

// ClampPage returns max(1, min(page, pages)) (IDN-094).
func ClampPage(page, total, perPage int) int {
	if page < 1 {
		page = 1
	}
	if pages := PageCount(total, perPage); page > pages {
		page = pages
	}
	return page
}

// FieldMessage maps a validation sentinel to the exact input name and copy key
// it renders under (IDN-079, IDN-080). ok is false for errors that are not
// field-scoped.
func FieldMessage(err error) (field, message string, ok bool) {
	switch {
	case errors.Is(err, ErrNameRequired):
		return "full_name", MsgNameRequired, true
	case errors.Is(err, ErrNameLong):
		return "full_name", MsgNameLong, true
	case errors.Is(err, secure.ErrEmailRequired):
		return "email", apperr.MsgEmailRequired, true
	case errors.Is(err, secure.ErrEmailInvalid):
		return "email", apperr.MsgEmailInvalid, true
	case errors.Is(err, ErrPasswordRequired):
		return "password", MsgPasswordRequired, true
	case errors.Is(err, secure.ErrPasswordPolicy):
		return "password", apperr.MsgPasswordPolicy, true
	case errors.Is(err, secure.ErrPasswordTooLong):
		return "password", apperr.MsgPasswordTooLong, true
	case errors.Is(err, ErrConfirmRequired):
		return "password_confirm", MsgConfirmRequired, true
	case errors.Is(err, ErrPasswordMismatch):
		return "password_confirm", apperr.MsgPasswordMismatch, true
	case errors.Is(err, ErrCurrentRequired):
		return "current_password", MsgCurrentRequired, true
	case errors.Is(err, ErrGradeRequired):
		return "grade_level", MsgGradeRequired, true
	case errors.Is(err, ErrMajorRequired):
		return "major_id", MsgMajorRequired, true
	case errors.Is(err, ErrTokenRequired):
		return "token", MsgTokenRequired, true
	case errors.Is(err, ErrTokenInvalid):
		return "token", apperr.MsgInvalidToken, true
	case errors.Is(err, ErrTokenUsed):
		return "token", apperr.MsgTokenUsed, true
	case errors.Is(err, ErrEmailTaken):
		return "email", apperr.MsgEmailTaken, true
	case errors.Is(err, ErrWrongPassword):
		return "current_password", apperr.MsgCurrentPassword, true
	case errors.Is(err, ErrSamePassword):
		return "new_password", apperr.MsgSamePassword, true
	default:
		return "", "", false
	}
}

// AddFieldError adds a validation sentinel to a field-error map. Errors that
// are not field-scoped are ignored here and handled by the route.
func AddFieldError(fields map[string]string, err error) {
	if field, message, ok := FieldMessage(err); ok {
		fields[field] = message
	}
}

// Rate-limit budgets and the fixed window (IDN-063). They are package
// constants so no handler invents a number.
const (
	// RateWindow is the fixed 15-minute window every bucket uses.
	RateWindow = 15 * time.Minute
	// LoginIPLimit is the per-IP login ceiling.
	LoginIPLimit = 30
	// LoginEmailLimit is the per-(email, IP) failure ceiling.
	LoginEmailLimit = 5
	// RegisterLimit is the per-IP ceiling for either registration form.
	RegisterLimit = 5
	// ResetConsumeLimit is the per-IP ceiling for reset consumption.
	ResetConsumeLimit = 10
	// ChangePasswordLimit is the per-account wrong-current ceiling.
	ChangePasswordLimit = 5
	// ResetIssueLimit is the per-teacher reset-token issue ceiling.
	ResetIssueLimit = 10
)

// Rate-limit key builders. Each purpose has its own namespace so a login
// failure can never consume a registration budget (IDN-062, IDN-063).
func LoginIPKey(ip string) string { return "login:ip:" + ip }

// LoginEmailKey is keyed by the normalised email and the peer address so
// casing variants share one bucket while a whole class behind one NAT does not.
func LoginEmailKey(email, ip string) string { return "login:email:" + email + ":" + ip }

// RegisterTeacherKey is the teacher-registration IP bucket.
func RegisterTeacherKey(ip string) string { return "register:teacher:" + ip }

// RegisterStudentKey is the student-registration IP bucket.
func RegisterStudentKey(ip string) string { return "register:student:" + ip }

// ResetConsumeKey is the reset-consumption IP bucket.
func ResetConsumeKey(ip string) string { return "reset:consume:" + ip }

// ChangePasswordKey is the per-account wrong-current bucket.
func ChangePasswordKey(accountID int64) string {
	return "change-password:" + strconv.FormatInt(accountID, 10)
}

// ResetIssueKey is the per-teacher reset-issue bucket.
func ResetIssueKey(accountID int64) string {
	return "reset-issue:" + strconv.FormatInt(accountID, 10)
}
