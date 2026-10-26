package export

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

var gradingHeaders = []string{
	"No", "Nama", "Kelas", "Percobaan", "Dikirim", "Status",
}

// responseCharsPerLine approximates how many characters one line of a
// 28-column-wide wrapped response cell carries.
const responseCharsPerLine = 24

func (b *builder) buildGrading() {
	sheet := sheetGrading
	n := len(b.data.Questions)
	idCol := b.gradeIDCol

	for i, h := range gradingHeaders {
		b.setStr(sheet, columnName(i+1)+"1", h)
	}
	for i := 0; i < n; i++ {
		soalRow := 2 + i
		b.setFormula(sheet, columnName(7+i)+"1", fmt.Sprintf(`"S"&Soal!$A$%d`, soalRow))
		b.setFormula(sheet, columnName(7+n+i)+"1", fmt.Sprintf(`"N"&Soal!$A$%d`, soalRow))
	}
	b.setStr(sheet, b.finalCol+"1", "Skor akhir")
	b.setStr(sheet, idCol+"1", "ID")
	b.styleRange(sheet, "A1", idCol+"1", b.st.header)
	b.rowHeight(sheet, 1, 26)

	b.styleRange(sheet, "A2", fmt.Sprintf("%s%d", idCol, b.aLast), b.st.body)

	statusRef := fmt.Sprintf("F2:F%d", b.aLast)
	answers := make([]string, 0, n)

	for i, at := range b.data.Attempts {
		row := 2 + i
		b.setNumber(sheet, fmt.Sprintf("A%d", row), i+1)
		b.setNumber(sheet, fmt.Sprintf("%s%d", idCol, row), at.RegistrationID)
		b.setFormula(sheet, fmt.Sprintf("B%d", row), fmt.Sprintf(
			`IF($%s%d="","",INDEX(Peserta!$B$2:$B$%d,MATCH($%s%d,Peserta!$J$2:$J$%d,0)))`,
			idCol, row, b.pLast, idCol, row, b.pLast))
		b.setFormula(sheet, fmt.Sprintf("C%d", row), fmt.Sprintf(
			`IF($%s%d="","",INDEX(Peserta!$D$2:$D$%d,MATCH($%s%d,Peserta!$J$2:$J$%d,0)))`,
			idCol, row, b.pLast, idCol, row, b.pLast))
		b.setNumber(sheet, fmt.Sprintf("D%d", row), at.Number)
		b.setDate(sheet, fmt.Sprintf("E%d", row), at.SubmittedAt)
		b.setFormula(sheet, fmt.Sprintf("F%d", row), fmt.Sprintf(
			`IF(%s%d="","Menunggu","Selesai")`, b.finalCol, row))

		answers = answers[:0]
		for qOut := 0; qOut < n; qOut++ {
			ans := answerAt(at, b.qOrder[qOut])
			text := "—"
			if ans != nil && strings.TrimSpace(ans.Text) != "" {
				text = ans.Text
			}
			answers = append(answers, text)
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
		b.styleCell(sheet, fmt.Sprintf("C%d", row), b.st.center)
		b.styleCell(sheet, fmt.Sprintf("D%d", row), b.st.integer)
		b.styleCell(sheet, fmt.Sprintf("E%d", row), b.st.date)
		b.styleCell(sheet, fmt.Sprintf("F%d", row), b.st.center)
		b.styleCell(sheet, b.finalCol+fmt.Sprintf("%d", row), b.st.final)
		b.styleCell(sheet, fmt.Sprintf("%s%d", idCol, row), b.st.body)
		for qOut := 0; qOut < n; qOut++ {
			b.styleCell(sheet, columnName(7+n+qOut)+fmt.Sprintf("%d", row), b.st.score)
		}
		b.rowHeight(sheet, row, attemptRowHeight(answers))
	}

	b.gridlines(sheet)
	b.width(sheet, "A", "A", 6)
	b.width(sheet, "B", "B", 30)
	b.width(sheet, "C", "C", 12)
	b.width(sheet, "D", "D", 12)
	b.width(sheet, "E", "E", 18)
	b.width(sheet, "F", "F", 12)
	for i := 0; i < n; i++ {
		b.width(sheet, columnName(7+i), columnName(7+i), 28)
		b.width(sheet, columnName(7+n+i), columnName(7+n+i), 8)
	}
	b.width(sheet, b.finalCol, b.finalCol, 12)
	b.hideCols(sheet, idCol+":"+idCol)

	b.freeze(sheet, "G2", 6, 1, "bottomRight")
	b.autoFilter(sheet, fmt.Sprintf("A1:%s%d", b.finalCol, b.aLast))
	// One range, one conditionalFormatting element: the best-attempt highlight
	// must precede the banding, because equal-priority fills resolve in rule
	// order and the highlight would otherwise lose on shaded rows.
	gradingFormats := []excelize.ConditionalFormatOptions{{
		Type: "formula",
		Criteria: fmt.Sprintf(
			`AND($%s2<>"",$%s2=IFERROR(_xlfn.AGGREGATE(14,6,$%s$2:$%s$%d/($%s$2:$%s$%d=$%s2),1),""))`,
			b.finalCol, b.finalCol, b.finalCol, b.finalCol, b.aLast, idCol, idCol, b.aLast, idCol),
		Format: &b.st.best,
	}, {
		Type:     "formula",
		Criteria: `MOD(ROW(),2)=1`,
		Format:   &b.st.band,
	}}
	b.fail(b.f.SetConditionalFormat(sheet, fmt.Sprintf("A2:%s%d", b.finalCol, b.aLast), gradingFormats))
	b.statusFonts(sheet, statusRef, "F2", []statusFormat{
		{"Menunggu", b.st.warn},
		{"Selesai", b.st.done},
	})

	note := "Baris dengan skor akhir terbaik setiap peserta disorot; jika ada nilai seri, semua barisnya disorot."
	b.setStr(sheet, fmt.Sprintf("A%d", b.aLast+2), note)
	b.styleCell(sheet, fmt.Sprintf("A%d", b.aLast+2), b.st.note)
}

// attemptRowHeight sizes one grading row from its longest wrapped response.
// Long essays otherwise clip behind the fixed row height. The estimate stays
// between the 24pt minimum and a 120pt cap so outliers remain navigable.
func attemptRowHeight(answers []string) float64 {
	lines := 1
	for _, text := range answers {
		if count := wrappedLines(text); count > lines {
			lines = count
		}
	}
	height := 15.0 * float64(lines)
	if height < 24 {
		height = 24
	}
	if height > 120 {
		height = 120
	}
	return height
}

func wrappedLines(text string) int {
	count := 0
	for _, part := range strings.Split(text, "\n") {
		line := utf8.RuneCountInString(part)
		if line == 0 {
			continue
		}
		count += (line + responseCharsPerLine - 1) / responseCharsPerLine
	}
	if count == 0 {
		return 1
	}
	return count
}

// finalScoreFormula reproduces FinalScoreUnits + DisplayPercent with integer
// arithmetic in Excel: recover the stored units with ROUND(x*100,0), apply the
// two FLOORs, and leave the cell blank when any score is still missing.
func finalScoreFormula(scores string) string {
	return fmt.Sprintf(
		`IF(OR(Ringkasan!$B$9=0,COUNT(%s)<Ringkasan!$B$9),"",FLOOR((FLOOR((2*SUMPRODUCT(ROUND(%s*100,0))+Ringkasan!$B$9)/(2*Ringkasan!$B$9),1)+50)/100,1))`,
		scores, scores)
}
