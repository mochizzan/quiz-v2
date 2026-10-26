package export

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"quiz-v2/internal/attempts"
	"quiz-v2/internal/viewmodel"
)

// TestWorkbookStructure pins the four sheets, their exact headers, the reserved
// blank row, the freeze panes, and the two AutoFilters from design §6 and §11.3.
func TestWorkbookStructure(t *testing.T) {
	f, workbook := buildAndOpen(t, sampleExportData(), "")

	wantSheets := []string{"Ringkasan", "Peserta", "Soal", "Penilaian"}
	if got := f.GetSheetList(); len(got) != len(wantSheets) {
		t.Fatalf("sheets = %v, want exactly %v", got, wantSheets)
	} else {
		for i := range wantSheets {
			if got[i] != wantSheets[i] {
				t.Errorf("sheet %d = %q, want %q", i, got[i], wantSheets[i])
			}
		}
	}

	for ref, want := range map[string]string{
		"Peserta!A1": "No", "Peserta!B1": "Nama", "Peserta!C1": "Jenis",
		"Peserta!D1": "Kelas", "Peserta!E1": "Jurusan", "Peserta!F1": "Terdaftar",
		"Peserta!G1": "Percobaan", "Peserta!H1": "Rata-rata", "Peserta!I1": "Status nilai",
		"Peserta!J1": "ID",
		"Soal!A1":    "No", "Soal!B1": "Tipe", "Soal!C1": "Soal", "Soal!D1": "Gambar",
		"Soal!E1": "Pilihan", "Soal!F1": "Kunci",
		"Penilaian!A1": "No", "Penilaian!B1": "Nama", "Penilaian!C1": "Kelas",
		"Penilaian!D1": "Percobaan", "Penilaian!E1": "Dikirim", "Penilaian!F1": "Status",
		"Penilaian!K1": "Skor akhir", "Penilaian!L1": "ID",
	} {
		parts := strings.SplitN(ref, "!", 2)
		wantCell(t, f, parts[0], parts[1], want)
	}

	// Data rows, plus the one blank row every zero-count range reserves.
	wantCell(t, f, "Peserta", "A2", "1")
	wantCell(t, f, "Peserta", "A4", "3")
	wantCell(t, f, "Peserta", "A5", "")
	wantCell(t, f, "Peserta", "J2", "201")
	wantCell(t, f, "Peserta", "J4", "203")
	wantCell(t, f, "Soal", "A2", "1")
	wantCell(t, f, "Soal", "A3", "2")
	wantCell(t, f, "Soal", "A4", "")
	wantCell(t, f, "Penilaian", "A2", "1")
	wantCell(t, f, "Penilaian", "A4", "3")
	wantCell(t, f, "Penilaian", "A5", "")
	wantCell(t, f, "Penilaian", "L2", "201")
	wantCell(t, f, "Penilaian", "L4", "202")
	wantCell(t, f, "Ringkasan", "A18", "Peringkat")
	wantCell(t, f, "Ringkasan", "F18", "Kunci")
	wantCell(t, f, "Ringkasan", "A19", "1")
	wantCell(t, f, "Ringkasan", "A20", "2")
	wantCell(t, f, "Ringkasan", "A21", "")

	// Soal is canonical: option letters follow the stored positions even though
	// the read model slice is not necessarily sorted.
	wantCell(t, f, "Soal", "E2", "A. Bandung\nB. Jakarta\nC. Surabaya")
	wantCell(t, f, "Soal", "F2", "B")
	wantCell(t, f, "Soal", "E3", "")
	wantCell(t, f, "Soal", "F3", "—")

	if got := cell(t, f, "Peserta", "D2"); got != "XII IPA" {
		t.Errorf("Peserta!D2 = %q, want the combined class label", got)
	}
	if got := cell(t, f, "Peserta", "D4"); got != "XII" {
		t.Errorf("Peserta!D4 = %q, want the grade-only label", got)
	}
	if got := cell(t, f, "Peserta", "E4"); got != "—" {
		t.Errorf("Peserta!E4 = %q, want — for a missing major", got)
	}

	for _, tc := range []struct {
		sheet string
		want  excelize.Panes
	}{
		{"Peserta", excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}},
		{"Soal", excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}},
		{"Penilaian", excelize.Panes{Freeze: true, XSplit: 6, YSplit: 1, TopLeftCell: "G2", ActivePane: "bottomRight"}},
	} {
		got, err := f.GetPanes(tc.sheet)
		if err != nil {
			t.Fatalf("GetPanes(%s): %v", tc.sheet, err)
		}
		if !got.Freeze || got.XSplit != tc.want.XSplit || got.YSplit != tc.want.YSplit ||
			got.TopLeftCell != tc.want.TopLeftCell || got.ActivePane != tc.want.ActivePane {
			t.Errorf("%s panes = %+v, want %+v", tc.sheet, got, tc.want)
		}
	}

	for _, ref := range []string{`autoFilter ref="$A$18:$E$20"`, `autoFilter ref="$A$1:$I$4"`, `autoFilter ref="$A$1:$K$4"`} {
		if !workbookZipContains(t, workbook, ref) {
			t.Errorf("workbook has no %s", ref)
		}
	}

	for _, tc := range []struct {
		sheet, col string
	}{
		{"Ringkasan", "F"}, {"Ringkasan", "J"}, {"Ringkasan", "K"},
		{"Ringkasan", "L"}, {"Ringkasan", "M"}, {"Ringkasan", "N"}, {"Ringkasan", "O"},
		{"Peserta", "J"},
		{"Penilaian", "L"},
	} {
		visible, err := f.GetColVisible(tc.sheet, tc.col)
		if err != nil {
			t.Fatalf("GetColVisible(%s, %s): %v", tc.sheet, tc.col, err)
		}
		if visible {
			t.Errorf("%s column %s is visible, want hidden", tc.sheet, tc.col)
		}
	}
	if visible, _ := f.GetColVisible("Ringkasan", "A"); !visible {
		t.Error("Ringkasan column A is hidden, want visible")
	}
	if visible, _ := f.GetColVisible("Peserta", "A"); !visible {
		t.Error("Peserta column A is hidden, want visible")
	}
}

// TestWorkbookFormulas pins the cross-sheet links and the score contract. The
// exact strings are the document the workbook hands to Excel; changing one is a
// workbook-layout change and needs its own design.
func TestWorkbookFormulas(t *testing.T) {
	f, _ := buildAndOpen(t, sampleExportData(), "")

	wantFormula(t, f, "Ringkasan", "B9", `COUNTA(Soal!$A$2:$A$3)`)
	wantFormula(t, f, "Ringkasan", "B12", `COUNTA(Peserta!$A$2:$A$4)`)
	wantFormula(t, f, "Ringkasan", "B13", `COUNTIF(Peserta!$G$2:$G$4,">0")`)
	wantFormula(t, f, "Ringkasan", "B14", `B12-B13`)
	wantFormula(t, f, "Ringkasan", "B15", `IFERROR(SUM($N$2:$N$3),"")`)
	wantFormula(t, f, "Ringkasan", "B16", `IFERROR(AVERAGE($N$2:$N$3),"")`)

	wantFormula(t, f, "Peserta", "G2", `COUNTIFS(Penilaian!$L$2:$L$4,$J2)`)
	wantFormula(t, f, "Peserta", "H2",
		`IFERROR(AVERAGEIFS(Penilaian!$K$2:$K$4,Penilaian!$L$2:$L$4,$J2),"")`)
	wantFormula(t, f, "Peserta", "I2",
		`IF($G2=0,"Belum ada nilai",IF($H2="","Menunggu","Sudah dinilai"))`)

	// The hidden helper reads Peserta only; the rank key is the mean minus a
	// row term small enough to leave every real average distinct.
	wantFormula(t, f, "Ringkasan", "K2",
		`IF($J2="","",INDEX(Peserta!$B$2:$B$4,MATCH($J2,Peserta!$J$2:$J$4,0)))`)
	wantFormula(t, f, "Ringkasan", "L2",
		`IF($J2="","",INDEX(Peserta!$D$2:$D$4,MATCH($J2,Peserta!$J$2:$J$4,0)))`)
	wantFormula(t, f, "Ringkasan", "M2",
		`IF($J2="","",INDEX(Peserta!$G$2:$G$4,MATCH($J2,Peserta!$J$2:$J$4,0)))`)
	wantFormula(t, f, "Ringkasan", "N2",
		`IF($J2="","",INDEX(Peserta!$H$2:$H$4,MATCH($J2,Peserta!$J$2:$J$4,0)))`)
	wantFormula(t, f, "Ringkasan", "O2", `IF($N2="","",$N2-ROW()/1000000000000)`)

	wantFormula(t, f, "Ringkasan", "F19", `IFERROR(LARGE($O$2:$O$3,$A19),"")`)
	wantFormula(t, f, "Ringkasan", "B19",
		`IF($F19="","",INDEX($K$2:$K$3,MATCH($F19,$O$2:$O$3,0)))`)
	wantFormula(t, f, "Ringkasan", "C19",
		`IF($F19="","",INDEX($L$2:$L$3,MATCH($F19,$O$2:$O$3,0)))`)
	wantFormula(t, f, "Ringkasan", "D19",
		`IF($F19="","",INDEX($M$2:$M$3,MATCH($F19,$O$2:$O$3,0)))`)
	wantFormula(t, f, "Ringkasan", "E19",
		`IF($F19="","",INDEX($N$2:$N$3,MATCH($F19,$O$2:$O$3,0)))`)

	wantFormula(t, f, "Penilaian", "G1", `"S"&Soal!$A$2`)
	wantFormula(t, f, "Penilaian", "H1", `"S"&Soal!$A$3`)
	wantFormula(t, f, "Penilaian", "I1", `"N"&Soal!$A$2`)
	wantFormula(t, f, "Penilaian", "J1", `"N"&Soal!$A$3`)
	wantFormula(t, f, "Penilaian", "B2",
		`IF($L2="","",INDEX(Peserta!$B$2:$B$4,MATCH($L2,Peserta!$J$2:$J$4,0)))`)
	wantFormula(t, f, "Penilaian", "C2",
		`IF($L2="","",INDEX(Peserta!$D$2:$D$4,MATCH($L2,Peserta!$J$2:$J$4,0)))`)
	wantFormula(t, f, "Penilaian", "F2", `IF(K2="","Menunggu","Selesai")`)
	wantFormula(t, f, "Penilaian", "K2", finalScoreFormula("I2:J2"))
	wantFormula(t, f, "Penilaian", "K4", finalScoreFormula("I4:J4"))

	// Counts and participant attempt counts are simple enough for the library's
	// evaluator, so they are checked as values, not only as text.
	if got, err := f.CalcCellValue("Ringkasan", "B9"); err != nil || got != "2" {
		t.Errorf("Jumlah soal = %q, %v; want 2", got, err)
	}
	if got, err := f.CalcCellValue("Ringkasan", "B12"); err != nil || got != "3" {
		t.Errorf("Total terdaftar = %q, %v; want 3", got, err)
	}
	if got, err := f.CalcCellValue("Ringkasan", "B13"); err != nil || got != "2" {
		t.Errorf("Total selesai = %q, %v; want 2", got, err)
	}
	if got, err := f.CalcCellValue("Ringkasan", "B14"); err != nil || got != "1" {
		t.Errorf("Belum selesai = %q, %v; want 1", got, err)
	}
}

// TestScoreFidelitySingleQuestion executes the real final-score formula with one
// score cell. The library evaluates a scalar ROUND correctly; it cannot
// vectorize the two-cell form, so the witness below is checked arithmetically.
func TestScoreFidelitySingleQuestion(t *testing.T) {
	for _, units := range []int{0, 50, 8449, 8450, 9999, 10000} {
		t.Run("score_"+itoa(units), func(t *testing.T) {
			data := oneQuestionData(units)
			f, _ := buildAndOpen(t, data, "")
			got, err := f.CalcCellValue("Penilaian", "I2")
			if err != nil {
				t.Fatalf("CalcCellValue final: %v", err)
			}
			wantUnits, ok := attempts.FinalScoreUnits([]int{units}, 1)
			if !ok {
				t.Fatalf("FinalScoreUnits rejected %d", units)
			}
			want := attempts.DisplayPercent(wantUnits)
			if got != itoa(want) {
				t.Errorf("final = %q, want %d (units %d)", got, want, wantUnits)
			}
			wantFormula(t, f, "Penilaian", "I2", finalScoreFormula("H2:H2"))
		})
	}
}

// TestScoreFidelityTwoScores pins the witness from design §7.1: the app path is
// 85 while a plain ROUND(AVERAGE(percent),0) is 84. The library's evaluator
// applies ROUND to only the first cell of a range expression, so this test
// applies the formula's own arithmetic to the score cells instead of asking it
// to execute the array formula.
func TestScoreFidelityTwoScores(t *testing.T) {
	cases := []struct {
		name  string
		units []int
	}{
		{"rounding witness", []int{8449, 8450}},
		{"design example", []int{10000, 6667}},
		{"zero and full", []int{0, 10000}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := twoQuestionData(tc.units)
			f, _ := buildAndOpen(t, data, "")

			wantUnits, ok := attempts.FinalScoreUnits(tc.units, len(tc.units))
			if !ok {
				t.Fatalf("FinalScoreUnits rejected %v", tc.units)
			}
			want := attempts.DisplayPercent(wantUnits)

			// Read the score cells exactly as Excel will see them.
			scores := make([]int, 0, len(tc.units))
			for i := range tc.units {
				ref := columnName(9+i) + "2"
				raw, err := f.GetCellValue("Penilaian", ref, excelize.Options{RawCellValue: true})
				if err != nil {
					t.Fatalf("read %s: %v", ref, err)
				}
				value := mustFloat(t, raw)
				scores = append(scores, int(math.Round(value*100)))
			}
			gotUnits := finalUnitsFromScores(scores)
			if got := attempts.DisplayPercent(gotUnits); got != want {
				t.Errorf("workbook formula path = %d, want %d", got, want)
			}

			// The naive path the design warns about, for the witness only.
			if tc.units[0] == 8449 && tc.units[1] == 8450 {
				plain := int(math.Round((float64(tc.units[0])/100 + float64(tc.units[1])/100) / 2))
				if plain != 84 {
					t.Errorf("plain ROUND(AVERAGE(percent),0) = %d, want the documented 84", plain)
				}
				if want != 85 {
					t.Errorf("app path = %d, want 85", want)
				}
			}
		})
	}
}

func finalUnitsFromScores(units []int) int {
	sum := 0
	for _, unit := range units {
		sum += unit
	}
	n := len(units)
	return (2*sum + n) / (2 * n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	pos := len(digits)
	for n > 0 {
		pos--
		digits[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[pos:])
}

func mustFloat(t *testing.T, raw string) float64 {
	t.Helper()
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return value
}

func oneQuestionData(units int) attempts.ExportData {
	return attempts.ExportData{
		Quiz: attempts.ExportQuiz{ID: 1, Title: "Satu soal", State: viewmodel.QuizOpen,
			CreatedAt: utcTime(2026, 10, 20, 1, 0), MaxAttempts: 1},
		Questions: []attempts.ExportQuestion{
			{ID: 1, Position: 1, Type: viewmodel.QuestionSingle, Prompt: "Soal",
				Options: []attempts.ExportOption{
					{ID: 1, Position: 1, Body: "Benar", Correct: true},
					{ID: 2, Position: 2, Body: "Salah"},
				}},
		},
		Participants: []attempts.ExportParticipant{
			{RegistrationID: 10, Name: "Ayu", Grade: 10, Major: "IPA",
				CreatedAt: utcTime(2026, 10, 21, 1, 0)},
		},
		Attempts: []attempts.ExportAttempt{
			{ID: 100, RegistrationID: 10, Number: 1, SubmittedAt: utcTime(2026, 10, 22, 1, 0),
				Answers: []attempts.ExportAnswer{
					{QuestionID: 1, Text: "A. Benar", ScoreUnits: &units},
				}},
		},
	}
}

func twoQuestionData(units []int) attempts.ExportData {
	data := oneQuestionData(units[0])
	data.Questions = append(data.Questions, attempts.ExportQuestion{
		ID: 2, Position: 2, Type: viewmodel.QuestionEssay, Prompt: "Esai",
	})
	data.Attempts[0].Answers = append(data.Attempts[0].Answers,
		attempts.ExportAnswer{QuestionID: 2, Text: "Jawaban", ScoreUnits: &units[1]})
	return data
}
