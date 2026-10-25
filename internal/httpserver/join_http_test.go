package httpserver_test

// join_http_test.go drives the real Fiber app against a migrated database. It
// exists because the domain delta and the transport delta are separate steps:
// a redirect handler that returns before parApplyCookies looks correct in a
// domain test yet leaves the browser without the invite ticket or the switched
// identity. These assertions are on Set-Cookie, not on the domain structs.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store/storetest"
)

func findCookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func getJoin(t *testing.T, app *fiber.App, path string, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

func TestJoinHTTPCookieFlow(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	ctx := context.Background()

	res, err := db.ExecContext(ctx, `
		INSERT INTO accounts (full_name, email, password_hash, role, status, approved_at)
		VALUES ('Bu Guru', 'guru@join-http.test', 'not-a-real-hash', 'teacher', 'active', UTC_TIMESTAMP())`)
	if err != nil {
		t.Fatalf("seed teacher: %v", err)
	}
	teacherID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx,
		"INSERT INTO majors (name, code) VALUES ('Rekayasa Perangkat Lunak', 'RPL')")
	if err != nil {
		t.Fatalf("seed major: %v", err)
	}
	majorID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, `
		INSERT INTO quizzes
			(owner_account_id, title, visibility, state, code, invite_token,
			 allow_retake, max_attempts, show_score, review_mode, published_at, closed_at)
		VALUES (?, 'Kuis HTTP', 'public', 'open', '123456', 'http-invite',
			0, 0, 1, 'questions_no_answers', UTC_TIMESTAMP(), NULL)`, teacherID)
	if err != nil {
		t.Fatalf("seed quiz: %v", err)
	}
	quizID, _ := res.LastInsertId()
	if _, err := db.ExecContext(ctx,
		"INSERT INTO questions (quiz_id, position, type, prompt) VALUES (?, 1, 'essay', 'Soal')", quizID); err != nil {
		t.Fatalf("seed question: %v", err)
	}

	app, err := httpserver.NewWithDeps(
		config.Config{Addr: ":8080", SessionSigningKey: testSigningKey},
		httpserver.Deps{DB: db})
	if err != nil {
		t.Fatalf("NewWithDeps: %v", err)
	}

	t.Run("invite redirect carries the ticket", func(t *testing.T) {
		resp := getJoin(t, app, "/invite/http-invite")
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", resp.StatusCode)
		}
		want := "/join/identify?quiz=" + itoa64(quizID)
		if got := resp.Header.Get("Location"); got != want {
			t.Fatalf("Location = %q, want %q", got, want)
		}
		ticket := findCookie(resp, "quiz_join_ticket")
		if ticket == nil || ticket.Value == "" {
			t.Fatalf("invite redirect did not set quiz_join_ticket: %v", resp.Cookies())
		}
		if ticket.Path != "/join" || !ticket.HttpOnly {
			t.Fatalf("ticket attributes = path %q HttpOnly %v", ticket.Path, ticket.HttpOnly)
		}
	})

	t.Run("switch clears the identity and rotates the ticket", func(t *testing.T) {
		// A fresh invite gives us a valid ticket for this subtest.
		invite := getJoin(t, app, "/invite/http-invite")
		ticket := findCookie(invite, "quiz_join_ticket")
		if ticket == nil {
			t.Fatal("invite did not set a ticket")
		}

		identityToken := "v1." + strings.Repeat("A", 43)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO anonymous_participants
				(display_name, grade_level, major_id, cookie_token_hash)
			VALUES ('Tamu HTTP', 10, ?, ?)`,
			majorID, secure.HashToken(identityToken)); err != nil {
			t.Fatalf("seed identity: %v", err)
		}

		resp := getJoin(t, app, "/join/identify?quiz="+itoa64(quizID)+"&switch=1",
			&http.Cookie{Name: "quiz_join_ticket", Value: ticket.Value},
			&http.Cookie{Name: "quiz_participant", Value: identityToken})
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", resp.StatusCode)
		}
		want := "/join/identify?quiz=" + itoa64(quizID)
		if got := resp.Header.Get("Location"); got != want {
			t.Fatalf("Location = %q, want %q", got, want)
		}

		cleared := findCookie(resp, "quiz_participant")
		if cleared == nil || cleared.Value != "" {
			t.Fatalf("switch did not expire quiz_participant: %v", resp.Cookies())
		}
		rotated := findCookie(resp, "quiz_join_ticket")
		if rotated == nil || rotated.Value == "" || rotated.Value == ticket.Value {
			t.Fatalf("switch did not rotate quiz_join_ticket: %v", resp.Cookies())
		}

		plain := getJoin(t, app, "/join/identify?quiz="+itoa64(quizID),
			&http.Cookie{Name: "quiz_join_ticket", Value: rotated.Value})
		if plain.StatusCode != http.StatusOK {
			t.Fatalf("rotated ticket status = %d, want 200", plain.StatusCode)
		}
		if body := bodyOf(t, plain); !strings.Contains(body, "Kuis HTTP") {
			t.Fatalf("identify form did not render the quiz title")
		}
	})

	t.Run("closed quiz redirect clears the ticket", func(t *testing.T) {
		invite := getJoin(t, app, "/invite/http-invite")
		ticket := findCookie(invite, "quiz_join_ticket")
		if ticket == nil {
			t.Fatal("invite did not set a ticket")
		}
		if _, err := db.ExecContext(ctx,
			"UPDATE quizzes SET state = 'closed', closed_at = UTC_TIMESTAMP() WHERE id = ?", quizID); err != nil {
			t.Fatalf("close quiz: %v", err)
		}

		resp := getJoin(t, app, "/join/identify?quiz="+itoa64(quizID),
			&http.Cookie{Name: "quiz_join_ticket", Value: ticket.Value})
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", resp.StatusCode)
		}
		if got := resp.Header.Get("Location"); got != "/join?error=not_open" {
			t.Fatalf("Location = %q, want not_open", got)
		}
		cleared := findCookie(resp, "quiz_join_ticket")
		if cleared == nil || cleared.Value != "" {
			t.Fatalf("closed redirect did not clear the ticket: %v", resp.Cookies())
		}
	})
}

// itoa64 keeps the test's path building independent of the server package's
// unexported helper.
func itoa64(n int64) string {
	return strconv.FormatInt(n, 10)
}
