package export

import "fmt"

var summaryLabels = []string{
	"Judul", "Guru", "Status", "Dibuat", "Diterbitkan", "Ditutup", "Jumlah soal", "Maks. percobaan",
}

var metricLabels = []string{
	"Total terdaftar", "Total selesai", "Belum selesai", "Total skor", "Rata-rata skor",
}

var rankingHeaders = []string{"Peringkat", "Nama", "Kelas", "Percobaan", "Rata-rata", "Kunci"}

func (b *builder) buildSummary() {
	sheet := sheetSummary

	for i, label := range summaryLabels {
		b.setStr(sheet, fmt.Sprintf("A%d", 3+i), label)
	}
	b.styleRange(sheet, "A3", "A10", b.st.label)

	b.setStr(sheet, "B3", b.data.Quiz.Title)
	b.setStr(sheet, "B4", b.data.Quiz.OwnerName)
	b.setStr(sheet, "B5", b.data.Quiz.State.Label())
	b.setDate(sheet, "B6", b.data.Quiz.CreatedAt)
	b.setOptionalDate(sheet, "B7", b.data.Quiz.PublishedAt)
	b.setOptionalDate(sheet, "B8", b.data.Quiz.ClosedAt)
	b.setFormula(sheet, "B9", fmt.Sprintf(`COUNTA(Soal!$A$2:$A$%d)`, b.qLast))
	b.setNumber(sheet, "B10", effectiveMaxAttempts(b.data.Quiz.MaxAttempts))

	b.styleRange(sheet, "B3", "B5", b.st.body)
	b.styleRange(sheet, "B6", "B8", b.st.date)
	b.styleCell(sheet, "B9", b.st.integer)
	b.styleCell(sheet, "B10", b.st.integer)

	for i, label := range metricLabels {
		b.setStr(sheet, fmt.Sprintf("A%d", 12+i), label)
	}
	b.styleRange(sheet, "A12", "A16", b.st.label)

	b.setFormula(sheet, "B12", fmt.Sprintf(`COUNTA(Peserta!$A$2:$A$%d)`, b.pLast))
	b.setFormula(sheet, "B13", fmt.Sprintf(`COUNTIF(Peserta!$G$2:$G$%d,">0")`, b.pLast))
	b.setFormula(sheet, "B14", `B12-B13`)
	b.setFormula(sheet, "B15", fmt.Sprintf(`IFERROR(SUM($N$2:$N$%d),"")`, b.hLast))
	b.setFormula(sheet, "B16", fmt.Sprintf(`IFERROR(AVERAGE($N$2:$N$%d),"")`, b.hLast))

	b.styleRange(sheet, "B12", "B14", b.st.integer)
	b.styleCell(sheet, "B15", b.st.score)
	b.styleCell(sheet, "B16", b.st.average)

	for i, h := range rankingHeaders {
		b.setStr(sheet, columnName(i+1)+"18", h)
	}
	b.styleRange(sheet, "A18", "F18", b.st.header)
	b.rowHeight(sheet, 18, 24)

	b.styleRange(sheet, "A19", fmt.Sprintf("F%d", b.rankLast), b.st.body)
	for k := 0; k < b.h; k++ {
		row := 19 + k
		b.setNumber(sheet, fmt.Sprintf("A%d", row), k+1)
		b.setFormula(sheet, fmt.Sprintf("F%d", row), fmt.Sprintf(
			`IFERROR(LARGE($O$2:$O$%d,$A%d),"")`, b.hLast, row))
		b.setFormula(sheet, fmt.Sprintf("B%d", row), b.rankLookup("K", row))
		b.setFormula(sheet, fmt.Sprintf("C%d", row), b.rankLookup("L", row))
		b.setFormula(sheet, fmt.Sprintf("D%d", row), b.rankLookup("M", row))
		b.setFormula(sheet, fmt.Sprintf("E%d", row), b.rankLookup("N", row))

		b.styleCell(sheet, fmt.Sprintf("A%d", row), b.st.integer)
		b.styleCell(sheet, fmt.Sprintf("D%d", row), b.st.integer)
		b.styleCell(sheet, fmt.Sprintf("E%d", row), b.st.average)
		b.rowHeight(sheet, row, 20)
	}

	b.buildRankingHelper()

	b.autoFilter(sheet, fmt.Sprintf("A18:E%d", b.rankLast))
	b.width(sheet, "A", "A", 24)
	b.width(sheet, "B", "B", 44)
	b.width(sheet, "C", "C", 18)
	b.width(sheet, "D", "D", 12)
	b.width(sheet, "E", "E", 12)
	b.width(sheet, "F", "F", 12)
	b.width(sheet, "J", "O", 14)
	b.hideCols(sheet, "F:F")
	b.hideCols(sheet, "J:O")

	for row := 3; row <= 10; row++ {
		b.rowHeight(sheet, row, 20)
	}
	for row := 12; row <= 16; row++ {
		b.rowHeight(sheet, row, 20)
	}

	note := "Peserta yang masih menunggu nilai esai tampil sebagai baris kosong di bawah; peringkatnya terisi setelah nilai masuk."
	noteRow := b.rankLast + 2
	b.setStr(sheet, fmt.Sprintf("A%d", noteRow), note)
	b.styleCell(sheet, fmt.Sprintf("A%d", noteRow), b.st.note)
	b.rowHeight(sheet, noteRow, 18)
}

func (b *builder) rankLookup(helperCol string, row int) string {
	return fmt.Sprintf(
		`IF($F%d="","",INDEX($%s$2:$%s$%d,MATCH($F%d,$O$2:$O$%d,0)))`,
		row, helperCol, helperCol, b.hLast, row, b.hLast)
}

func (b *builder) buildRankingHelper() {
	sheet := sheetSummary
	participants := participantsWithAttempts(b.data.Participants, b.data.Attempts)
	b.styleRange(sheet, "J2", fmt.Sprintf("O%d", b.hLast), b.st.body)

	for i, p := range participants {
		row := 2 + i
		b.setNumber(sheet, fmt.Sprintf("J%d", row), p.RegistrationID)
		b.setFormula(sheet, fmt.Sprintf("K%d", row), fmt.Sprintf(
			`IF($J%d="","",INDEX(Peserta!$B$2:$B$%d,MATCH($J%d,Peserta!$A$2:$A$%d,0)))`,
			row, b.pLast, row, b.pLast))
		b.setFormula(sheet, fmt.Sprintf("L%d", row), fmt.Sprintf(
			`IF($J%d="","",INDEX(Peserta!$D$2:$D$%d,MATCH($J%d,Peserta!$A$2:$A$%d,0)))`,
			row, b.pLast, row, b.pLast))
		b.setFormula(sheet, fmt.Sprintf("M%d", row), fmt.Sprintf(
			`IF($J%d="","",INDEX(Peserta!$G$2:$G$%d,MATCH($J%d,Peserta!$A$2:$A$%d,0)))`,
			row, b.pLast, row, b.pLast))
		b.setFormula(sheet, fmt.Sprintf("N%d", row), fmt.Sprintf(
			`IF($J%d="","",INDEX(Peserta!$H$2:$H$%d,MATCH($J%d,Peserta!$A$2:$A$%d,0)))`,
			row, b.pLast, row, b.pLast))
		b.setFormula(sheet, fmt.Sprintf("O%d", row), fmt.Sprintf(
			`IF($N%d="","",$N%d-ROW()/1000000000000)`, row, row))
	}
}
