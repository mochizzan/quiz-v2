package viewmodel

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Score is a preformatted percentage on the 0–100 display scale. Storage keeps
// the 0–1 ratio; handlers convert once when they build the view model, so no
// template ever multiplies or divides.
type Score struct {
	Percent int
}

// NewScore rounds a 0–100 percentage to the nearest whole point.
func NewScore(percent float64) Score {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return Score{Percent: int(percent + 0.5)}
}

// NewScoreFromRatio converts the stored 0–1 ratio to the display score.
func NewScoreFromRatio(ratio float64) Score { return NewScore(ratio * 100) }

// Int returns the whole percentage.
func (s Score) Int() int { return s.Percent }

// String renders "84%".
func (s Score) String() string { return strconv.Itoa(s.Percent) + "%" }

// Tone maps a score to the closest semantic theme. The numeral is always shown
// beside the colour, so the tone is never the only signal.
func (s Score) Tone() string {
	switch {
	case s.Percent >= 80:
		return "success"
	case s.Percent >= 60:
		return "warning"
	default:
		return "danger"
	}
}

// Average is a preformatted mean with one decimal and an Indonesian comma.
type Average struct {
	Value float64
}

// NewAverage rounds to one decimal.
func NewAverage(v float64) Average { return Average{Value: v} }

// String renders "84,5%".
func (a Average) String() string {
	return strings.ReplaceAll(fmt.Sprintf("%.1f%%", a.Value), ".", ",")
}

// WIB is the display timezone. All server-side dates are normalised to it
// before they reach a template.
var WIB = time.FixedZone("WIB", 7*60*60)

var monthShort = [...]string{
	"Jan", "Feb", "Mar", "Apr", "Mei", "Jun",
	"Jul", "Agu", "Sep", "Okt", "Nov", "Des",
}

var monthLong = [...]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

var dayLong = [...]string{
	"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu",
}

// LocalTime is a WIB-normalised date. Methods are the only way templates
// format dates: the raw time.Time is deliberately unexported so a template
// cannot print Go's default layout.
type LocalTime struct {
	t time.Time
}

// NewLocalTime normalises any instant to WIB.
func NewLocalTime(t time.Time) LocalTime { return LocalTime{t: t.In(WIB)} }

// LocalDate builds a WIB date from wall-clock components.
func LocalDate(year int, month time.Month, day, hour, minute int) LocalTime {
	return LocalTime{t: time.Date(year, month, day, hour, minute, 0, 0, WIB)}
}

// Ptr returns a pointer, for optional model fields.
func Ptr(t LocalTime) *LocalTime { return &t }

// LocalDatePtr is Ptr(LocalDate(...)) without the nesting.
func LocalDatePtr(year int, month time.Month, day, hour, minute int) *LocalTime {
	t := LocalDate(year, month, day, hour, minute)
	return &t
}

// Time returns the underlying instant.
func (t LocalTime) Time() time.Time { return t.t }

// IsZero reports whether the value was left unset.
func (t LocalTime) IsZero() bool { return t.t.IsZero() }

// Input renders the value for <input type="datetime-local">: 2026-10-23T08:00.
func (t LocalTime) Input() string {
	if t.t.IsZero() {
		return ""
	}
	return t.t.Format("2006-01-02T15:04")
}

// DateInput renders the value for <input type="date">: 2026-10-23.
func (t LocalTime) DateInput() string {
	if t.t.IsZero() {
		return ""
	}
	return t.t.Format("2006-01-02")
}

// Short renders "23 Okt 2026".
func (t LocalTime) Short() string {
	if t.t.IsZero() {
		return "—"
	}
	return fmt.Sprintf("%d %s %d", t.t.Day(), monthShort[int(t.t.Month())-1], t.t.Year())
}

// Long renders "Jumat, 23 Oktober 2026".
func (t LocalTime) Long() string {
	if t.t.IsZero() {
		return "—"
	}
	return fmt.Sprintf("%s, %d %s %d", dayLong[int(t.t.Weekday())], t.t.Day(), monthLong[int(t.t.Month())-1], t.t.Year())
}

// DayDate renders "23 Okt".
func (t LocalTime) DayDate() string {
	if t.t.IsZero() {
		return "—"
	}
	return fmt.Sprintf("%d %s", t.t.Day(), monthShort[int(t.t.Month())-1])
}

// Clock renders "08.00" (Indonesian decimal separator).
func (t LocalTime) Clock() string {
	if t.t.IsZero() {
		return "—"
	}
	return t.t.Format("15.04")
}

// DateTime renders "23 Okt 2026 · 08.00".
func (t LocalTime) DateTime() string {
	if t.t.IsZero() {
		return "—"
	}
	return t.Short() + " · " + t.Clock()
}

// DateTimeRange renders a start–end pair on one line, e.g.
// "23 Okt 2026 · 08.00–12.00".
func (t LocalTime) DateTimeRange(end LocalTime) string {
	if t.t.IsZero() {
		return "—"
	}
	if end.t.IsZero() {
		return t.DateTime()
	}
	if t.Short() == end.Short() {
		return t.Short() + " · " + t.Clock() + "–" + end.Clock()
	}
	return t.DateTime() + " – " + end.DateTime()
}

// RelativeShort renders the distance from now in Indonesian, rounded to the
// largest useful unit. It is used for "menunggu 2 hari" labels.
func (t LocalTime) RelativeShort(now LocalTime) string {
	if t.t.IsZero() {
		return "—"
	}
	d := now.t.Sub(t.t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "baru saja"
	case d < time.Hour:
		return fmt.Sprintf("%d menit", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d jam", int(d.Hours()))
	default:
		return fmt.Sprintf("%d hari", int(d.Hours()/24))
	}
}

// Initials returns at most two uppercase initials from a display name.
func Initials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}
	first := []rune(parts[0])
	out := string(first[0])
	if len(parts) > 1 {
		second := []rune(parts[len(parts)-1])
		out += string(second[0])
	}
	return strings.ToUpper(out)
}

// RomanGrade converts 10/11/12 to the Indonesian class numeral.
func RomanGrade(grade int) string {
	switch grade {
	case 10:
		return "X"
	case 11:
		return "XI"
	case 12:
		return "XII"
	default:
		return "—"
	}
}

// ClassLabel composes "XII IPA". Major may be empty for anonymous participants
// whose only recorded audience is the grade.
func ClassLabel(grade int, major string) string {
	switch {
	case grade == 0 && major == "":
		return "—"
	case grade == 0:
		return major
	case major == "":
		return RomanGrade(grade)
	default:
		return RomanGrade(grade) + " " + major
	}
}

// Number renders an integer with Indonesian thousands separators: 1240 →
// "1.240".
func Number(n int) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return sign + b.String()
}

// FileSize renders bytes with one decimal and an Indonesian comma.
func FileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	value := float64(bytes) / float64(div)
	return strings.ReplaceAll(fmt.Sprintf("%.1f %cB", value, "KMGT"[exp]), ".", ",")
}

// OptionLetter converts a zero-based option index to A, B, C, …
func OptionLetter(index int) string {
	if index < 0 {
		return ""
	}
	return string(rune('A' + index))
}
