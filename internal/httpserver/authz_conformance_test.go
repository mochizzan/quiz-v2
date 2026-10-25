package httpserver_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// authz_conformance_test.go pins the authorization matrix itself: the table
// covers the audited surface, and every request is enforced against it even
// when no account exists.

// TestAuthorizationClassTableCoversAuditedSurface proves the matrix has one row
// per audited pair: the 77 pinned routes plus GET /healthz and
// GET /uploads/:name (OPS-220, IDN-015).
func TestAuthorizationClassTableCoversAuditedSurface(t *testing.T) {
	const audited = 79
	if got := httpserver.ClassedRoutes(); got != audited {
		t.Fatalf("authorization table has %d rows, want %d", got, audited)
	}
	for _, p := range httpserver.RegisteredPages() {
		if _, ok := httpserver.RouteClass(p.Method, p.Path); !ok {
			t.Errorf("route %s %s has no authorization class", p.Method, p.Path)
		}
	}
	for _, pair := range [][2]string{
		{"GET", "/healthz"},
		{"GET", "/uploads/:name"},
	} {
		if _, ok := httpserver.RouteClass(pair[0], pair[1]); !ok {
			t.Errorf("route %s %s has no authorization class", pair[0], pair[1])
		}
	}
}

// samplePath substitutes an inert id for every path parameter. The
// authorization matrix runs before any handler, so the concrete id is
// irrelevant here; a value that parses as a number keeps the router happy.
func samplePath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "1"
		}
	}
	return strings.Join(parts, "/")
}

// TestProtectedRoutesRedirectInProduction proves the matrix is enforced: an
// anonymous request to any route that needs an identity lands on /login, while
// a public route does not.
func TestProtectedRoutesRedirectInProduction(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})

	for _, p := range httpserver.RegisteredPages() {
		if p.Method != "GET" {
			continue
		}
		p := p
		t.Run(p.Path, func(t *testing.T) {
			class, ok := httpserver.RouteClass(p.Method, p.Path)
			if !ok {
				t.Fatalf("no class for %s", p.Path)
			}
			resp, err := app.Test(httptest.NewRequest("GET", samplePath(p.Path), nil))
			if err != nil {
				t.Fatal(err)
			}
			loc := resp.Header.Get("Location")
			switch class {
			case httpserver.ClassStudent, httpserver.ClassTeacher,
				httpserver.ClassOwnerTeacher, httpserver.ClassAnyAuth:
				if p.Path == "/teacher/quizzes/:id/stream" {
					// ATT-191: the SSE endpoint answers an anonymous request with
					// an empty 404. An EventSource cannot follow a login redirect,
					// so the redirect class's transport rule does not apply.
					if resp.StatusCode != 404 {
						t.Fatalf("status = %d, want 404 for the anonymous stream", resp.StatusCode)
					}
					body, _ := io.ReadAll(resp.Body)
					if len(body) != 0 {
						t.Errorf("stream body = %q, want empty", body)
					}
					return
				}
				if resp.StatusCode != 303 {
					t.Fatalf("status = %d, want 303 for class %s", resp.StatusCode, class)
				}
				if !strings.HasPrefix(loc, "/login") {
					t.Errorf("Location = %q, want /login", loc)
				}
			default:
				// GET / is the role router: it redirects an anonymous visitor
				// to /login on purpose (IDN-026). Every other public route
				// must not require a session.
				if p.Path == "/" {
					return
				}
				if resp.StatusCode == 303 && strings.HasPrefix(loc, "/login") {
					t.Errorf("public route redirected to /login (status 303)")
				}
			}
		})
	}
}

// TestProductionCSRFRejectsMissingToken proves the token is actually verified:
// a POST with no cookie and no field gets 403, never a handler.
func TestProductionCSRFRejectsMissingToken(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})
	req := httptest.NewRequest("POST", "/login", strings.NewReader("email=a%40b.co&password=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

// TestSessionCookieAttributes pins the cookie contract (IDN-067, IDN-069): the
// CSRF cookie is HttpOnly, SameSite=Lax, and scoped to the whole site.
func TestSessionCookieAttributes(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})
	resp, err := app.Test(httptest.NewRequest("GET", "/login", nil))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name != httpserver.CookieCSRF {
			continue
		}
		found = true
		if !c.HttpOnly {
			t.Error("CSRF cookie must be HttpOnly")
		}
		if c.SameSite != 2 { // http.SameSiteLaxMode
			t.Errorf("SameSite = %d, want Lax", c.SameSite)
		}
		if c.Path != "/" {
			t.Errorf("Path = %q, want /", c.Path)
		}
	}
	if !found {
		t.Fatal("no CSRF cookie issued")
	}
}
