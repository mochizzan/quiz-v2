package authoring

import (
	"strings"
	"time"
	"unicode/utf8"

	"quiz-v2/internal/viewmodel"
)

// MaxImageBytes is the business image cap (10 MiB). The transport cap is the
// foundation's, and AUT-034 explains why the two differ.
const MaxImageBytes = 10 << 20

// MaxQuestions is the live-question cap for one quiz (AUT-009/AUT-010).
const MaxQuestions = 200

// MaxOptions is the option cap derived from the A–Z option letter formatter.
const MaxOptions = 26

// NormalizeText is the single boundary normalisation for authored text
// (AUT-023): CRLF and lone CR become LF, NUL bytes are removed, and leading and
// trailing whitespace is trimmed. Internal newlines and tabs are preserved.
func NormalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimSpace(s)
}

// RuneLen counts Unicode code points, matching the utf8mb4 column limits.
func RuneLen(s string) int { return utf8.RuneCountInString(s) }

// TruncateRunes returns at most n leading code points. It is used for the
// search box, where excess input is ignored rather than rejected (AUT-001).
func TruncateRunes(s string, n int) string {
	if n < 0 {
		return ""
	}
	if RuneLen(s) <= n {
		return s
	}
	out := make([]rune, 0, n)
	for _, r := range s {
		if len(out) == n {
			break
		}
		out = append(out, r)
	}
	return string(out)
}

// ParseLocalDateTime parses a datetime-local value as fixed WIB (UTC+7), with
// optional seconds truncated to the minute, and returns it in UTC. A blank
// value is absent; a value carrying an offset or Z is invalid because the
// input contract is wall-clock local (AUT-037).
func ParseLocalDateTime(raw string) (time.Time, bool, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, true
	}
	if strings.ContainsAny(raw, "Zz+") {
		return time.Time{}, true, false
	}
	var (
		t   time.Time
		err error
	)
	switch len(raw) {
	case len("2006-01-02T15:04"):
		t, err = time.ParseInLocation("2006-01-02T15:04", raw, viewmodel.WIB)
	case len("2006-01-02T15:04:05"):
		t, err = time.ParseInLocation("2006-01-02T15:04:05", raw, viewmodel.WIB)
	default:
		return time.Time{}, true, false
	}
	if err != nil {
		return time.Time{}, true, false
	}
	return t.Truncate(time.Minute).UTC(), true, true
}

// LocalTimePtr converts a nullable database timestamp into the view model's
// WIB-normalised pointer.
func LocalTimePtr(t *time.Time) *viewmodel.LocalTime {
	if t == nil {
		return nil
	}
	v := viewmodel.NewLocalTime(*t)
	return &v
}
