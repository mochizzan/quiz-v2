package avatarid

import (
	"regexp"
	"strings"
	"testing"
)

// wellFormed is one hand-written name that satisfies the grammar: 32 lowercase
// hex characters with the .webp suffix.
const wellFormed = "0123456789abcdef0123456789abcdef.webp"

// TestValidGrammar pins the avatar name grammar (AVT-010/AVT-070): the
// accepted shape is exactly 32 lowercase hex characters and .webp, and every
// near-miss a database value or a request path could carry is refused.
func TestValidGrammar(t *testing.T) {
	for _, name := range []string{
		wellFormed,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.webp",
		"ffffffffffffffffffffffffffffffff.webp",
		"00000000000000000000000000000000.webp",
	} {
		if !Valid(name) {
			t.Errorf("Valid(%q) = false, want true", name)
		}
	}

	for _, name := range []string{
		"",
		".webp",
		"webp",
		// Length: one short, one long, hex only, no extension.
		"0123456789abcdef0123456789abcde.webp",
		"0123456789abcdef0123456789abcdef0.webp",
		"0123456789abcdef0123456789abcdef",
		// Case: uppercase and mixed case must not alias a real file (AVT-012).
		"0123456789ABCDEF0123456789ABCDEF.webp",
		"0123456789abcdef0123456789ABCDEF.webp",
		"0123456789abcdef0123456789abcdef.WEBP",
		// Non-hex characters.
		"gggggggggggggggggggggggggggggggg.webp",
		"-123456789abcdef0123456789abcdef.webp",
		// Other extensions and near extensions.
		"0123456789abcdef0123456789abcdef.jpg",
		"0123456789abcdef0123456789abcdef.png",
		"0123456789abcdef0123456789abcdef.webppe",
		// Traversal and path shapes.
		"../../../etc/passwd",
		"..%2f..%2favatars.webp",
		"0123456789abcdef0123456789abcdef.webp/../x.webp",
		"/avatars/0123456789abcdef0123456789abcdef.webp",
		"..\\..\\0123456789abcdef0123456789abcdef.webp",
		// Whitespace and newline: the anchors are end-of-text, not end-of-line.
		" " + wellFormed,
		wellFormed + " ",
		wellFormed + "\n",
		wellFormed + "\x00",
		// Temp files are dot-prefixed and must never pass (§4.2).
		".0123456789abcdef0123456789abcdef.tmp",
	} {
		if Valid(name) {
			t.Errorf("Valid(%q) = true, want false", name)
		}
	}
}

// TestPatternIsTheExportedGrammar: the exported pattern and Valid agree, so a
// route constraint built from Pattern admits exactly what Valid accepts.
func TestPatternIsTheExportedGrammar(t *testing.T) {
	re := regexp.MustCompile(Pattern)
	for _, name := range []string{wellFormed, "ffffffffffffffffffffffffffffffff.webp"} {
		if !re.MatchString(name) {
			t.Errorf("Pattern does not match %q", name)
		}
	}
	if re.MatchString("0123456789ABCDEF0123456789ABCDEF.webp") {
		t.Error("Pattern must reject uppercase hex")
	}
	if Pattern != `^[a-f0-9]{32}\.webp$` {
		t.Errorf("Pattern = %q, want the AVT-070 grammar", Pattern)
	}
	if Suffix != ".webp" {
		t.Errorf("Suffix = %q, want .webp", Suffix)
	}
}

// TestGeneratedNamesMatchTheGrammarAndAreUnique: 1,000 Generate() calls each
// return a name the grammar accepts, and no two collide — a collision would
// let one account's photo resolve to another's file (AVT-011).
func TestGeneratedNamesMatchTheGrammarAndAreUnique(t *testing.T) {
	const calls = 1000
	seen := make(map[string]int, calls)
	for i := 0; i < calls; i++ {
		name, err := Generate()
		if err != nil {
			t.Fatalf("Generate() #%d: %v", i, err)
		}
		if !Valid(name) {
			t.Fatalf("Generate() #%d = %q, which fails the grammar", i, name)
		}
		if len(name) != 37 {
			t.Errorf("Generate() #%d = %q, want 32 hex characters + .webp (37 bytes)", i, name)
		}
		if !strings.HasSuffix(name, Suffix) {
			t.Errorf("Generate() #%d = %q, want the %q suffix", i, name, Suffix)
		}
		if prev, dup := seen[name]; dup {
			t.Fatalf("Generate() #%d repeated the name from call #%d: %q", i, prev, name)
		}
		seen[name] = i
	}
}
