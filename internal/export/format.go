package export

import (
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"quiz-v2/internal/attempts"
	"quiz-v2/internal/viewmodel"
)

// maxCellUTF16 is the safety cap for one cell. Excel's hard limit is 32767
// UTF-16 code units; the spec asks for 32000 characters, so the two limits
// leave the same margin.
const maxCellUTF16 = 32000

// sanitizeCell removes the control characters Excel rejects and caps the value
// at 32000 UTF-16 units. A truncated value ends with an ellipsis so the reader
// can tell it was cut. Everything untrusted goes through this before SetCellStr.
func sanitizeCell(value string) string {
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(r)
		case r >= 0x00 && r <= 0x08:
			// drop
		case r == 0x0B || r == 0x0C:
			// drop
		case r >= 0x0E && r <= 0x1F:
			// drop
		default:
			b.WriteRune(r)
		}
	}
	out := strings.ToValidUTF8(b.String(), "\uFFFD")

	total := 0
	for _, r := range out {
		total += utf16.RuneLen(r)
	}
	if total <= maxCellUTF16 {
		return out
	}

	limit := maxCellUTF16 - 1 // reserve one unit for the ellipsis
	units := 0
	for i, r := range out {
		n := utf16.RuneLen(r)
		if n < 1 {
			n = 1
		}
		if units+n > limit {
			return out[:i] + "…"
		}
		units += n
	}
	return out
}

// excelTime turns an instant into the UTC time whose wall-clock components are
// the WIB wall-clock components. Excelize converts a time.Time by instant, so
// this is what makes the stored serial display as WIB.
func excelTime(t time.Time) time.Time {
	w := t.In(viewmodel.WIB)
	return time.Date(w.Year(), w.Month(), w.Day(), w.Hour(), w.Minute(), w.Second(), 0, time.UTC)
}

// columnName converts a 1-based column number to its Excel letters.
func columnName(n int) string {
	if n < 1 {
		return ""
	}
	var letters []byte
	for n > 0 {
		n--
		letters = append([]byte{byte('A' + n%26)}, letters...)
		n /= 26
	}
	return string(letters)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// effectiveMaxAttempts is the value shown in Ringkasan!B10. A quiz with retakes
// off stores 0; the effective limit is one attempt.
func effectiveMaxAttempts(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// sortedParticipants returns a copy sorted by name then id, the order used by
// both Peserta and the hidden ranking helper.
func sortedParticipants(ps []attempts.ExportParticipant) []attempts.ExportParticipant {
	out := append([]attempts.ExportParticipant(nil), ps...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if an != bn {
			return an < bn
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.RegistrationID < b.RegistrationID
	})
	return out
}

// participantsWithAttempts returns the participants that have at least one
// submitted attempt, sorted by name then id. Waiting attempts count: they are
// submitted even though they have no final score.
func participantsWithAttempts(ps []attempts.ExportParticipant, ats []attempts.ExportAttempt) []attempts.ExportParticipant {
	ids := make(map[int64]bool, len(ats))
	for _, a := range ats {
		ids[a.RegistrationID] = true
	}
	out := make([]attempts.ExportParticipant, 0, len(ps))
	for _, p := range ps {
		if ids[p.RegistrationID] {
			out = append(out, p)
		}
	}
	return sortedParticipants(out)
}

// questionOrder maps each Soal row to the question's index in the read model.
// The read model already orders by position; sorting again is a cheap guard and
// keeps Penilaian's response columns aligned with the same order.
func questionOrder(qs []attempts.ExportQuestion) []int {
	order := make([]int, len(qs))
	for i := range order {
		order[i] = i
	}
	allPositioned := true
	for _, q := range qs {
		if q.Position <= 0 {
			allPositioned = false
			break
		}
	}
	if allPositioned {
		sort.SliceStable(order, func(i, j int) bool {
			return qs[order[i]].Position < qs[order[j]].Position
		})
	}
	return order
}

// optionOrder is questionOrder for options. Canonical option letters are the
// index in this order, not the stored Position field.
func optionOrder(opts []attempts.ExportOption) []int {
	order := make([]int, len(opts))
	for i := range order {
		order[i] = i
	}
	allPositioned := true
	for _, o := range opts {
		if o.Position <= 0 {
			allPositioned = false
			break
		}
	}
	if allPositioned {
		sort.SliceStable(order, func(i, j int) bool {
			return opts[order[i]].Position < opts[order[j]].Position
		})
	}
	return order
}

func answerAt(at attempts.ExportAttempt, questionIndex int) *attempts.ExportAnswer {
	if questionIndex < 0 || questionIndex >= len(at.Answers) {
		return nil
	}
	return &at.Answers[questionIndex]
}

func clampScoreUnits(units int) int {
	if units < 0 {
		return 0
	}
	if units > 10000 {
		return 10000
	}
	return units
}
