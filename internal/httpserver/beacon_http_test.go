package httpserver_test

// beacon_http_test.go drives the real Fiber pipeline for ATT-156..ATT-160. The
// beacon is the one CSRF-exempt POST, so this is the only test that can prove
// the transport contract: strict Origin, empty error bodies, Retry-After, and
// the submitted-before-rate ordering.

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
)

func TestBeaconHTTPSuccessAndErrors(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedBeaconHTTP(t, db)
	app, err := httpserver.NewWithDeps(
		config.Config{Addr: ":8080", SessionSigningKey: testSigningKey},
		httpserver.Deps{DB: db})
	if err != nil {
		t.Fatalf("NewWithDeps: %v", err)
	}
	identity := &http.Cookie{Name: "quiz_participant", Value: w.token}

	t.Run("text focus success", func(t *testing.T) {
		resp := postBeacon(t, app, w.attemptID, " blur\n", "text/plain", "http://example.com", identity)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", resp.StatusCode)
		}
		if body := bodyOf(t, resp); body != "" {
			t.Fatalf("204 body = %q, want empty", body)
		}
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM focus_loss_events WHERE attempt_id = ? AND kind = 'blur'`,
			w.attemptID).Scan(&count); err != nil {
			t.Fatalf("count focus: %v", err)
		}
		if count != 1 {
			t.Fatalf("focus rows = %d, want 1", count)
		}
	})

	t.Run("json progress success", func(t *testing.T) {
		old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		if _, err := db.Exec(`UPDATE quiz_attempts SET updated_at = ? WHERE id = ?`, old, w.attemptID); err != nil {
			t.Fatalf("age attempt: %v", err)
		}
		resp := postBeacon(t, app, w.attemptID, `{"kind":"progress","at":"2001-01-01T00:00:00Z"}`,
			"application/json; charset=utf-8", "http://example.com", identity)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", resp.StatusCode)
		}
		if body := bodyOf(t, resp); body != "" {
			t.Fatalf("204 body = %q, want empty", body)
		}
		var updated time.Time
		if err := db.QueryRow(`SELECT updated_at FROM quiz_attempts WHERE id = ?`, w.attemptID).Scan(&updated); err != nil {
			t.Fatalf("read updated_at: %v", err)
		}
		if !updated.After(old) {
			t.Fatalf("progress did not touch updated_at: %v", updated)
		}
	})

	errors := []struct {
		name        string
		attemptID   int64
		body        string
		contentType string
		origin      string
		cookie      *http.Cookie
		want        int
	}{
		{"malformed json", w.attemptID, `{"kind":`, "application/json", "http://example.com", identity, 400},
		{"unknown kind", w.attemptID, "idle", "text/plain", "http://example.com", identity, 400},
		{"empty body", w.attemptID, "", "text/plain", "http://example.com", identity, 400},
		{"wrong content type", w.attemptID, "blur", "application/x-www-form-urlencoded", "http://example.com", identity, 400},
		{"missing origin", w.attemptID, "blur", "text/plain", "", identity, 403},
		{"foreign origin", w.attemptID, "blur", "text/plain", "http://evil.example", identity, 403},
		{"scheme mismatch", w.attemptID, "blur", "text/plain", "https://example.com", identity, 403},
		{"origin with path", w.attemptID, "blur", "text/plain", "http://example.com/evil", identity, 403},
		{"foreign attempt", w.foreignAttemptID, "blur", "text/plain", "http://example.com", identity, 404},
		{"missing identity", w.attemptID, "blur", "text/plain", "http://example.com", nil, 404},
		{"unknown attempt", 999999, "blur", "text/plain", "http://example.com", identity, 404},
		{"oversized", w.attemptID, strings.Repeat("p", 4097), "text/plain", "http://example.com", identity, 400},
	}
	for _, tt := range errors {
		t.Run(tt.name, func(t *testing.T) {
			resp := postBeacon(t, app, tt.attemptID, tt.body, tt.contentType, tt.origin, tt.cookie)
			if resp.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.want)
			}
			if body := bodyOf(t, resp); body != "" {
				t.Fatalf("%d body = %q, want empty", tt.want, body)
			}
		})
	}
}

func TestBeaconHTTPRateLimitAndSubmittedNoop(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedBeaconHTTP(t, db)
	app, err := httpserver.NewWithDeps(
		config.Config{Addr: ":8080", SessionSigningKey: testSigningKey},
		httpserver.Deps{DB: db})
	if err != nil {
		t.Fatalf("NewWithDeps: %v", err)
	}
	identity := &http.Cookie{Name: "quiz_participant", Value: w.token}

	for i := 0; i < 30; i++ {
		resp := postBeacon(t, app, w.attemptID, "progress", "text/plain", "http://example.com", identity)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("progress %d/%d = %d, want 204", i+1, 30, resp.StatusCode)
		}
		_ = bodyOf(t, resp)
	}
	resp := postBeacon(t, app, w.attemptID, "progress", "text/plain", "http://example.com", identity)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("31st progress = %d, want 429", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
	if body := bodyOf(t, resp); body != "" {
		t.Fatalf("429 body = %q, want empty", body)
	}

	// A beacon queued before submit may land after it. It is a 204 no-op even
	// though the progress bucket is exhausted: submitted is checked first
	// (ATT-160).
	if _, err := db.Exec(`UPDATE quiz_attempts SET submitted_at = UTC_TIMESTAMP() WHERE id = ?`, w.attemptID); err != nil {
		t.Fatalf("submit: %v", err)
	}
	resp = postBeacon(t, app, w.attemptID, "progress", "text/plain", "http://example.com", identity)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("submitted progress = %d, want 204", resp.StatusCode)
	}
	if body := bodyOf(t, resp); body != "" {
		t.Fatalf("submitted 204 body = %q, want empty", body)
	}
}

func postBeacon(t *testing.T, app *fiber.App, attemptID int64, body, contentType, origin string, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/attempt/"+itoa64(attemptID)+"/beacon", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("POST beacon: %v", err)
	}
	return resp
}

type beaconHTTPWorld struct {
	attemptID        int64
	foreignAttemptID int64
	token            string
}

func seedBeaconHTTP(t *testing.T, db *store.DB) beaconHTTPWorld {
	t.Helper()
	ctx := context.Background()

	res, err := db.ExecContext(ctx, `
		INSERT INTO accounts (full_name, email, password_hash, role, status, approved_at)
		VALUES ('Bu Guru', 'guru@beacon-http.test', 'not-a-real-hash', 'teacher', 'active', UTC_TIMESTAMP())`)
	if err != nil {
		t.Fatalf("seed teacher: %v", err)
	}
	teacherID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx,
		`INSERT INTO majors (name, code) VALUES ('Beacon HTTP Major', 'BHM')`)
	if err != nil {
		t.Fatalf("seed major: %v", err)
	}
	majorID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, `
		INSERT INTO quizzes
			(owner_account_id, title, visibility, state, code, invite_token,
			 allow_retake, max_attempts, show_score, review_mode, published_at, closed_at)
		VALUES (?, 'Kuis Beacon HTTP', 'public', 'open', '654321', 'beacon-http-invite',
			0, 1, 1, 'questions_only', UTC_TIMESTAMP(), NULL)`, teacherID)
	if err != nil {
		t.Fatalf("seed quiz: %v", err)
	}
	quizID, _ := res.LastInsertId()

	token := "v1." + strings.Repeat("Q", 43)
	res, err = db.ExecContext(ctx, `
		INSERT INTO anonymous_participants (display_name, grade_level, major_id, cookie_token_hash)
		VALUES ('Peserta HTTP', 10, ?, ?)`, majorID, secure.HashToken(token))
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}
	anonID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx,
		`INSERT INTO registrations (quiz_id, anonymous_participant_id) VALUES (?, ?)`, quizID, anonID)
	if err != nil {
		t.Fatalf("seed registration: %v", err)
	}
	registrationID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx,
		`INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed) VALUES (?, 1, 11)`, registrationID)
	if err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	attemptID, _ := res.LastInsertId()

	foreignToken := "v1." + strings.Repeat("R", 43)
	res, err = db.ExecContext(ctx, `
		INSERT INTO anonymous_participants (display_name, grade_level, major_id, cookie_token_hash)
		VALUES ('Peserta Lain', 11, ?, ?)`, majorID, secure.HashToken(foreignToken))
	if err != nil {
		t.Fatalf("seed foreign participant: %v", err)
	}
	foreignAnonID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx,
		`INSERT INTO registrations (quiz_id, anonymous_participant_id) VALUES (?, ?)`, quizID, foreignAnonID)
	if err != nil {
		t.Fatalf("seed foreign registration: %v", err)
	}
	foreignRegistrationID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx,
		`INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed) VALUES (?, 1, 12)`, foreignRegistrationID)
	if err != nil {
		t.Fatalf("seed foreign attempt: %v", err)
	}
	foreignAttemptID, _ := res.LastInsertId()

	var submitted sql.NullTime
	if err := db.QueryRowContext(ctx,
		`SELECT submitted_at FROM quiz_attempts WHERE id = ?`, attemptID).Scan(&submitted); err != nil {
		t.Fatalf("read seeded attempt: %v", err)
	}
	if submitted.Valid {
		t.Fatal("seeded attempt is already submitted")
	}

	return beaconHTTPWorld{
		attemptID:        attemptID,
		foreignAttemptID: foreignAttemptID,
		token:            token,
	}
}
