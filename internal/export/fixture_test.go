package export

// fixture_test.go builds the read-model snapshots the workbook tests render. It
// never touches the database: the builder's contract starts at attempts.ExportData.

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"quiz-v2/internal/attempts"
	"quiz-v2/internal/viewmodel"
)

func utcTime(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

func intPtr(n int) *int { return &n }

func timePtr(t time.Time) *time.Time { return &t }

// sampleExportData is three participants, two questions, and three submitted
// attempts: Ayu has one graded attempt and one waiting for her essay; Budi has a
// graded attempt; Citra registered but never submitted.
func sampleExportData() attempts.ExportData {
	created := utcTime(2026, 10, 20, 2, 0) // 09:00 WIB
	published := utcTime(2026, 10, 20, 3, 0)
	return attempts.ExportData{
		Quiz: attempts.ExportQuiz{
			ID:          7,
			Title:       "Ulangan Harian Bab 3",
			OwnerName:   "Bu Guru",
			State:       viewmodel.QuizOpen,
			CreatedAt:   created,
			PublishedAt: &published,
			MaxAttempts: 3,
		},
		Questions: []attempts.ExportQuestion{
			{
				ID:       101,
				Position: 1,
				Type:     viewmodel.QuestionSingle,
				Prompt:   "Ibu kota Indonesia?",
				Options: []attempts.ExportOption{
					{ID: 1001, Position: 1, Body: "Bandung"},
					{ID: 1002, Position: 2, Body: "Jakarta", Correct: true},
					{ID: 1003, Position: 3, Body: "Surabaya"},
				},
			},
			{
				ID:       102,
				Position: 2,
				Type:     viewmodel.QuestionEssay,
				Prompt:   "Jelaskan prosesnya.",
			},
		},
		Participants: []attempts.ExportParticipant{
			{RegistrationID: 201, Name: "Ayu Lestari", Grade: 12, Major: "IPA",
				CreatedAt: utcTime(2026, 10, 21, 1, 0)}, // 08:00 WIB
			{RegistrationID: 202, Name: "Budi Santoso", Grade: 11, Major: "RPL",
				CreatedAt: utcTime(2026, 10, 21, 2, 0)},
			{RegistrationID: 203, Name: "Citra Anonim", Anonymous: true, Grade: 12,
				CreatedAt: utcTime(2026, 10, 21, 3, 0)},
		},
		Attempts: []attempts.ExportAttempt{
			{
				ID:             301,
				RegistrationID: 201,
				Number:         1,
				SubmittedAt:    utcTime(2026, 10, 22, 1, 0), // 08:00 WIB
				FinalUnits:     intPtr(8450),
				Answers: []attempts.ExportAnswer{
					{QuestionID: 101, Text: "B. Jakarta", ScoreUnits: intPtr(8449)},
					{QuestionID: 102, Text: "Jawaban esai Ayu", ScoreUnits: intPtr(8450)},
				},
			},
			{
				ID:             302,
				RegistrationID: 201,
				Number:         2,
				SubmittedAt:    utcTime(2026, 10, 22, 2, 0),
				Answers: []attempts.ExportAnswer{
					{QuestionID: 101, Text: "B. Jakarta", ScoreUnits: intPtr(10000)},
					{QuestionID: 102, Text: "—"},
				},
			},
			{
				ID:             303,
				RegistrationID: 202,
				Number:         1,
				SubmittedAt:    utcTime(2026, 10, 22, 3, 0),
				FinalUnits:     intPtr(2500), // FinalScoreUnits({0, 5000}, 2)
				Answers: []attempts.ExportAnswer{
					{QuestionID: 101, Text: "A. Bandung", ScoreUnits: intPtr(0)},
					{QuestionID: 102, Text: "Jawaban esai Budi", ScoreUnits: intPtr(5000)},
				},
			},
		},
	}
}

func buildAndOpen(t *testing.T, data attempts.ExportData, uploadsDir string) (*excelize.File, []byte) {
	t.Helper()
	workbook, err := Build(data, uploadsDir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(workbook) == 0 {
		t.Fatal("Build returned an empty workbook")
	}
	return openWorkbook(t, workbook), workbook
}

func openWorkbook(t *testing.T, workbook []byte) *excelize.File {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(workbook))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func cell(t *testing.T, f *excelize.File, sheet, ref string) string {
	t.Helper()
	value, err := f.GetCellValue(sheet, ref)
	if err != nil {
		t.Fatalf("GetCellValue(%s!%s): %v", sheet, ref, err)
	}
	return value
}

func formula(t *testing.T, f *excelize.File, sheet, ref string) string {
	t.Helper()
	value, err := f.GetCellFormula(sheet, ref)
	if err != nil {
		t.Fatalf("GetCellFormula(%s!%s): %v", sheet, ref, err)
	}
	return value
}

func wantFormula(t *testing.T, f *excelize.File, sheet, ref, want string) {
	t.Helper()
	if got := formula(t, f, sheet, ref); got != want {
		t.Errorf("%s!%s formula = %q, want %q", sheet, ref, got, want)
	}
}

func wantCell(t *testing.T, f *excelize.File, sheet, ref, want string) {
	t.Helper()
	if got := cell(t, f, sheet, ref); got != want {
		t.Errorf("%s!%s = %q, want %q", sheet, ref, got, want)
	}
}

// workbookZipContains reports whether any XML part in the workbook contains the
// needle. It is used for the autoFilter reference, for which excelize exposes no
// reader, while still proving the filter reached the saved package.
func workbookZipContains(t *testing.T, workbook []byte, needle string) bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(workbook), int64(len(workbook)))
	if err != nil {
		t.Fatalf("open workbook zip: %v", err)
	}
	for _, part := range zr.File {
		if !strings.HasSuffix(part.Name, ".xml") {
			continue
		}
		rc, err := part.Open()
		if err != nil {
			t.Fatalf("open %s: %v", part.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", part.Name, err)
		}
		if bytes.Contains(data, []byte(needle)) {
			return true
		}
	}
	return false
}
