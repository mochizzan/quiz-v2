package httpserver_test

// routes_teacher_export_http_test.go drives the XLSX export through the whole
// HTTP pipeline: a real signed session, the route class, the read model, the
// workbook builder, and the binary response. It is the integration counterpart
// of routes_teacher_export_test.go (transport helpers) and the attempts/export
// package tests (read model and workbook internals).

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/xuri/excelize/v2"

	"quiz-v2/internal/authn"
	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
)

const exportHTTPKey = "0123456789abcdef0123456789abcdef"

func exportHTTPExec(t *testing.T, db *store.DB, query string, args ...any) int64 {
	t.Helper()
	res, err := db.Exec(query, args...)
	if err != nil {
		t.Fatalf("seed: %v\nquery: %s", err, query)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seed last id: %v", err)
	}
	return id
}

type exportHTTPWorld struct {
	app          *fiber.App
	openQuiz     int64
	draftQuiz    int64
	ownerToken   string
	foreignToken string
	studentToken string
}

func (w *exportHTTPWorld) get(t *testing.T, path, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: httpserver.CookieSession, Value: token})
	}
	resp, err := w.app.Test(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// seedExportHTTPWorld creates an owner, a foreign teacher, a student, an open
// quiz, and a draft. The open quiz has one question so the workbook is not an
// empty shell; no participant is needed to prove the transport.
func seedExportHTTPWorld(t *testing.T) *exportHTTPWorld {
	t.Helper()
	db := storetest.DB(t)
	storetest.Reset(t, db)

	ownerID := exportHTTPExec(t, db, `
		INSERT INTO accounts (full_name, email, password_hash, role, status, approved_at)
		VALUES ('Guru Pemilik', 'pemilik@export.test', 'test', 'teacher', 'active', UTC_TIMESTAMP())`)
	foreignID := exportHTTPExec(t, db, `
		INSERT INTO accounts (full_name, email, password_hash, role, status, approved_at)
		VALUES ('Guru Lain', 'lain@export.test', 'test', 'teacher', 'active', UTC_TIMESTAMP())`)
	studentAccountID := exportHTTPExec(t, db, `
		INSERT INTO accounts (full_name, email, password_hash, role, status)
		VALUES ('Siswa Export', 'siswa@export.test', 'test', 'student', 'active')`)
	majorID := exportHTTPExec(t, db, `INSERT INTO majors (name, code) VALUES ('RPL', 'RPL')`)
	exportHTTPExec(t, db, `
		INSERT INTO students (account_id, grade_level, major_id) VALUES (?, 10, ?)`,
		studentAccountID, majorID)

	openQuiz := exportHTTPExec(t, db, `
		INSERT INTO quizzes (owner_account_id, title, visibility, state, code, invite_token,
		                     published_at, show_score, allow_retake, max_attempts, review_mode)
		VALUES (?, 'Ulangan Harian Bab 3', 'public', 'open', '123456', 'export-invite-open',
		        '2026-10-20 09:00:00', 1, 0, 1, 'questions_with_key')`, ownerID)
	draftQuiz := exportHTTPExec(t, db, `
		INSERT INTO quizzes (owner_account_id, title, visibility, state, code, invite_token,
		                     max_attempts, review_mode)
		VALUES (?, 'Draf Export', 'public', 'draft', '654321', 'export-invite-draft',
		        1, 'questions_with_key')`, ownerID)

	questionID := exportHTTPExec(t, db, `
		INSERT INTO questions (quiz_id, position, type, prompt)
		VALUES (?, 1, 'single', 'Ibu kota Indonesia?')`, openQuiz)
	exportHTTPExec(t, db, `
		INSERT INTO answer_options (question_id, position, body, is_correct)
		VALUES (?, 1, 'Bandung', 0), (?, 2, 'Jakarta', 1), (?, 3, 'Surabaya', 0)`,
		questionID, questionID, questionID)

	app, err := httpserver.NewWithDeps(config.Config{
		Addr:              ":8080",
		SessionSigningKey: exportHTTPKey,
		UploadsDir:        t.TempDir(),
	}, httpserver.Deps{DB: db})
	if err != nil {
		t.Fatalf("NewWithDeps: %v", err)
	}

	sessions := authn.NewStore(db)
	issue := func(accountID int64, label string) string {
		token, _, err := sessions.Issue(context.Background(), nil, accountID, nil, "export-test")
		if err != nil {
			t.Fatalf("issue %s session: %v", label, err)
		}
		return token
	}

	return &exportHTTPWorld{
		app:          app,
		openQuiz:     openQuiz,
		draftQuiz:    draftQuiz,
		ownerToken:   issue(ownerID, "owner"),
		foreignToken: issue(foreignID, "foreign teacher"),
		studentToken: issue(studentAccountID, "student"),
	}
}

// TestTeacherExportHTTPOwnerDownloadsWorkbook pins the success contract: the
// owner gets the exact XLSX media type, an attachment filename derived from the
// quiz title, no-store caching, and a body that opens as a four-sheet workbook.
func TestTeacherExportHTTPOwnerDownloadsWorkbook(t *testing.T) {
	w := seedExportHTTPWorld(t)
	path := "/teacher/quizzes/" + strconv.FormatInt(w.openQuiz, 10) + "/export.xlsx"

	resp := w.get(t, path, w.ownerToken)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	if got, want := resp.Header.Get("Content-Type"),
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	wantDisposition := `attachment; filename="kuis-` + strconv.FormatInt(w.openQuiz, 10) +
		`-ulangan-harian-bab-3.xlsx"`
	if got := resp.Header.Get("Content-Disposition"); got != wantDisposition {
		t.Errorf("Content-Disposition = %q, want %q", got, wantDisposition)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.HasPrefix(body, []byte("PK")) {
		t.Fatalf("body does not start with a ZIP signature (first bytes %q)", body[:min(8, len(body))])
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer workbook.Close()
	wantSheets := []string{"Ringkasan", "Peserta", "Soal", "Penilaian"}
	gotSheets := workbook.GetSheetList()
	if len(gotSheets) != len(wantSheets) {
		t.Fatalf("sheets = %v, want %v", gotSheets, wantSheets)
	}
	for i := range wantSheets {
		if gotSheets[i] != wantSheets[i] {
			t.Errorf("sheet %d = %q, want %q", i, gotSheets[i], wantSheets[i])
		}
	}
	if got, err := workbook.GetCellValue("Soal", "C2"); err != nil || got != "Ibu kota Indonesia?" {
		t.Errorf("Soal!C2 = %q, %v; want the seeded prompt", got, err)
	}
}

// TestTeacherExportHTTPRejections pins the owner-only 404 contract through the
// real route class: a foreign teacher, a student, a draft quiz, an unknown id,
// and a visitor are indistinguishable. None may set a download header.
func TestTeacherExportHTTPRejections(t *testing.T) {
	w := seedExportHTTPWorld(t)
	openPath := "/teacher/quizzes/" + strconv.FormatInt(w.openQuiz, 10) + "/export.xlsx"
	draftPath := "/teacher/quizzes/" + strconv.FormatInt(w.draftQuiz, 10) + "/export.xlsx"

	cases := []struct {
		name  string
		path  string
		token string
	}{
		{"foreign teacher", openPath, w.foreignToken},
		{"student", openPath, w.studentToken},
		{"draft", draftPath, w.ownerToken},
		{"missing quiz", "/teacher/quizzes/999999/export.xlsx", w.ownerToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := w.get(t, tc.path, tc.token)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", resp.StatusCode)
			}
			if got := resp.Header.Get("Content-Disposition"); got != "" {
				t.Errorf("Content-Disposition = %q, want none on a rejection", got)
			}
			if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
				t.Errorf("Content-Type = %q, want the HTML error page", got)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if bytes.HasPrefix(body, []byte("PK")) {
				t.Error("rejection body is a ZIP, not the error page")
			}
		})
	}
}
