package httpserver_test

import (
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
)

// conformance_test.go is the whole-frontend gate for the typed-data build. It
// pins the exact 74-route surface from the design contract, renders every GET
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

	reBackLink   = regexp.MustCompile(`<a class="link-quiet[^"]*" href="([^"]+)"`)
	reCancelLink = regexp.MustCompile(`<a[^>]*href="([^"]+)"[^>]*>Batal</a>`)
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

// contractRoutes is the exact 77-entry surface from the design contract:
// public/auth 18, student 9, attempt 6, teacher core/authoring 18, grading 4,
// management 17, error 5.
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
	r("GET", "/profile/password"), r("POST", "/profile/password"),
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

	// Management (17).
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
	r("GET", "/teacher/profile"), r("POST", "/teacher/profile"),
	r("GET", "/teacher/profile/password"), r("POST", "/teacher/profile/password"),

	// Error pages (5; the 404 catch-all is additionally tested below).
	r("GET", "/error/403"),
	r("GET", "/error/404"),
	r("GET", "/error/422"),
	r("GET", "/error/429"),
	r("GET", "/error/500"),
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

// TestRouteSurface pins the exact 77 method+path pairs. Membership equality is
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
	if len(want) != 81 {
		t.Fatalf("contract fixture = %d routes, want 81", len(want))
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

// TestEveryTemplateParses parses every file under ./templates on its own. A
// template with a truncated block (an unclosed {{define}}/{{with}}) makes
// ViewLoader fail for every page set that includes it, so it must fail here,
// named, rather than as a 500 on an unrelated route.
func TestEveryTemplateParses(t *testing.T) {
	for path, body := range walkTemplates(t) {
		if _, err := template.New(path).Parse(body); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

// TestEveryRouteTemplateExists keeps every render path honest: a route that
// names a template that is not on disk (or an ActForm POST that resolved to
// none) fails here instead of at request time as a 500.
func TestEveryRouteTemplateExists(t *testing.T) {
	files := walkTemplates(t)
	for _, p := range httpserver.RegisteredPages() {
		if p.Template == "" {
			if p.ActForm != nil {
				t.Errorf("%s %s: ActForm route renders no template, so a 422 fails", p.Method, p.Path)
			}
			continue
		}
		if _, ok := files["templates/"+p.Template]; !ok {
			t.Errorf("%s %s: template %q is not a file under ./templates", p.Method, p.Path, p.Template)
		}
	}
}

// TestValidationReRendersTheForm proves the 422 contract end to end: a rejected
// POST answers the submitting page with its fields, not the generic error
// page. POST /login needs no database (validation runs before the lookup), so
// it is the cheapest witness — and every ActForm POST resolves its template the
// same way (route.Template, filled by resolveTemplates).
func TestValidationReRendersTheForm(t *testing.T) {
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})

	// The GET issues the double-submit CSRF cookie the form echoes.
	get, err := app.Test(httptest.NewRequest("GET", "/login", nil))
	if err != nil {
		t.Fatal(err)
	}
	var token string
	for _, c := range get.Cookies() {
		if c.Name == httpserver.CookieCSRF {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("GET /login issued no CSRF cookie")
	}

	// Empty credentials are a field error before any service call.
	form := url.Values{"csrf_token": {token}, "email": {""}, "password": {""}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: httpserver.CookieCSRF, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	// The form itself came back — same action, same field — rather than the
	// generic error page. Before resolveTemplates this answered 500.
	for _, want := range []string{`action="/login"`, `name="email"`, `name="password"`} {
		if !strings.Contains(html, want) {
			t.Errorf("422 body missing %q (the form did not re-render)", want)
		}
	}
	if strings.Contains(html, "Terjadi kesalahan") {
		t.Error("422 body is the generic error page, not the submitted form")
	}
}

// TestNoCancelLinkDuplicatesBackLink pins DESIGN §7's "one destination, one
// affordance" rule on form pages: the back link already cancels, so a Batal
// anchor pointing at the same URL is a duplicate. The first implementation pass
// left this pattern on quiz-new, question-editor, both profile pages, and the
// student/anonymous edit forms.
func TestNoCancelLinkDuplicatesBackLink(t *testing.T) {
	for path, body := range walkTemplates(t) {
		backs := map[string]bool{}
		for _, m := range reBackLink.FindAllStringSubmatch(body, -1) {
			backs[m[1]] = true
		}
		for _, m := range reCancelLink.FindAllStringSubmatch(body, -1) {
			if backs[m[1]] {
				t.Errorf("%s: Batal link to %q duplicates the back link", path, m[1])
			}
		}
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
				// Data-driven icon (empty-state); the literal scan below
				// pins every value a handler can set.
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
	// them. Every literal Icon value a handler produces must resolve.
	reIconLiteral := regexp.MustCompile(`Icon:\s+"([a-z-]+)"`)
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range reIconLiteral.FindAllStringSubmatch(string(src), -1) {
			if !defined["i-"+m[1]] {
				t.Errorf("%s: empty-state icon #i-%s not defined in the sprite", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Icon literals: %v", err)
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
	app := httpserver.New(config.Config{Addr: ":8080", SessionSigningKey: testSigningKey})
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

// TestNoNativeBrowserDialogs: confirm()/alert()/prompt() paint the browser's
// own chrome, ignore the design system and cannot be styled. Destructive
// actions ask through partials/dialogs.html (Bootstrap 6 dialog) instead;
// everything else states its message in the page.
func TestNoNativeBrowserDialogs(t *testing.T) {
	native := regexp.MustCompile(`\b(?:window\.)?(?:confirm|alert|prompt)\s*\(`)

	b, err := os.ReadFile(filepath.FromSlash("static/js/app.js"))
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	for _, hit := range native.FindAllString(stripJSComments(string(b)), -1) {
		t.Errorf("app.js calls the browser's native %s; use partials/dialogs.html", hit)
	}

	templateComment := regexp.MustCompile(`(?s)\{\{/\*.*?\*/\}\}`)
	for path, body := range walkTemplates(t) {
		body = templateComment.ReplaceAllString(body, "")
		if hit := native.FindString(body); hit != "" {
			t.Errorf("%s calls the browser's native %s; use partials/dialogs.html", path, hit)
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
