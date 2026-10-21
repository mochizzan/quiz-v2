package httpserver_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// Plan Task 5 — icon sprite + 15 shared chrome partials. These tests pin the
// sub-contracts the plan's Interfaces section fixes for every partial. TestMain
// (server_test.go) chdirs to the repository root, so the relative paths below
// resolve to the real templates directory.

// pinnedPartials: the exact partial filename list of plan Task 5.
var pinnedPartials = []string{
	"icons", "head", "foot", "navbar-teacher", "navbar-student",
	"drawer-teacher", "breadcrumb", "empty-state", "banners", "toasts",
	"no-js-banner", "progress-bar", "stepper", "dialogs", "skeleton",
}

// iconCatalog: the exact symbol id list pinned in plan Task 5 (47 icons).
var iconCatalog = []string{
	"i-dashboard", "i-quiz", "i-bank", "i-users", "i-grade", "i-settings",
	"i-bell", "i-search", "i-menu", "i-plus", "i-bolt", "i-trend",
	"i-clipboard", "i-boxes", "i-sparkle", "i-tower", "i-clock", "i-check",
	"i-chev-left", "i-chev-right", "i-chev-down", "i-chev-up",
	"i-arrow-left", "i-arrow-right", "i-star", "i-pencil", "i-send",
	"i-grid", "i-warn", "i-refresh", "i-calendar", "i-layers", "i-cap",
	"i-signout", "i-radio", "i-image", "i-upload", "i-trash", "i-eye",
	"i-eye-off", "i-lock", "i-info", "i-close", "i-person", "i-history",
	"i-flag", "i-dots",
}

// paletteHex: the 24 DESIGN §2.1 hexes, lowercase — the ONLY hexes permitted
// in authored markup or inline styles.
var paletteHex = map[string]bool{
	"#293681": true, "#1e2864": true, "#ffffff": true, "#4274d9": true,
	"#e24a21": true, "#fdf0ed": true, "#5589c3": true, "#eef4fa": true,
	"#95ccdd": true, "#eaf6fa": true, "#d0e7e6": true, "#edf0f4": true,
	"#f4f6f8": true, "#e2e7ec": true, "#838ea0": true, "#121721": true,
	"#687182": true, "#94a3b8": true, "#e2e6eb": true, "#eceff2": true,
	"#237a57": true, "#966000": true, "#b42318": true, "#6b7b85": true,
}

// v5ClassBans: the class tokens the v5→v6 map renames (plan Global Constraints
// + Task 15's TestNoBootstrapV5Names list, the entries meaningful for partials).
var v5ClassBans = map[string]bool{
	"btn-primary": true, "btn-outline-primary": true, "text-primary": true,
	"text-muted": true, "text-white": true, "text-bg-primary": true,
	"form-select": true, "form-check-input": true, "modal": true,
	"modal-dialog": true, "modal-content": true, "offcanvas": true,
	"offcanvas-header": true, "offcanvas-body": true, "dropdown-menu": true,
	"dropdown-item": true, "dropdown-toggle": true, "placeholder-glow": true,
	"d-lg-block": true, "d-md-block": true, "d-md-flex": true,
	"d-sm-block": true, "d-xl-flex": true, "d-md-none": true, "d-lg-none": true,
	"fs-1": true, "fs-2": true, "fs-3": true, "fs-4": true, "fs-5": true,
	"fs-6": true, "sr-only": true, "text-body": true, "btn-block": true,
	"font-weight-bold": true, "font-italic": true, "bg-opacity-10": true,
	"text-opacity-75": true, "border-opacity-25": true,
	"bg-body-secondary": true, "bg-body-tertiary": true,
	"accordion-button": true, "accordion-collapse": true,
	"link-underline": true, "ratio": true, "spinner-border-sm": true,
}

var v5ClassPrefixRe = regexp.MustCompile(`^(col-(sm|md|lg|xl)-|row-cols-(sm|md|lg|xl)-|d-(sm|md|lg|xl)-(block|flex|inline))`)

// bannedSubstrings: raw markup/fingerprint bans (CDNs, icon fonts, v5 data-API
// names, focus suppression, emoji codepoints are checked separately).
var bannedSubstrings = []string{
	`data-bs-toggle="modal"`, `data-bs-toggle="offcanvas"`,
	`data-bs-toggle="dropdown"`, "window.bootstrap", "cdn.",
	"bootstrap@", "code.jquery", "fonts.googleapis", "outline: none",
	"outline:none", "fa-solid", "fa-regular", "bi bi-", "https://",
	`<i class="fa`, `class="fas `, `class="far `,
}

// inlineStyleProps: the spec §15.11 closed list of properties an authored
// style="" attribute may set.
var inlineStyleProps = map[string]bool{
	"padding-bottom": true, "width": true, "position": true, "z-index": true,
	"cursor": true, "overflow": true, "print-color-adjust": true,
	"aspect-ratio": true, "object-fit": true, "white-space": true,
	"color": true, "background-color": true, "border-color": true,
	"font-family": true, "font-size": true, "line-height": true,
	"border-radius": true, "box-shadow": true,
}

var (
	t5HexRe     = regexp.MustCompile(`#[0-9A-Fa-f]{6}`)
	styleRe     = regexp.MustCompile(`style="([^"]*)"`)
	classAttrRe = regexp.MustCompile(`class="([^"]*)"`)
)

func readPartial(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("templates", "partials", name+".html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestEveryPartialDefinesItsName: every pinned partial file exists and its body
// is wrapped in {{define "partials/<file>.html"}} … {{end}}; no partial may
// define "content" (that name is reserved for pages).
func TestEveryPartialDefinesItsName(t *testing.T) {
	for _, name := range pinnedPartials {
		t.Run(name, func(t *testing.T) {
			body := readPartial(t, name)
			if name == "banners" {
				// Plan-pinned exception: banners.html ships the three variant
				// defines, never a file-named define.
				for _, variant := range []string{"banner-error", "banner-stale", "banner-locked"} {
					want := `{{define "partials/` + variant + `"}}`
					if !strings.Contains(body, want) {
						t.Errorf("banners.html must contain %q", want)
					}
				}
			} else {
				want := `{{define "partials/` + name + `.html"}}`
				if !strings.Contains(body, want) {
					t.Errorf("%s.html must contain %q", name, want)
				}
			}
			if strings.Contains(body, `{{define "content"`) {
				t.Errorf("%s.html must never define \"content\" (page contract)", name)
			}
		})
	}
}

// TestEveryPartialKeepsGlobalConstraints: over every pinned partial — no v5
// class names, no CDN/icon-font/v5-data-API fingerprints, no emoji codepoints,
// only palette hexes, and inline styles restricted to the §15.11 closed list.
func TestEveryPartialKeepsGlobalConstraints(t *testing.T) {
	for _, name := range pinnedPartials {
		t.Run(name, func(t *testing.T) {
			body := readPartial(t, name)

			for _, banned := range bannedSubstrings {
				if strings.Contains(body, banned) {
					t.Errorf("banned substring %q found in %s.html", banned, name)
				}
			}

			for _, m := range classAttrRe.FindAllStringSubmatch(body, -1) {
				for _, tok := range strings.Fields(m[1]) {
					if v5ClassBans[tok] || v5ClassPrefixRe.MatchString(tok) {
						t.Errorf("v5 class token %q in %s.html class attribute", tok, name)
					}
				}
			}

			for _, m := range styleRe.FindAllStringSubmatch(body, -1) {
				for _, decl := range strings.Split(m[1], ";") {
					decl = strings.TrimSpace(decl)
					if decl == "" {
						continue
					}
					kv := strings.SplitN(decl, ":", 2)
					if len(kv) != 2 {
						t.Errorf("malformed inline declaration %q in %s.html", decl, name)
						continue
					}
					prop := strings.TrimSpace(kv[0])
					if !inlineStyleProps[prop] {
						t.Errorf("inline property %q in %s.html is outside the §15.11 closed list", prop, name)
					}
				}
			}

			for _, m := range t5HexRe.FindAllString(body, -1) {
				if !paletteHex[strings.ToLower(m)] {
					t.Errorf("off-palette hex %s in %s.html", m, name)
				}
			}

			for _, r := range body {
				if unicode.Is(unicode.So, r) && r >= 0x2600 || r >= 0x1F000 {
					t.Errorf("emoji/symbol codepoint U+%04X in %s.html (emoji-as-icon banned)", r, name)
					break
				}
			}
		})
	}
}

// TestEveryPartialSubContracts: the fixed sub-contracts per partial from the
// plan's Interfaces section (head/foot wiring live in TestFootScriptOrder and
// are re-anchored here only where the assertion is order-independent).
func TestEveryPartialSubContracts(t *testing.T) {
	t.Run("head", func(t *testing.T) {
		body := readPartial(t, "head")
		for _, want := range []string{
			`<meta charset="utf-8">`,
			`<meta name="viewport"`,
			`<title>{{template "partials/title" .}}</title>`,
			`{{define "partials/title"}}Twisty Quiz{{end}}`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("head.html missing %q", want)
			}
		}
		bootstrapIdx := strings.Index(body, `href="/static/bootstrap/bootstrap.min.css"`)
		tokensIdx := strings.Index(body, `href="/static/css/tokens.css"`)
		if bootstrapIdx < 0 || tokensIdx < 0 {
			t.Fatalf("head.html must link both stylesheets (bootstrap=%d tokens=%d)", bootstrapIdx, tokensIdx)
		}
		if bootstrapIdx > tokensIdx {
			t.Error("head.html must emit bootstrap.min.css BEFORE tokens.css (spec §15.1)")
		}
		if n := strings.Count(body, "<style"); n != 1 {
			t.Errorf("head.html must emit exactly ONE <style> element, found %d", n)
		}
		for _, want := range []string{"@font-face", "PlusJakartaSans-latin.woff2", "font-weight: 400 700", "OFL"} {
			if !strings.Contains(body, want) {
				t.Errorf("head.html <style> must contain %q", want)
			}
		}
	})

	t.Run("progress-bar", func(t *testing.T) {
		body := readPartial(t, "progress-bar")
		for _, want := range []string{
			`class="progress" role="progressbar"`,
			`aria-valuemin="0" aria-valuemax="100" aria-valuenow="{{.}}"`,
			`class="progress-bar theme-primary"`,
			`style="width: {{.}}%"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("progress-bar.html missing %q", want)
			}
		}
	})

	t.Run("toasts", func(t *testing.T) {
		body := readPartial(t, "toasts")
		for _, want := range []string{
			`toast-container`,
			`class="toast theme-success show"`,
			`background-color: #121721`,
			`color: #FFFFFF`,
			`role="status"`,
			`aria-live="polite"`,
			`data-bs-dismiss="toast"`,
			`aria-label="Tutup notifikasi"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("toasts.html missing %q", want)
			}
		}
	})

	t.Run("no-js-banner", func(t *testing.T) {
		body := readPartial(t, "no-js-banner")
		if !strings.Contains(body, `alert theme-warning`) {
			t.Error("no-js-banner.html must be an alert theme-warning")
		}
	})

	t.Run("navbar-teacher", func(t *testing.T) {
		body := readPartial(t, "navbar-teacher")
		for _, want := range []string{
			"TWISTY QUIZ", `href="/teacher"`, `href="/teacher/quizzes"`,
			`href="/teacher/questions"`, `href="/teacher/students"`,
			`href="/teacher/grading"`, `href="/teacher/teachers"`,
			`aria-current="page"`, `data-bs-toggle="drawer"`,
			`data-bs-target="#nav-drawer"`, `aria-label="Buka menu"`,
			`aria-label="Notifikasi"`, `for="teacher-nav-search"`, ">Cari<",
			`class="form-control"`, `visually-hidden`,
			"Ratna Wijaya", ">RW<",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("navbar-teacher.html missing %q", want)
			}
		}
		hamburger := `class="btn d-inline-flex xl:d-none"`
		if !strings.Contains(body, hamburger) {
			t.Errorf("navbar-teacher.html hamburger must carry %q (verified v6 responsive hide)", hamburger)
		}
	})

	t.Run("navbar-student", func(t *testing.T) {
		body := readPartial(t, "navbar-student")
		for _, want := range []string{
			"TWISTY QUIZ", `href="/student"`, `href="/student/history"`,
			`href="/profile"`, `aria-current="page"`, "Ayu Lestari", ">AL<",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("navbar-student.html missing %q", want)
			}
		}
	})

	t.Run("drawer-teacher", func(t *testing.T) {
		body := readPartial(t, "drawer-teacher")
		for _, want := range []string{
			`<dialog class="drawer drawer-end" id="nav-drawer">`,
			`drawer-header`, `drawer-body`, `data-bs-dismiss="drawer"`,
			`aria-label="Tutup"`,
			`href="/teacher"`, `href="/teacher/quizzes"`,
			`href="/teacher/questions"`, `href="/teacher/students"`,
			`href="/teacher/grading"`, `href="/teacher/teachers"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("drawer-teacher.html missing %q", want)
			}
		}
	})

	t.Run("breadcrumb", func(t *testing.T) {
		body := readPartial(t, "breadcrumb")
		for _, want := range []string{
			`<nav aria-label="Breadcrumb">`, `class="breadcrumb"`,
			`breadcrumb-link`, `breadcrumb-divider`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("breadcrumb.html missing %q", want)
			}
		}
		if !strings.Contains(body, `<li class="breadcrumb-divider" aria-hidden="true"></li>`) {
			t.Error("breadcrumb.html must ship EMPTY .breadcrumb-divider separators (v6 :empty mask shape)")
		}
	})

	t.Run("empty-state", func(t *testing.T) {
		body := readPartial(t, "empty-state")
		for _, want := range []string{
			`class="card"`, `alert theme-info`, `<h2 class="h2">`,
			"Belum ada data.", `btn btn-solid theme-primary`, "Buat kuis baru",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("empty-state.html missing %q", want)
			}
		}
	})

	t.Run("banners", func(t *testing.T) {
		body := readPartial(t, "banners")
		for _, want := range []string{
			`{{define "partials/banner-error"}}`,
			`{{define "partials/banner-stale"}}`,
			`{{define "partials/banner-locked"}}`,
			`alert theme-danger`, `alert theme-warning`,
			`btn btn-outline theme-danger`, "Coba lagi", `role="alert"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("banners.html missing %q", want)
			}
		}
	})

	t.Run("stepper", func(t *testing.T) {
		body := readPartial(t, "stepper")
		for _, want := range []string{
			`<ol`, "Data kuis", "Soal", "Pengaturan", "Terbitkan",
			`badge theme-primary`, `aria-current="step"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("stepper.html missing %q", want)
			}
		}
	})

	t.Run("dialogs", func(t *testing.T) {
		body := readPartial(t, "dialogs")
		for _, want := range []string{
			`<dialog class="dialog" id="dialog-confirm">`,
			"Konfirmasi", "Lanjutkan?", "Batal",
			`data-bs-dismiss="dialog"`, `btn btn-solid theme-primary`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("dialogs.html missing %q", want)
			}
		}
	})

	t.Run("skeleton", func(t *testing.T) {
		body := readPartial(t, "skeleton")
		if !strings.Contains(body, `aria-busy="true"`) {
			t.Error("skeleton.html must carry aria-busy=\"true\"")
		}
		if n := strings.Count(body, "placeholder placeholder-wave"); n != 3 {
			t.Errorf("skeleton.html must ship exactly 3 placeholder rows, found %d", n)
		}
		for _, want := range []string{`visually-hidden`, "Memuat"} {
			if !strings.Contains(body, want) {
				t.Errorf("skeleton.html missing %q", want)
			}
		}
	})
}

// TestIconSpriteHasCatalog: icons.html is the inline SVG sprite — every pinned
// id exists as a 24x24 stroke symbol, the root carries stroke="currentColor"
// and class="d-none" (never an inline display:none), and no partial ships
// icon-font markup.
func TestIconSpriteHasCatalog(t *testing.T) {
	icons := readPartial(t, "icons")

	rootPrefix := `<svg class="d-none" aria-hidden="true" xmlns="http://www.w3.org/2000/svg"`
	if !strings.Contains(icons, rootPrefix) {
		t.Errorf("sprite root must start %q (NOT inline display:none)", rootPrefix)
	}
	if strings.Contains(icons, `style="display:none"`) || strings.Contains(icons, `style="display: none"`) {
		t.Error("sprite root must not use inline display:none (outside the §15.11 closed list)")
	}
	if !strings.Contains(icons, `stroke="currentColor"`) {
		t.Error("sprite root must carry stroke=\"currentColor\"")
	}

	symbolRe := regexp.MustCompile(`<symbol id="(i-[a-z-]+)"`)
	seen := map[string]bool{}
	for _, m := range symbolRe.FindAllStringSubmatch(icons, -1) {
		if seen[m[1]] {
			t.Errorf("duplicate symbol id %q", m[1])
		}
		seen[m[1]] = true
	}
	for _, id := range iconCatalog {
		if !seen[id] {
			t.Errorf("catalog symbol %q missing from icons.html", id)
		}
	}
	for id := range seen {
		found := false
		for _, want := range iconCatalog {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("icons.html ships symbol %q outside the pinned catalog", id)
		}
	}
	if len(iconCatalog) != 47 {
		t.Errorf("pinned catalog must hold 47 ids, got %d", len(iconCatalog))
	}

	// Every symbol: 24x24 viewBox and non-empty stroke geometry before </symbol>.
	symbolBlockRe := regexp.MustCompile(`<symbol id="(i-[a-z-]+)" viewBox="0 0 24 24">(.*?)</symbol>`)
	geomRe := regexp.MustCompile(`<path|<circle|<rect|<line|<polyline|<polygon`)
	seenShape := map[string]bool{}
	for _, m := range symbolBlockRe.FindAllStringSubmatch(icons, -1) {
		seenShape[m[1]] = true
		if !geomRe.MatchString(m[2]) {
			t.Errorf("symbol %q has no stroke geometry", m[1])
		}
	}
	for _, id := range iconCatalog {
		if !seenShape[id] {
			t.Errorf("symbol %q must be <symbol id=%q viewBox=\"0 0 24 24\"> (hand-drawn geometry)", id, id)
		}
	}

	// No icon-font markup anywhere in templates/partials/.
	entries, err := os.ReadDir(filepath.Join("templates", "partials"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		data, err := os.ReadFile(filepath.Join("templates", "partials", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{`<i class="fa`, "fa-solid", "fa-regular", "bi bi-", `<svg class="fa`} {
			if strings.Contains(string(data), banned) {
				t.Errorf("icon-font markup %q in %s (sprite-only rule, spec §15.9)", banned, e.Name())
			}
		}
	}
}

// TestFootScriptOrder: foot.html emits, in order (AC-217) — the <noscript>
// no-js banner wrapper, the Bootstrap ESM bundle, jQuery (deferred, classic,
// never a module), then app.js as a module — and never closes the document.
func TestFootScriptOrder(t *testing.T) {
	foot := readPartial(t, "foot")

	noscriptIdx := strings.Index(foot, "<noscript>")
	bannerIdx := strings.Index(foot, `{{template "partials/no-js-banner.html" .}}`)
	bundleIdx := strings.Index(foot, `<script type="module" src="/static/bootstrap/bootstrap.bundle.min.js">`)
	jqueryIdx := strings.Index(foot, `<script src="/static/jquery/jquery-4.0.0.min.js"`)
	appIdx := strings.Index(foot, `<script type="module" src="/static/js/app.js">`)

	if noscriptIdx < 0 || bannerIdx < 0 {
		t.Fatalf("foot.html must wrap the no-js banner in <noscript> (noscript=%d banner=%d)", noscriptIdx, bannerIdx)
	}
	if bundleIdx < 0 {
		t.Fatal("foot.html missing the bootstrap ESM bundle script tag")
	}
	if jqueryIdx < 0 {
		t.Fatal("foot.html missing the jquery script tag")
	}
	if appIdx < 0 {
		t.Fatal("foot.html missing the app.js module script tag")
	}
	if !(noscriptIdx < bundleIdx && bundleIdx < jqueryIdx && jqueryIdx < appIdx) {
		t.Errorf("foot.html script order wrong: noscript=%d bundle=%d jquery=%d app=%d, want strictly increasing",
			noscriptIdx, bundleIdx, jqueryIdx, appIdx)
	}

	// Isolate the jquery tag and pin its attributes.
	tag := foot[jqueryIdx:]
	if end := strings.Index(tag, ">"); end > 0 {
		tag = tag[:end]
	} else {
		t.Fatal("jquery script tag is not terminated")
	}
	if !strings.Contains(tag, "defer") {
		t.Errorf("jquery tag must carry defer: %q", tag)
	}
	if strings.Contains(tag, `type="module"`) {
		t.Errorf("jquery tag must NOT be a module (classic script): %q", tag)
	}

	if strings.Contains(foot, "</body>") || strings.Contains(foot, "</html>") {
		t.Error("foot.html must NOT emit </body></html> — layouts close the document")
	}
}
