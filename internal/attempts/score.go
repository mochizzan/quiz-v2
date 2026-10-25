package attempts

import "fmt"

// score.go owns the whole numeric contract of the domain (ATT-002, ATT-143,
// ATT-174, ATT-242, ATT-244). Every conversion below is integer math on the
// stored 4-dp units, so a float can never round a value a second time, and
// every screen that shows a score goes through exactly one of these functions.

// scoreScale is the number of stored units per 1.0 score (DECIMAL(5,4)).
const scoreScale = 10000

// ScoreUnits converts a stored DECIMAL(5,4) value (scanned as float64) into
// its integer units, rounding half-up at the 4th decimal. Values outside
// 0..1 are clamped defensively: the CHECK constraints guarantee the range,
// so a witness would be a corrupted row (ATT-259).
func ScoreUnits(v float64) int {
	u := int(roundHalfUp(v * float64(scoreScale)))
	return clampUnits(u)
}

// roundHalfUp rounds x half away from zero, which is half-up for the non-negative
// domain used here.
func roundHalfUp(x float64) float64 {
	if x >= 0 {
		return float64(int(x + 0.5))
	}
	return float64(int(x - 0.5))
}

func clampUnits(u int) int {
	if u < 0 {
		return 0
	}
	if u > scoreScale {
		return scoreScale
	}
	return u
}

// UnitsDecimal renders units as the DECIMAL(5,4) literal the driver writes:
// 8550 → "0.8550", 10000 → "1.0000". Scores are stored on the 0–1 scale, never
// as 0–100 (ATT-002), and the text form avoids float formatting entirely.
func UnitsDecimal(units int) string {
	return fmt.Sprintf("%d.%04d", units/scoreScale, units%scoreScale)
}

// DisplayPercent implements ATT-242: p = floor((units + 50) / 100), clamped to
// 0–100. It is the only 0–1 → 0–100 conversion in the domain, half-up at the
// whole-percent boundary: 0→0, 49→0, 50→1, 8449→84, 8450→85, 9950→100,
// 10000→100.
func DisplayPercent(units int) int {
	return clampUnitsPct((clampUnits(units) + 50) / 100)
}

func clampUnitsPct(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// ProgressPercent is the answered share of a header or monitor rail
// (ATT-117, ATT-184): floor(answered × 100 / total), clamped, 0 when total
// is 0.
func ProgressPercent(answered, total int) int {
	if total <= 0 {
		return 0
	}
	return clampUnitsPct(answered * 100 / total)
}

// FinalScoreUnits implements ATT-143 over the stored units of all n slots
// (scores non-null, 0–10000 each): units = floor((2ΣU + n) / 2n). It returns
// ok=false when the set is not complete (no slots, count mismatch, or a slot
// value outside the scale), which leaves final_score NULL (ATT-INV-06).
// Worked examples: {10000,0,0} n=3 → 3333; {10000,6667} n=2 → 8334.
func FinalScoreUnits(scores []int, slots int) (int, bool) {
	if len(scores) == 0 || slots == 0 {
		return 0, false
	}
	if len(scores) != slots {
		return 0, false
	}
	sum := 0
	for _, u := range scores {
		if u < 0 || u > scoreScale {
			return 0, false
		}
		sum += u
	}
	n := slots
	units := (2*sum + n) / (2 * n)
	return clampUnits(units), true
}

// AverageTenths implements ATT-174: tenths = floor((2ΣU + 10n) / 20n) over
// every submitted attempt with a non-null final (no filters, no
// show_score gate). It returns ok=false when there is no attempt with a score,
// which renders "—" (ATT-175). 7250 over one attempt → 725 (72,5%); over two →
// 363 (36,3%).
func AverageTenths(finals []int) (int, bool) {
	n := len(finals)
	if n == 0 {
		return 0, false
	}
	sum := 0
	for _, u := range finals {
		sum += clampUnits(u)
	}
	return (2*sum + 10*n) / (20 * n), true
}

// ScoreTenths converts stored units to a one-decimal percentage, half-up:
// 8550 → 855 ("85,5"), 8450 → 845, 8449 → 845, 10000 → 1000. It is the
// grade-detail representation required by ATT-212; other screens use the
// whole-percent DisplayPercent (ATT-242).
func ScoreTenths(units int) int {
	return (clampUnits(units) + 5) / 10
}

// TenthsPercent rounds a tenths-of-a-percent value to the nearest whole
// percent, half-up. It exists only where a renderer can express whole
// percents (a viewmodel.Score): 725 → 73. The one-decimal average string is
// TenthsLabel.
func TenthsPercent(tenths int) int {
	return clampUnitsPct((tenths + 5) / 10)
}

// PercentLabel renders a whole-percent numeral with the percent sign, e.g.
// "84%". A raw 0–1 value never reaches a template (ATT-244).
func PercentLabel(percent int) string {
	return fmt.Sprintf("%d%%", clampUnitsPct(percent))
}

// TenthsLabel renders a one-decimal average with the Indonesian comma:
// 725 → "72,5%", 363 → "36,3%", 1000 → "100,0%" (ATT-244).
func TenthsLabel(tenths int) string {
	return TenthsValue(tenths) + "%"
}

// TenthsValue renders the numeral half of TenthsLabel, for a form field that
// must round-trip a stored one-decimal grade: 855 → "85,5".
func TenthsValue(tenths int) string {
	if tenths < 0 {
		tenths = 0
	}
	if tenths > 10*100 {
		tenths = 10 * 100
	}
	return fmt.Sprintf("%d,%d", tenths/10, tenths%10)
}
