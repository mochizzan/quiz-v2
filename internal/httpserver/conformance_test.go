package httpserver_test

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
	"quiz-v2/internal/httpserver/fixtures"
)

// conformance_test.go is the whole-frontend gate for the typed-data build. It
// pins the exact 72-route surface from the design contract, renders every GET
// page and exercises every POST action, and walks the template/CSS/JS sources
// for the design bans. TestMain (server_test.go) chdirs to the repository root,
// so ./templates and ./static resolve correctly.

var (
	reStyleElem = regexp.MustCompile(`(?s)<style[^>]*>(.*?)</style>`)
	reHex       = regexp.MustCompile(`#[0-9A-Fa-f]{6}\b`)
	reIconRef   = regexp.MustCompile(`href="#(i-[^"]+)"`)
	reClassAttr = regexp.MustCompile(`class="([^"]*)"`)
	reSymbolID  = regexp.MustCompile(`<symbol id="(i-[^"]+)"`)
	reTitle     = regexp.MustCompile(`(?s)<title[^>]*>(.*?)</title>`)
	reTagStrip  = regexp.MustCompile(`<[^>]*>`)
	reForm      = regexp.MustCompile(`(?is)<form\b[^>]*method="post"[^>]*>(.*?)</form>`)
	reOnHandler = regexp.MustCompile(`\son(click|change|submit|input|focus|blur|load|error)\s*=`)
)

var v5ResponsiveRe = []*regexp.Regexp{
	regexp.MustCompile(`^col-(sm|md|lg|xl)-`),
	regexp.MustCompile(`^row-cols-(sm|md|lg|xl)-`),
	regexp.MustCompile(`^d-(sm|md|lg|xl)-(block|flex|inline)`),
	regexp.MustCompile(`^(p|m|pt|pb|ps|pe|px|py|mt|mb|ms|me|mx|my|gap)-(sm|md|lg|xl)-`),
	regexp.MustCompile(`^(flex|justify-content|align-items|text|w|h)-(sm|md|lg|xl)-`),
}

// route is one method + path pair in the design contract.
type route struct{ method, path string }

func r(method, path string) route { return route{method: method, path: path} }

// contractRoutes is the exact 72-entry surface from the design contract:
// public/auth 18, student 9, attempt 6, teacher core/authoring 18, grading 4,
// management 15, error 2.
var contractRoutes = []route{
	// Public and auth (18).
	r("GET", "/"),
	r("GET", "/login"), r("POST", "/login"),
	r("GET", "/register/teacher"), r("POST", "/register/teacher"),
	r("GET", "/register/student"), r("POST", "/register/student"),
	r("GET", "/reset-password"), r("POST", "/reset-password"),
	r("GET", "/change-password"), r("POST", "/change-password"),
	r("GET", "/auth/pending"),
	r("GET", "/join"), r("POST", "/join"),
	r("GET", "/invite/:token"),
	r("GET", "/join/identify"), r("POST", "/join/identify"),
	r("POST", "/logout"),

	// Student (9).
	r("GET", "/student"),
	r("GET", "/student/history"),
	r("GET", "/student/history/:quizID"),
	r("GET", "/profile"), r("POST", "/profile"),
	r("GET", "/quiz/:id"),
	r("POST", "/quiz/:id/register"),
	r("POST", "/quiz/:id/cancel"),
	r("POST", "/quiz/:id/attempts"),

	// Attempt (6).
	r("GET", "/attempt/:id"),
	r("POST", "/attempt/:id/answer"),
	r("GET", "/attempt/:id/review"),
	r("POST", "/attempt/:id/submit"),
	r("GET", "/attempt/:id/result"),
	r("POST", "/attempt/:id/beacon"),

	// Teacher core + authoring + monitor (18).
	r("GET", "/teacher"),
	r("GET", "/teacher/quizzes"),
	r("GET", "/teacher/quizzes/new"),
	r("POST", "/teacher/quizzes"),
	r("GET", "/teacher/quizzes/:id"),
	r("POST", "/teacher/quizzes/:id"),
	r("POST", "/teacher/quizzes/:id/publish"),
	r("POST", "/teacher/quizzes/:id/close"),
	r("POST", "/teacher/quizzes/:id/delete"),
	r("GET", "/teacher/quizzes/:id/results"),
	r("GET", "/teacher/quizzes/:id/questions/new"),
	r("POST", "/teacher/quizzes/:id/questions"),
	r("GET", "/teacher/quizzes/:id/questions/:qid"),
	r("POST", "/teacher/quizzes/:id/questions/:qid"),
	r("POST", "/teacher/quizzes/:id/questions/:qid/delete"),
	r("POST", "/teacher/quizzes/:id/questions/reorder"),
	r("GET", "/teacher/quizzes/:id/monitor"),
	r("GET", "/teacher/quizzes/:id/stream"),

	// Grading + recap (4).
	r("GET", "/teacher/grading"),
	r("GET", "/teacher/grading/:answerID"),
	r("POST", "/teacher/grading/:answerID"),
	r("GET", "/teacher/recap/:attemptID"),

	// Management (15).
	r("GET", "/teacher/students"),
	r("GET", "/teacher/students/:id"),
	r("POST", "/teacher/students/:id"),
	r("POST", "/teacher/students/:id/delete"),
	r("POST", "/teacher/students/:id/reset-token"),
	r("GET", "/teacher/anonymous"),
	r("GET", "/teacher/anonymous/:id"),
	r("POST", "/teacher/anonymous/:id"),
	r("POST", "/teacher/anonymous/:id/delete"),
	r("GET", "/teacher/majors"),
	r("POST", "/teacher/majors"),
	r("POST", "/teacher/majors/:id"),
	r("POST", "/teacher/majors/:id/delete"),
	r("GET", "/teacher/teachers"),
	r("POST", "/teacher/teachers/:id/approve"),

	// Error pages (2; the 404 catch-all is additionally tested below).
	r("GET", "/error/404"),
	r("GET", "/error/500"),
}

// samplePath substitutes concrete demo ids for path parameters.
func samplePath(path string) string {
	if path == "/teacher/anonymous/:id" || path == "/teacher/anonymous/:id/delete" ||
		path == "/teacher/anonymous/:id/edit" {
		return strings.ReplaceAll(path, ":id", anonymousRegistrationID())
	}
	repl := map[string]string{
		":token":     "bio4-7f2k",
		":quizID":    "1",
		":answerID":  "400604",
		":attemptID": "1001",
		":qid":       "101",
		":id":        "1",
	}
	for token, value := range repl {
		path = strings.ReplaceAll(path, token, value)
	}
	switch path {
	case "/attempt/1":
		return "/attempt/1009"
	case "/attempt/1/review":
		return "/attempt/1009/review"
	case "/attempt/1/result":
		return "/attempt/1001/result"
	case "/attempt/1/answer", "/attempt/1/submit", "/attempt/1/beacon":
		return strings.Replace(path, "/attempt/1/", "/attempt/1009/", 1)
	}
	return path
}

func anonymousRegistrationID() string {
	for _, reg := range fixtures.Default().Registrations {
		if reg.Anonymous {
			return strconv.FormatInt(reg.ID, 10)
		}
	}
	return "1"
}

// walkTemplates returns every file under ./templates (slash-normalised).
func walkTemplates(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	root := filepath.FromSlash("templates")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(path)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk templates: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no template files found")
	}
	return files
}

// TestRouteSurface pins the exact 72 method+path pairs. Membership equality is
// used, not slice order, because registration order is an implementation detail
// (server.go sorts a copy).
func TestRouteSurface(t *testing.T) {
	want := map[string]bool{}
	for _, rt := range contractRoutes {
		key := rt.method + " " + rt.path
		if want[key] {
			t.Fatalf("duplicate route in contract fixture: %s", key)
		}
		want[key] = true
	}
	if len(want) != 72 {
		t.Fatalf("contract fixture = %d routes, want 72", len(want))
	}

	seen := map[string]int{}
	for _, p := range httpserver.RegisteredPages() {
		seen[p.Method+" "+p.Path]++
	}
	for key, n := range seen {
		if !want[key] {
			t.Errorf("unexpected registered route %s", key)
		}
		if n != 1 {
			t.Errorf("route %s registered %d times", key, n)
		}
	}
	for key := range want {
		if seen[key] == 0 {
			t.Errorf("missing registered route %s", key)
		}
	}
	if len(seen) != len(want) {
		t.Errorf("registered surface = %d routes, want %d", len(seen), len(want))
	}
}

// TestEveryRegisteredGetPageRenders renders every GET page with demo ids and
// asserts the shared chrome, a real title, one <style> block, and the
// production-mode absence of demo affordances.
func TestEveryRegisteredGetPageRenders(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080"})
	for _, p := range httpserver.RegisteredPages() {
		if p.Method != "GET" {
			continue
		}
		p := p
		t.Run(p.Path, func(t *testing.T) {
			path := samplePath(p.Path)
			resp, err := app.Test(httptest.NewRequest("GET", path, nil))
			if err != nil {
				t.Fatal(err)
			}
			if p.Redirect != "" || p.Path == "/" {
				if resp.StatusCode != 303 {
					t.Fatalf("status = %d, want 303 redirect", resp.StatusCode)
				}
				if loc := resp.Header.Get("Location"); loc != p.Redirect && p.Path == "/" {
					t.Errorf("Location = %q, want %q", loc, p.Redirect)
				}
				return
			}
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if p.Path == "/teacher/quizzes/:id/stream" {
				if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
					t.Errorf("stream Content-Type = %q, want text/event-stream", ct)
				}
			}
			body, _ := io.ReadAll(resp.Body)
			html := string(body)
			if p.Path == "/teacher/quizzes/:id/stream" {
				return
			}
			for _, want := range []string{
				`<html lang="id"`, `data-bs-theme="light"`,
				`href="/static/vendor/bootstrap/bootstrap.min.css"`,
				`href="/static/css/tokens.css"`,
				`type="module" src="/static/vendor/bootstrap/bootstrap.bundle.min.js"`,
				`type="module" src="/static/js/app.js"`,
			} {
				if !strings.Contains(html, want) {
					t.Errorf("page missing %q", want)
				}
			}
			if i, j := strings.Index(html, `href="/static/vendor/bootstrap/bootstrap.min.css"`),
				strings.Index(html, `href="/static/css/tokens.css"`); i < 0 || j < i {
				t.Errorf("tokens.css link must appear after the bootstrap link")
			}
			m := reTitle.FindStringSubmatch(html)
			if m == nil || strings.TrimSpace(m[1]) == "" {
				t.Errorf("page has no <title>")
			} else if strings.TrimSpace(m[1]) == "Quiz" {
				t.Errorf("legacy title %q on %s", m[1], p.Path)
			}
			styles := reStyleElem.FindAllStringSubmatch(html, -1)
			if len(styles) != 1 {
				t.Errorf("want exactly 1 <style> element, got %d", len(styles))
			} else {
				css := styles[0][1]
				if !strings.Contains(css, "@font-face") {
					t.Errorf("<style> missing @font-face")
				}
				for _, bad := range []string{"@import", "url(http://", "url(https://", "url(//"} {
					if strings.Contains(css, bad) {
						t.Errorf("<style> must not fetch external CSS, found %q", bad)
					}
				}
			}
			for _, forbidden := range []string{"data-state-demo", "Mode demo", "theme-neutral", "theme-dark"} {
				if strings.Contains(html, forbidden) {
					t.Errorf("production page contains %q", forbidden)
				}
			}
		})
	}
}

// TestEveryRegisteredPostRedirects exercises every POST route. UI-only actions
// must answer 303 with a relative Location; the beacon answers 204.
func TestEveryRegisteredPostRedirects(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080"})
	for _, p := range httpserver.RegisteredPages() {
		if p.Method != "POST" {
			continue
		}
		p := p
		t.Run(p.Path, func(t *testing.T) {
			req := httptest.NewRequest("POST", samplePath(p.Path), strings.NewReader(""))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if p.Path == "/attempt/:id/beacon" {
				if resp.StatusCode != 204 {
					t.Fatalf("beacon status = %d, want 204", resp.StatusCode)
				}
				return
			}
			if resp.StatusCode != 303 {
				t.Fatalf("status = %d, want 303", resp.StatusCode)
			}
			loc := resp.Header.Get("Location")
			if loc == "" || !strings.HasPrefix(loc, "/") {
				t.Errorf("Location = %q, want a relative redirect", loc)
			}
		})
	}
}

// TestDemoModeAddsStrip proves APP_DEMO is the only switch that reveals demo
// affordances, and that production ignores ?state=.
func TestDemoModeAddsStrip(t *testing.T) {
	prod := httpserver.New(config.Config{Addr: ":8080"})
	resp, err := prod.Test(httptest.NewRequest("GET", "/login", nil))
	if err != nil {
		t.Fatal(err)
	}
	prodBody, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(prodBody), `class="demo-strip"`) || strings.Contains(string(prodBody), "Mode demo") {
		t.Error("production login must not render the demo strip")
	}

	demo := httpserver.New(config.Config{Addr: ":8080", Demo: true})
	resp, err = demo.Test(httptest.NewRequest("GET", "/login", nil))
	if err != nil {
		t.Fatal(err)
	}
	demoBody, _ := io.ReadAll(resp.Body)
	for _, want := range []string{"demo-strip", "Mode demo", "tidak ada penyimpanan"} {
		if !strings.Contains(string(demoBody), want) {
			t.Errorf("demo login missing %q", want)
		}
	}
}

func TestPageRequestStateIsDemoOnly(t *testing.T) {
	req := httpserver.PageRequest{Query: map[string]string{"state": "empty"}}
	if got := req.State(); got != "" {
		t.Errorf("production State() = %q, want empty", got)
	}
	req.Demo = true
	if got := req.State(); got != "empty" {
		t.Errorf("demo State() = %q, want empty", got)
	}
}

// TestNoBootstrapV5Names checks class tokens and raw markup across templates.
func TestNoBootstrapV5Names(t *testing.T) {
	for path, body := range walkTemplates(t) {
		for _, m := range reClassAttr.FindAllStringSubmatch(body, -1) {
			for _, tok := range strings.Fields(m[1]) {
				if v5ClassBans[tok] {
					t.Errorf("%s: banned class token %q", path, tok)
				}
				if theme := themeTokRe.FindStringSubmatch(tok); theme != nil && !allowedThemes[theme[1]] {
					t.Errorf("%s: theme token %q is outside the closed theme set", path, tok)
				}
				for _, re := range v5ResponsiveRe {
					if re.MatchString(tok) {
						t.Errorf("%s: banned v5 prefix-responsive class %q", path, tok)
					}
				}
			}
		}
		for _, bad := range bannedSubstrings {
			if strings.Contains(body, bad) {
				t.Errorf("%s: banned substring %q", path, bad)
			}
		}
		for _, bad := range []string{"theme-neutral", "theme-dark"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s: banned theme token %q", path, bad)
			}
		}
		if reOnHandler.MatchString(body) {
			t.Errorf("%s: inline event handler found (use static/js/app.js)", path)
		}
	}
}

// TestPaletteHexOnly: every 6-digit hex in templates and static/css is one of
// the 24 palette values.
func TestPaletteHexOnly(t *testing.T) {
	check := func(path, body string) {
		for _, hit := range reHex.FindAllString(body, -1) {
			if !paletteHexes[strings.ToLower(hit)] {
				t.Errorf("%s: hex %s is not in the 24-colour palette", path, hit)
			}
		}
	}
	for path, body := range walkTemplates(t) {
		check(path, body)
	}
	cssDir := filepath.FromSlash("static/css")
	entries, err := os.ReadDir(cssDir)
	if err != nil {
		t.Fatalf("read static/css: %v", err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(cssDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		check("static/css/"+e.Name(), string(b))
	}
}

// TestIconRefsResolve: every referenced sprite id exists; every <svg> carrying
// a <use> is aria-hidden; every button/link whose only content is such an svg
// carries an aria-label.
func TestIconRefsResolve(t *testing.T) {
	icons := walkTemplates(t)["templates/partials/icons.html"]
	defined := map[string]bool{}
	for _, m := range reSymbolID.FindAllStringSubmatch(icons, -1) {
		defined[m[1]] = true
	}
	if len(defined) == 0 {
		t.Fatal("no sprite symbols found")
	}
	for path, body := range walkTemplates(t) {
		for _, m := range reIconRef.FindAllStringSubmatch(body, -1) {
			if strings.Contains(m[1], "{{") {
				// Data-driven icon (empty-state); the fixture-source check
				// below pins every literal value it can receive.
				continue
			}
			if !defined[m[1]] {
				t.Errorf("%s: icon #%s not defined in the sprite", path, m[1])
			}
		}
		for _, svg := range regexp.MustCompile(`(?s)<svg[^>]*>.*?</svg>`).FindAllString(body, -1) {
			if strings.Contains(svg, "<use ") && !strings.Contains(svg, `aria-hidden="true"`) {
				t.Errorf("%s: <svg> with <use> missing aria-hidden: %s", path, svg[:min(60, len(svg))])
			}
		}
		reControl := regexp.MustCompile(`(?s)<(button|a)\b([^>]*)>(.*?)</(button|a)>`)
		for _, m := range reControl.FindAllStringSubmatch(body, -1) {
			inner := m[3]
			if !strings.Contains(inner, "<svg") {
				continue
			}
			text := strings.TrimSpace(reTagStrip.ReplaceAllString(inner, ""))
			if text != "" {
				continue
			}
			if !strings.Contains(m[2], "aria-label=") {
				t.Errorf("%s: icon-only %s needs aria-label: <%s%s>", path, m[1], m[1], m[2])
			}
		}
	}

	// EmptyState icons are data-driven, so the sprite check above cannot see
	// them. Every literal Icon value in the fixtures package must resolve.
	reFixtureIcon := regexp.MustCompile(`Icon:\s+"([a-z-]+)"`)
	fixtureFiles, _ := filepath.Glob(filepath.FromSlash("internal/httpserver/fixtures/*.go"))
	for _, file := range fixtureFiles {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range reFixtureIcon.FindAllStringSubmatch(string(src), -1) {
			if !defined["i-"+m[1]] {
				t.Errorf("%s: empty-state icon #i-%s not defined in the sprite", file, m[1])
			}
		}
	}
}

// TestEveryPostFormCarriesCSRF: every POST form in a page ships the hidden
// placeholder token. This is a template contract, not server validation.
func TestEveryPostFormCarriesCSRF(t *testing.T) {
	for path, body := range walkTemplates(t) {
		for _, m := range reForm.FindAllStringSubmatch(body, -1) {
			if !strings.Contains(m[1], `name="csrf_token"`) {
				t.Errorf("%s: POST form missing name=\"csrf_token\"", path)
			}
		}
	}
}

// TestScriptWiring: bundle is a module and precedes jQuery; jQuery is defer
// and classic; window.bootstrap is absent.
func TestScriptWiring(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080"})
	resp, err := app.Test(httptest.NewRequest("GET", "/login", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	bundle := `type="module" src="/static/vendor/bootstrap/bootstrap.bundle.min.js"`
	jq := `src="/static/vendor/jquery/jquery-4.0.0.min.js"`
	i, j := strings.Index(html, bundle), strings.Index(html, jq)
	if i < 0 {
		t.Errorf("bundle script tag missing or malformed: %q", bundle)
	}
	if j < 0 {
		t.Errorf("jquery script tag missing: %q", jq)
	}
	if i >= 0 && j >= 0 && i > j {
		t.Errorf("bundle (module) must appear before jQuery")
	}
	if j >= 0 {
		end := strings.Index(html[j:], ">")
		if end < 0 {
			t.Fatalf("unterminated jquery script tag")
		}
		jqTag := html[j : j+end]
		if !strings.Contains(jqTag, "defer") {
			t.Errorf("jquery tag must carry defer: %s", jqTag)
		}
		if strings.Contains(jqTag, `type="module"`) {
			t.Errorf("jquery tag must not be type=module: %s", jqTag)
		}
	}
	if strings.Contains(html, "window.bootstrap") {
		t.Errorf("window.bootstrap must be absent")
	}
}

// TestAppJsHasNoNetworkCalls: app.js must not contain any client network API.
func TestAppJsHasNoNetworkCalls(t *testing.T) {
	b, err := os.ReadFile(filepath.FromSlash("static/js/app.js"))
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	src := stripJSComments(string(b))
	for _, api := range []string{"fetch(", "XMLHttpRequest", "EventSource", "WebSocket", "sendBeacon"} {
		if strings.Contains(src, api) {
			t.Errorf("app.js contains banned network API %q in code", api)
		}
	}
}

// stripJSComments removes comments so a comment naming a banned API is not a
// failure.
func stripJSComments(src string) string {
	src = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
