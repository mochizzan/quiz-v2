package export

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

var gradingHeaders = []string{
	"ID peserta", "Nama", "Kelas", "Percobaan", "Dikirim", "Status",
}

func (b *builder) buildGrading() {
	sheet := sheetGrading
	n := len(b.data.Questions)

	for i, h := range gradingHeaders {
		b.setStr(sheet, columnName(i+1)+"1", h)
	}
	for i := 0; i < n; i++ {
		soalRow := 2 + i
		b.setFormula(sheet, columnName(7+i)+"1", fmt.Sprintf(`"S"&Soal!$A$%d`, soalRow))
		b.setFormula(sheet, columnName(7+n+i)+"1", fmt.Sprintf(`"N"&Soal!$A$%d`, soalRow))
	}
	b.setStr(sheet, b.finalCol+"1", "Skor akhir")
	b.styleRange(sheet, "A1", b.finalCol+"1", b.st.header)
	b.rowHeight(sheet, 1, 24)

	b.styleRange(sheet, "A2", fmt.Sprintf("%s%d", b.finalCol, b.aLast), b.st.body)

	for i, at := range b.data.Attempts {
		row := 2 + i
		b.setNumber(sheet, fmt.Sprintf("A%d", row), at.RegistrationID)
		b.setFormula(sheet, fmt.Sprintf("B%d", row), fmt.Sprintf(
			`IF($A%d="","",INDEX(Peserta!$B$2:$B$%d,MATCH($A%d,Peserta!$A$2:$A$%d,0)))`,
			row, b.pLast, row, b.pLast))
		b.setFormula(sheet, fmt.Sprintf("C%d", row), fmt.Sprintf(
			`IF($A%d="","",INDEX(Peserta!$D$2:$D$%d,MATCH($A%d,Peserta!$A$2:$A$%d,0)))`,
			row, b.pLast, row, b.pLast))
		b.setNumber(sheet, fmt.Sprintf("D%d", row), at.Number)
		b.setDate(sheet, fmt.Sprintf("E%d", row), at.SubmittedAt)
		b.setFormula(sheet, fmt.Sprintf("F%d", row), fmt.Sprintf(
			`IF(%s%d="","Menunggu","Selesai")`, b.finalCol, row))

		for qOut := 0; qOut < n; qOut++ {
			ans := answerAt(at, b.qOrder[qOut])
			text := "—"
			if ans != nil && strings.TrimSpace(ans.Text) != "" {
				text = ans.Text
			}
			b.setStr(sheet, columnName(7+qOut)+fmt.Sprintf("%d", row), text)
			if ans != nil && ans.ScoreUnits != nil {
				b.setNumber(sheet, columnName(7+n+qOut)+fmt.Sprintf("%d", row),
					float64(clampScoreUnits(*ans.ScoreUnits))/100)
			}
		}

		if n == 0 {
			// No questions: a blank final is the only division-safe result.
			b.setFormula(sheet, b.finalCol+fmt.Sprintf("%d", row), `""`)
		} else {
			scores := fmt.Sprintf("%s%d:%s%d",
				columnName(7+n), row, columnName(7+2*n-1), row)
			b.setFormula(sheet, b.finalCol+fmt.Sprintf("%d", row), finalScoreFormula(scores))
		}

		b.styleCell(sheet, fmt.Sprintf("A%d", row), b.st.integer)
		b.styleCell(sheet, fmt.Sprintf("D%d", row), b.st.integer)
		b.styleCell(sheet, fmt.Sprintf("E%d", row), b.st.date)
		b.styleCell(sheet, b.finalCol+fmt.Sprintf("%d", row), b.st.integer)
		for qOut := 0; qOut < n; qOut++ {
			b.styleCell(sheet, columnName(7+n+qOut)+fmt.Sprintf("%d", row), b.st.score)
		}
		b.rowHeight(sheet, row, 36)
	}

	b.width(sheet, "A", "A", 10)
	b.width(sheet, "B", "B", 28)
	b.width(sheet, "C", "C", 14)
	b.width(sheet, "D", "D", 12)
	b.width(sheet, "E", "E", 18)
	b.width(sheet, "F", "F", 12)
	for i := 0; i < n; i++ {
		b.width(sheet, columnName(7+i), columnName(7+i), 28)
		b.width(sheet, columnName(7+n+i), columnName(7+n+i), 8)
	}
	b.width(sheet, b.finalCol, b.finalCol, 12)

	b.freeze(sheet, "G2", 6, 1, "bottomRight")
	b.autoFilter(sheet, fmt.Sprintf("A1:%s%d", b.finalCol, b.aLast))
	b.bestAttemptFormat(fmt.Sprintf("A2:%s%d", b.finalCol, b.aLast))

	note := "Baris dengan skor akhir terbaik setiap peserta disorot; jika ada nilai seri, semua barisnya disorot."
	b.setStr(sheet, fmt.Sprintf("A%d", b.aLast+2), note)
	b.styleCell(sheet, fmt.Sprintf("A%d", b.aLast+2), b.st.note)
}

// finalScoreFormula reproduces FinalScoreUnits + DisplayPercent with integer
// arithmetic in Excel: recover the stored units with ROUND(x*100,0), apply the
// two FLOORs, and leave the cell blank when any score is still missing.
func finalScoreFormula(scores string) string {
	return fmt.Sprintf(
		`IF(OR(Ringkasan!$B$9=0,COUNT(%s)<Ringkasan!$B$9),"",FLOOR((FLOOR((2*SUMPRODUCT(ROUND(%s*100,0))+Ringkasan!$B$9)/(2*Ringkasan!$B$9),1)+50)/100,1))`,
		scores, scores)
}

func (b *builder) bestAttemptFormat(rangeRef string) {
	if b.err != nil {
		return
	}
	// AGGREGATE is newer than the OOXML baseline, so Excel requires the
	// _xlfn. prefix in the stored formula. Without it the rule parses but
	// silently never matches, leaving every row unhighlighted.
	formula := fmt.Sprintf(
		`AND($%s2<>"",$%s2=IFERROR(_xlfn.AGGREGATE(14,6,$%s$2:$%s$%d/($A$2:$A$%d=$A2),1),""))`,
		b.finalCol, b.finalCol, b.finalCol, b.finalCol, b.aLast, b.aLast)
	b.fail(b.f.SetConditionalFormat(sheetGrading, rangeRef, []excelize.ConditionalFormatOptions{{
		Type:     "formula",
		Criteria: formula,
		Format:   &b.st.best,
	}}))
}
