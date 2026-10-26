package export

import (
	"fmt"

	"quiz-v2/internal/viewmodel"
)

// participantIDCol is the hidden trailing helper column that carries the
// registration id the cross-sheet lookups match on (layout-polish spec §1.2).
const participantIDCol = "J"

var participantHeaders = []string{
	"No", "Nama", "Jenis", "Kelas", "Jurusan", "Terdaftar", "Percobaan", "Rata-rata", "Status nilai",
}

func (b *builder) buildParticipants() {
	sheet := sheetParticipants
	for i, h := range participantHeaders {
		b.setStr(sheet, columnName(i+1)+"1", h)
	}
	b.setStr(sheet, participantIDCol+"1", "ID")
	b.styleRange(sheet, "A1", participantIDCol+"1", b.st.header)
	b.rowHeight(sheet, 1, 26)

	participants := sortedParticipants(b.data.Participants)
	b.styleRange(sheet, "A2", fmt.Sprintf("%s%d", participantIDCol, b.pLast), b.st.body)

	participantBand := fmt.Sprintf("A2:I%d", b.pLast)
	statusRef := fmt.Sprintf("I2:I%d", b.pLast)

	for i, p := range participants {
		row := 2 + i
		b.setNumber(sheet, fmt.Sprintf("A%d", row), i+1)
		b.setNumber(sheet, fmt.Sprintf("%s%d", participantIDCol, row), p.RegistrationID)
		b.setStr(sheet, fmt.Sprintf("B%d", row), p.Name)
		kind := "Siswa"
		if p.Anonymous {
			kind = "Anonim"
		}
		b.setStr(sheet, fmt.Sprintf("C%d", row), kind)
		b.setStr(sheet, fmt.Sprintf("D%d", row), viewmodel.ClassLabel(p.Grade, p.Major))
		major := p.Major
		if major == "" {
			major = "—"
		}
		b.setStr(sheet, fmt.Sprintf("E%d", row), major)
		b.setDate(sheet, fmt.Sprintf("F%d", row), p.CreatedAt)
		b.setFormula(sheet, fmt.Sprintf("G%d", row), fmt.Sprintf(
			`COUNTIFS(Penilaian!$%s$2:$%s$%d,$J%d)`, b.gradeIDCol, b.gradeIDCol, b.aLast, row))
		b.setFormula(sheet, fmt.Sprintf("H%d", row), fmt.Sprintf(
			`IFERROR(AVERAGEIFS(Penilaian!$%s$2:$%s$%d,Penilaian!$%s$2:$%s$%d,$J%d),"")`,
			b.finalCol, b.finalCol, b.aLast, b.gradeIDCol, b.gradeIDCol, b.aLast, row))
		b.setFormula(sheet, fmt.Sprintf("I%d", row), fmt.Sprintf(
			`IF($G%d=0,"Belum ada nilai",IF($H%d="","Menunggu","Sudah dinilai"))`, row, row))

		b.styleCell(sheet, fmt.Sprintf("A%d", row), b.st.integer)
		b.styleCell(sheet, fmt.Sprintf("C%d", row), b.st.center)
		b.styleCell(sheet, fmt.Sprintf("D%d", row), b.st.center)
		b.styleCell(sheet, fmt.Sprintf("F%d", row), b.st.date)
		b.styleCell(sheet, fmt.Sprintf("G%d", row), b.st.integer)
		b.styleCell(sheet, fmt.Sprintf("H%d", row), b.st.average)
		b.styleCell(sheet, fmt.Sprintf("I%d", row), b.st.center)
		b.rowHeight(sheet, row, 24)
	}

	b.gridlines(sheet)
	b.width(sheet, "A", "A", 6)
	b.width(sheet, "B", "B", 32)
	b.width(sheet, "C", "C", 10)
	b.width(sheet, "D", "D", 12)
	b.width(sheet, "E", "E", 14)
	b.width(sheet, "F", "F", 18)
	b.width(sheet, "G", "G", 12)
	b.width(sheet, "H", "H", 12)
	b.width(sheet, "I", "I", 16)
	b.hideCols(sheet, participantIDCol+":"+participantIDCol)
	b.freeze(sheet, "A2", 0, 1, "bottomLeft")
	b.autoFilter(sheet, fmt.Sprintf("A1:I%d", b.pLast))
	b.zebra(sheet, participantBand)
	b.statusFonts(sheet, statusRef, "I2", []statusFormat{
		{"Belum ada nilai", b.st.idle},
		{"Menunggu", b.st.warn},
		{"Sudah dinilai", b.st.done},
	})
}
