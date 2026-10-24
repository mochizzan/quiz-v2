package httpserver_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// tokensCSSPath is the single token stylesheet under test (spec §15.11):
// the ONLY stylesheet besides the vendored bootstrap.min.css.
const tokensCSSPath = "static/css/tokens.css"

// paletteHexes is the closed docs/DESIGN.md §2.1 palette (lower-cased) —
// the ONLY hex literals permitted anywhere in tokens.css.
var paletteHexes = map[string]bool{
	"#293681": true, "#1e2864": true, "#ffffff": true, "#4274d9": true,
	"#e24a21": true, "#fdf0ed": true, "#5589c3": true, "#eef4fa": true,
	"#95ccdd": true, "#eaf6fa": true, "#d0e7e6": true, "#edf0f4": true,
	"#f4f6f8": true, "#e2e7ec": true, "#838ea0": true, "#121721": true,
	"#687182": true, "#94a3b8": true, "#e2e6eb": true, "#eceff2": true,
	"#237a57": true, "#966000": true, "#b42318": true, "#6b7b85": true,
}

// allowedRootSelectors: tokens.css may open with :root and/or :host blocks.
var allowedRootSelectors = map[string]bool{
	":root":        true,
	":host":        true,
	":root, :host": true,
	":host, :root": true,
}

// allowedStateRules is the CLOSED state-rule list (spec §15.11).
var allowedStateRules = map[string]bool{
	".btn-solid.theme-primary:hover":         true,
	".btn-solid.theme-primary:active":        true,
	".btn-solid.theme-primary:focus-visible": true,
	".btn-close:hover":                       true,
	".menu-item:hover":                       true,
	".menu-item.active":                      true,
	".form-control:focus-visible":            true,
	".check:focus-visible":                   true,
	".radio:focus-visible":                   true,
	".switch:focus-within":                   true,
}

// allowedComponentRules is the CLOSED component-override list (spec §15.11).
var allowedComponentRules = map[string]bool{
	".card":                        true,
	".dialog":                      true,
	".drawer":                      true,
	".alert":                       true,
	".popover":                     true,
	".list-group":                  true,
	".btn":                         true,
	".btn-outline.theme-primary":   true,
	".btn-outline.theme-danger":    true,
	".btn-outline.theme-secondary": true,
	".badge":                       true,
	".pagination":                  true,
	".form-control":                true,
	".form-control:not(textarea)":  true,
	".h1":                          true,
	".h2":                          true,
	".spinner-border":              true,
}

// allowedReducedMotionMedia is the single permitted at-rule; its body may
// contain ONLY a :root block.
const allowedReducedMotionMedia = "@media (prefers-reduced-motion: reduce)"

var (
	commentRe    = regexp.MustCompile(`(?s)/\*.*?\*/`)
	tokensHexRe  = regexp.MustCompile(`#[0-9A-Fa-f]+\b`)
	customPropRe = regexp.MustCompile(`^--[A-Za-z][A-Za-z0-9-]*$`)
)

type cssDecl struct {
	prop  string
	value string
}

type cssRule struct {
	selector string // whitespace-normalized
	decls    []cssDecl
}

type cssMedia struct {
	selector string // normalized at-rule prelude, e.g. "@media (prefers-reduced-motion: reduce)"
	rules    []cssRule
}

func normalizeSelector(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// matchBrace returns the index just past the '}' that closes the '{' at
// bodyStart, with correct nesting.
func matchBrace(s string, bodyStart int) int {
	depth := 1
	for i := bodyStart; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

func parseDecls(t *testing.T, body string) []cssDecl {
	t.Helper()
	var out []cssDecl
	for _, seg := range strings.Split(body, ";") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		j := strings.IndexByte(seg, ':')
		if j < 0 {
			t.Fatalf("tokens.css: declaration without ':': %q", seg)
		}
		out = append(out, cssDecl{
			prop:  strings.TrimSpace(seg[:j]),
			value: strings.TrimSpace(seg[j+1:]),
		})
	}
	return out
}

// parseRuleList parses a brace-balanced chunk into style rules (rejecting
// nested @media). Each returned rule's body is raw text for parseDecls.
func parseRuleList(t *testing.T, s string, where string) []cssRule {
	t.Helper()
	var rules []cssRule
	i := 0
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) {
			return rules
		}
		open := strings.IndexByte(s[i:], '{')
		if open < 0 {
			if strings.TrimSpace(s[i:]) != "" {
				t.Fatalf("tokens.css: %s: trailing content outside any block: %q", where, strings.TrimSpace(s[i:]))
			}
			return rules
		}
		selector := normalizeSelector(s[i : i+open])
		if strings.HasPrefix(selector, "@media") {
			t.Fatalf("tokens.css: %s: nested @media block %q is not allowed", where, selector)
		}
		bodyStart := i + open + 1
		end := matchBrace(s, bodyStart)
		if end < 0 {
			t.Fatalf("tokens.css: %s: unbalanced braces in rule %q", where, selector)
		}
		rules = append(rules, cssRule{selector: selector, decls: parseDecls(t, s[bodyStart:end-1])})
		i = end
	}
}

// parseTokensCSS strips comments, then splits the file into top-level style
// rules and at-rule (@media) blocks.
func parseTokensCSS(t *testing.T, src string) ([]cssRule, []cssMedia) {
	t.Helper()
	s := commentRe.ReplaceAllString(src, " ")
	var rules []cssRule
	var medias []cssMedia
	i := 0
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) {
			return rules, medias
		}
		open := strings.IndexByte(s[i:], '{')
		if open < 0 {
			if strings.TrimSpace(s[i:]) != "" {
				t.Fatalf("tokens.css: trailing content outside any block: %q", strings.TrimSpace(s[i:]))
			}
			return rules, medias
		}
		selector := normalizeSelector(s[i : i+open])
		bodyStart := i + open + 1
		end := matchBrace(s, bodyStart)
		if end < 0 {
			t.Fatalf("tokens.css: unbalanced braces in block %q", selector)
		}
		body := s[bodyStart : end-1]
		if strings.HasPrefix(selector, "@media") {
			medias = append(medias, cssMedia{selector: selector, rules: parseRuleList(t, body, selector)})
		} else {
			rules = append(rules, cssRule{selector: selector, decls: parseDecls(t, body)})
		}
		i = end
	}
}

// checkDecls asserts every declaration sets a custom property only. The one
// sanctioned exception is `color-scheme: light` inside a root block — it is
// a standard property, but it is what forces the light branch of the dist's
// light-dark() tokens (plan Task 3 item 1).
func checkDecls(t *testing.T, selector string, decls []cssDecl, isRoot bool) {
	t.Helper()
	if len(decls) == 0 {
		t.Errorf("selector %q declares nothing", selector)
	}
	for _, d := range decls {
		switch {
		case d.prop == "color-scheme":
			if !isRoot {
				t.Errorf("selector %q: color-scheme is only allowed in :root/:host blocks", selector)
			}
			if d.value != "light" {
				t.Errorf("selector %q: color-scheme must be `light`, got %q", selector, d.value)
			}
		case customPropRe.MatchString(d.prop):
			if strings.Contains(d.value, "!important") {
				t.Errorf("selector %q: !important is forbidden (spec §15.11): %s", selector, d.prop)
			}
		default:
			t.Errorf("selector %q: declaration %q is not a custom property — tokens.css may set custom properties only", selector, d.prop+": "+d.value)
		}
	}
}

// TestTokensCssOnlyAllowedSelectors pins tokens.css to the closed selector
// lists of spec §15.11: :root/:host blocks, the ten state rules, the fourteen
// component overrides, and a single prefers-reduced-motion at-rule whose body
// contains only a :root block. Every declaration must set a custom property
// (the sole exception: `color-scheme: light` on :root/:host), and every hex
// literal must come from the 24-colour docs/DESIGN.md §2.1 palette.
func TestTokensCssOnlyAllowedSelectors(t *testing.T) {
	src, err := os.ReadFile(tokensCSSPath)
	if err != nil {
		t.Fatalf("read %s: %v", tokensCSSPath, err)
	}
	rules, medias := parseTokensCSS(t, string(src))

	sawRoot := false
	for _, r := range rules {
		switch {
		case allowedRootSelectors[r.selector]:
			sawRoot = true
			checkDecls(t, r.selector, r.decls, true)
		case allowedStateRules[r.selector]:
			checkDecls(t, r.selector, r.decls, false)
		case allowedComponentRules[r.selector]:
			checkDecls(t, r.selector, r.decls, false)
		default:
			t.Errorf("selector %q is not in the closed allowlist (spec §15.11): :root/:host, the state rules, or the component overrides only", r.selector)
		}
	}
	if !sawRoot {
		t.Error("tokens.css must contain a :root (or :host) block")
	}

	if len(medias) > 1 {
		t.Errorf("tokens.css must contain at most one at-rule, found %d", len(medias))
	}
	for _, m := range medias {
		if m.selector != allowedReducedMotionMedia {
			t.Errorf("at-rule %q is not allowed; the only permitted at-rule is %q", m.selector, allowedReducedMotionMedia)
			continue
		}
		if len(m.rules) != 1 {
			t.Errorf("%s must contain exactly one block, found %d", allowedReducedMotionMedia, len(m.rules))
			continue
		}
		if m.rules[0].selector != ":root" {
			t.Errorf("%s body must contain ONLY a :root block, found %q", allowedReducedMotionMedia, m.rules[0].selector)
		}
		checkDecls(t, allowedReducedMotionMedia+" :root", m.rules[0].decls, true)
	}

	stripped := commentRe.ReplaceAllString(string(src), " ")
	for _, h := range tokensHexRe.FindAllString(strings.ToLower(stripped), -1) {
		if !paletteHexes[h] {
			t.Errorf("off-palette hex %s in tokens.css (docs/DESIGN.md §2.1 allows only the 24 palette hexes)", h)
		}
	}
	if strings.Contains(stripped, "!important") {
		t.Error("tokens.css must not use !important (spec §15.11)")
	}
}

// requiredRootRoles is the closed role list from plan Task 3 Step 1: every
// one of these custom properties must be declared on :root/:host.
var requiredRootRoles = []string{
	"--bs-primary-bg:",
	"--bs-primary-contrast:",
	"--bs-primary-fg:",
	"--bs-secondary-fg:",
	"--bs-accent-bg:",
	"--bs-secondary-bg:",
	"--bs-success-bg:",
	"--bs-warning-bg:",
	"--bs-danger-bg:",
	"--bs-info-bg:",
	"--bs-fg-body:",
	"--bs-fg-3:",
	"--bs-bg-body:",
	"--bs-bg-1:",
	"--bs-bg-2:",
	"--bs-bg-3:",
	"--bs-bg-4:",
	"--bs-border-color:",
	"--bs-link-color:",
	"--bs-body-font-family:",
	"--bs-body-font-size:",
	"--bs-body-line-height:",
	"--bs-font-size-3xl:",
	"--bs-font-size-2xl:",
	"--bs-radius-pill:",
	"--bs-box-shadow:",
}

// TestTokensCssDeclaresPaletteRoles asserts :root/:host declares every
// required palette-role token, and pins the two highest-risk values:
// --bs-bg-body must stay #FFFFFF (a canvas-valued --bs-bg-body paints every
// card grey — `.card{--bs-card-bg:var(--bs-bg-body)}`), and --bs-secondary-fg
// must stay #687182 (the .fg-secondary muted-text role, never co-opted by the
// secondary action colour).
func TestTokensCssDeclaresPaletteRoles(t *testing.T) {
	src, err := os.ReadFile(tokensCSSPath)
	if err != nil {
		t.Fatalf("read %s: %v", tokensCSSPath, err)
	}
	rules, _ := parseTokensCSS(t, string(src))

	rootDecls := map[string]string{}
	for _, r := range rules {
		if !allowedRootSelectors[r.selector] {
			continue
		}
		for _, d := range r.decls {
			rootDecls[d.prop] = d.value
		}
	}

	var missing []string
	for _, name := range requiredRootRoles {
		prop := strings.TrimSuffix(name, ":")
		if _, ok := rootDecls[prop]; !ok {
			missing = append(missing, prop)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf(":root must declare %d required roles; missing: %s", len(missing), strings.Join(missing, " "))
	}

	if got := rootDecls["--bs-bg-body"]; !strings.EqualFold(got, "#FFFFFF") {
		t.Errorf("--bs-bg-body = %q, want #FFFFFF (the .card paint chain requires the card white, never the canvas)", got)
	}
	if got := rootDecls["--bs-secondary-fg"]; !strings.EqualFold(got, "#687182") {
		t.Errorf("--bs-secondary-fg = %q, want #687182 (the .fg-secondary muted-text role)", got)
	}
}
