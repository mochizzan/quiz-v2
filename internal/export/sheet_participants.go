package export

import (
	"fmt"

	"quiz-v2/internal/viewmodel"
)

var participantHeaders = []string{
	"ID", "Nama", "Jenis", "Kelas", "Jurusan", "Terdaftar", "Percobaan", "Rata-rata", "Status nilai",
}

func (b *builder) buildParticipants() {
	sheet := sheetParticipants
	for i, h := range participantHeaders {
		b.setStr(sheet, columnName(i+1)+"1", h)
	}
	b.styleRange(sheet, "A1", "I1", b.st.header)
	b.rowHeight(sheet, 1, 24)

	participants := sortedParticipants(b.data.Participants)
	b.styleRange(sheet, "A2", fmt.Sprintf("I%d", b.pLast), b.st.body)

	for i, p := range participants {
		row := 2 + i
		b.setNumber(sheet, fmt.Sprintf("A%d", row), p.RegistrationID)
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
			`COUNTIFS(Penilaian!$A$2:$A$%d,$A%d)`, b.aLast, row))
		b.setFormula(sheet, fmt.Sprintf("H%d", row), fmt.Sprintf(
			`IFERROR(AVERAGEIFS(Penilaian!$%s$2:$%s$%d,Penilaian!$A$2:$A$%d,$A%d),"")`,
			b.finalCol, b.finalCol, b.aLast, b.aLast, row))
		b.setFormula(sheet, fmt.Sprintf("I%d", row), fmt.Sprintf(
			`IF($G%d=0,"Belum ada nilai",IF($H%d="","Menunggu","Sudah dinilai"))`, row, row))

		b.styleCell(sheet, fmt.Sprintf("A%d", row), b.st.integer)
		b.styleCell(sheet, fmt.Sprintf("F%d", row), b.st.date)
		b.styleCell(sheet, fmt.Sprintf("G%d", row), b.st.integer)
		b.styleCell(sheet, fmt.Sprintf("H%d", row), b.st.average)
		b.rowHeight(sheet, row, 24)
	}

	b.width(sheet, "A", "A", 10)
	b.width(sheet, "B", "B", 30)
	b.width(sheet, "C", "C", 10)
	b.width(sheet, "D", "D", 14)
	b.width(sheet, "E", "E", 16)
	b.width(sheet, "F", "F", 18)
	b.width(sheet, "G", "G", 12)
	b.width(sheet, "H", "H", 12)
	b.width(sheet, "I", "I", 16)
	b.freeze(sheet, "A2", 0, 1, "bottomLeft")
}
