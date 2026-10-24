package secure_test

import (
	"errors"
	"strings"
	"testing"

	"quiz-v2/internal/secure"
)

func TestValidatePassword(t *testing.T) {
	valid := []string{
		"Password1",
		"Abcdefg1",
		"KataSandi1",
		"R4hasiaSekali",
	}
	for _, p := range valid {
		if err := secure.ValidatePassword(p); err != nil {
			t.Errorf("%q rejected: %v", p, err)
		}
	}

	invalid := map[string]string{
		"too short":      "Abc123",
		"no digit":       "PasswordOnly",
		"no uppercase":   "password1",
		"leading space":  " Password1",
		"trailing space": "Password1 ",
		"inner space":    "Pass word1",
		"tab":            "Pass\tword1",
		"newline":        "Pass\nword1",
	}
	for name, p := range invalid {
		if err := secure.ValidatePassword(p); err == nil {
			t.Errorf("%s: %q accepted", name, p)
		}
	}
}

func TestPasswordTooLongUsesItsOwnError(t *testing.T) {
	// 73 ASCII bytes: over bcrypt's 72-byte ceiling.
	p := "Aa1" + strings.Repeat("x", 70)
	if err := secure.ValidatePassword(p); !errors.Is(err, secure.ErrPasswordTooLong) {
		t.Fatalf("err = %v, want ErrPasswordTooLong", err)
	}
}

func TestPasswordLengthCountsRunes(t *testing.T) {
	// Eight runes, but more than eight bytes: rune counting must accept it.
	if err := secure.ValidatePassword("KataSand1"); err != nil {
		t.Fatalf("rejected: %v", err)
	}
	// Seven runes with a multibyte character must still be too short.
	if err := secure.ValidatePassword("KataSñ1"); err == nil {
		t.Fatal("seven-rune password accepted")
	}
}

func TestHashAndVerify(t *testing.T) {
	hash, err := secure.HashPassword("Password1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$2a$12$") && !strings.HasPrefix(hash, "$2b$12$") {
		t.Fatalf("hash %q is not bcrypt cost 12", hash)
	}
	if !secure.VerifyPassword(hash, "Password1") {
		t.Fatal("correct password failed verification")
	}
	if secure.VerifyPassword(hash, "Password2") {
		t.Fatal("wrong password passed verification")
	}
	if secure.VerifyPassword("not-a-hash", "Password1") {
		t.Fatal("malformed hash must never verify")
	}
}

func TestSpendDummyCompareRuns(t *testing.T) {
	secure.SpendDummyCompare("anything") // must not panic, must not accept
}

func TestNormalizeEmail(t *testing.T) {
	valid := map[string]string{
		"  Budi@Example.COM  ": "budi@example.com",
		"a@b.co":               "a@b.co",
		"first.last@school.id": "first.last@school.id",
		"UPPER@SCHOOL.ID":      "upper@school.id",
	}
	for in, want := range valid {
		got, err := secure.NormalizeEmail(in)
		if err != nil {
			t.Errorf("%q rejected: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}

	invalid := map[string]string{
		"empty":                 "",
		"only spaces":           "   ",
		"no at":                 "budi.example.com",
		"two at":                "a@@b.co",
		"empty local":           "@b.co",
		"empty domain":          "a@",
		"no dot in domain":      "a@localhost",
		"leading dot local":     ".a@b.co",
		"trailing dot local":    "a.@b.co",
		"double dot local":      "a..b@c.co",
		"leading dot domain":    "a@.b.co",
		"trailing dot domain":   "a@b.co.",
		"double dot domain":     "a@b..co",
		"inner space":           "a b@c.co",
		"non ascii":             "büdü@c.co",
		"control char":          "a\x00b@c.co",
		"local part over 64":    strings.Repeat("a", 65) + "@b.co",
		"total length over 254": strings.Repeat("a", 64) + "@" + strings.Repeat("b", 190) + ".co",
	}
	for name, in := range invalid {
		if _, err := secure.NormalizeEmail(in); err == nil {
			t.Errorf("%s: %q accepted", name, in)
		}
	}
}

func TestNormalizeEmailRequiredVsInvalid(t *testing.T) {
	if _, err := secure.NormalizeEmail("  "); !errors.Is(err, secure.ErrEmailRequired) {
		t.Errorf("blank email err = %v, want ErrEmailRequired", err)
	}
	if _, err := secure.NormalizeEmail("nope"); !errors.Is(err, secure.ErrEmailInvalid) {
		t.Errorf("malformed email err = %v, want ErrEmailInvalid", err)
	}
}

func TestNormalizeName(t *testing.T) {
	if got := secure.NormalizeName("  Budi   Santoso  "); got != "Budi Santoso" {
		t.Errorf("NormalizeName = %q", got)
	}
}
