package export

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"quiz-v2/internal/attempts"
	"quiz-v2/internal/viewmodel"
)

// TestEmptyQuizStillBuildsWorkbook covers design §4.2 and §11.3: a quiz with no
// questions, registrations, or attempts is a valid four-sheet file whose ranges
// all point at the one reserved blank row.
func TestEmptyQuizStillBuildsWorkbook(t *testing.T) {
	data := attempts.ExportData{
		Quiz: attempts.ExportQuiz{
			ID: 9, Title: "Kuis Kosong", OwnerName: "Bu Guru", State: viewmodel.QuizOpen,
			CreatedAt: utcTime(2026, 10, 20, 2, 0), MaxAttempts: 1,
		},
	}
	f, _ := buildAndOpen(t, data, "")

	if got := f.GetSheetList(); len(got) != 4 {
		t.Fatalf("sheets = %v, want four", got)
	}
	wantCell(t, f, "Peserta", "A2", "")
	wantCell(t, f, "Soal", "A2", "")
	wantCell(t, f, "Penilaian", "A2", "")
	wantCell(t, f, "Ringkasan", "A19", "")
	wantCell(t, f, "Ringkasan", "B9", "")
	wantFormula(t, f, "Penilaian", "G1", "")
	wantCell(t, f, "Penilaian", "G1", "Skor akhir")

	for cellRef, want := range map[string]string{
		"B9": "0", "B12": "0", "B13": "0", "B14": "0", "B15": "0", "B16": "",
	} {
		got, err := f.CalcCellValue("Ringkasan", cellRef)
		if err != nil {
			t.Fatalf("CalcCellValue(%s): %v", cellRef, err)
		}
		if got != want {
			t.Errorf("Ringkasan!%s = %q, want %q", cellRef, got, want)
		}
	}

	props, err := f.GetCalcProps()
	if err != nil {
		t.Fatalf("GetCalcProps: %v", err)
	}
	if props.CalcMode == nil || *props.CalcMode != "auto" {
		t.Errorf("CalcMode = %v, want auto", props.CalcMode)
	}
	if props.FullCalcOnLoad == nil || !*props.FullCalcOnLoad {
		t.Errorf("FullCalcOnLoad = %v, want true", props.FullCalcOnLoad)
	}
}

// TestClassLabelsAndWIBDates covers design §4.2 and §11.8. The fixture supplies
// UTC instants, so a passing display proves both the WIB conversion and the
// number-format style.
func TestClassLabelsAndWIBDates(t *testing.T) {
	f, _ := buildAndOpen(t, sampleExportData(), "")

	wantCell(t, f, "Peserta", "D2", "XII IPA")
	wantCell(t, f, "Peserta", "D3", "XI RPL")
	wantCell(t, f, "Peserta", "D4", "XII")
	wantCell(t, f, "Peserta", "E2", "IPA")
	wantCell(t, f, "Peserta", "E4", "—")

	wantCell(t, f, "Peserta", "F2", "2026-10-21 08:00")
	wantCell(t, f, "Peserta", "F3", "2026-10-21 09:00")
	wantCell(t, f, "Penilaian", "E2", "2026-10-22 08:00")
	wantCell(t, f, "Ringkasan", "B6", "2026-10-20 09:00")
	wantCell(t, f, "Ringkasan", "B7", "2026-10-20 10:00")
	wantCell(t, f, "Ringkasan", "B8", "—")

	raw, err := f.GetCellValue("Peserta", "F2", excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatalf("raw date: %v", err)
	}
	value := mustFloat(t, raw)
	if value < 40000 {
		t.Errorf("Peserta!F2 raw value = %s, want an Excel date serial", raw)
	}

	styleID, err := f.GetCellStyle("Peserta", "F2")
	if err != nil {
		t.Fatalf("GetCellStyle: %v", err)
	}
	style, err := f.GetStyle(styleID)
	if err != nil {
		t.Fatalf("GetStyle: %v", err)
	}
	if style.CustomNumFmt == nil || *style.CustomNumFmt != "yyyy-mm-dd hh:mm" {
		t.Errorf("date format = %v, want yyyy-mm-dd hh:mm", style.CustomNumFmt)
	}
}

// TestUserStringsAreLiteral covers design §10 and §11.10: a value that starts
// with a formula sigil is still a string cell, never a formula.
func TestUserStringsAreLiteral(t *testing.T) {
	data := sampleExportData()
	data.Quiz.Title = "=1+1"
	data.Quiz.OwnerName = "+cmd"
	data.Participants[0].Name = `=HYPERLINK("http://example.test","klik")`
	data.Participants[1].Name = "@Budi"
	data.Questions[0].Prompt = "@SUM(1,2)"
	data.Questions[0].Options[0].Body = "-2+3"
	data.Attempts[0].Answers[1].Text = "=1+1"

	f, _ := buildAndOpen(t, data, "")

	for _, tc := range []struct {
		sheet, ref, want string
	}{
		{"Ringkasan", "B3", "=1+1"},
		{"Ringkasan", "B4", "+cmd"},
		{"Peserta", "B2", `=HYPERLINK("http://example.test","klik")`},
		{"Peserta", "B3", "@Budi"},
		{"Soal", "C2", "@SUM(1,2)"},
		{"Soal", "E2", "A. -2+3\nB. Jakarta\nC. Surabaya"},
		{"Penilaian", "H2", "=1+1"},
	} {
		if got := formula(t, f, tc.sheet, tc.ref); got != "" {
			t.Errorf("%s!%s is a formula %q, want a literal string", tc.sheet, tc.ref, got)
		}
		wantCell(t, f, tc.sheet, tc.ref, tc.want)
	}
}

// TestSanitizeCell covers the cell hygiene rules in design §10: drop the bytes
// Excel rejects, preserve line breaks, and cap the UTF-16 length with a visible
// truncation marker.
func TestSanitizeCell(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"control bytes", "a\x00b\x01c\x0bd\x0ce\x1ff", "abcdef"},
		{"line breaks", "a\tb\nc\rd", "a\tb\nc\rd"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeCell(tc.in); got != tc.want {
				t.Errorf("sanitizeCell(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	long := sanitizeCell(strings.Repeat("a", maxCellUTF16+10))
	if got := len([]rune(long)); got != maxCellUTF16 {
		t.Errorf("long cell runes = %d, want %d", got, maxCellUTF16)
	}
	if !strings.HasSuffix(long, "…") {
		t.Error("truncated cell does not end with an ellipsis")
	}

	emoji := sanitizeCell(strings.Repeat("😀", maxCellUTF16/2+10))
	if !utf8.ValidString(emoji) {
		t.Error("truncated emoji cell is not valid UTF-8")
	}
	if !strings.HasSuffix(emoji, "…") {
		t.Error("truncated emoji cell does not end with an ellipsis")
	}
}

// TestBestAttemptConditionalFormat covers design §6.4 and §11.9. The formula is
// applied to every row of the grading table and groups by participant id, so a
// tie leaves both equal best rows true.
func TestBestAttemptConditionalFormat(t *testing.T) {
	data := sampleExportData()
	data.Attempts = append(data.Attempts, attempts.ExportAttempt{
		ID: 304, RegistrationID: 202, Number: 2, SubmittedAt: utcTime(2026, 10, 22, 4, 0),
		FinalUnits: intPtr(2500), // FinalScoreUnits({0, 5000}, 2)
		Answers: []attempts.ExportAnswer{
			{QuestionID: 101, Text: "A. Bandung", ScoreUnits: intPtr(0)},
			{QuestionID: 102, Text: "Jawaban esai Budi", ScoreUnits: intPtr(5000)},
		},
	})
	f, _ := buildAndOpen(t, data, "")

	formats, err := f.GetConditionalFormats("Penilaian")
	if err != nil {
		t.Fatalf("GetConditionalFormats: %v", err)
	}
	opts, ok := formats["A2:K5"]
	if !ok {
		t.Fatalf("conditional formats = %v, want range A2:K5", formats)
	}
	if len(opts) != 1 {
		t.Fatalf("conditional format rules = %d, want 1", len(opts))
	}
	want := `AND($K2<>"",$K2=IFERROR(_xlfn.AGGREGATE(14,6,$K$2:$K$5/($A$2:$A$5=$A2),1),""))`
	if opts[0].Type != "formula" || opts[0].Criteria != want {
		t.Errorf("rule = %+v, want formula %q", opts[0], want)
	}
	if opts[0].Format == nil {
		t.Error("best-attempt rule has no fill format")
	}
}
