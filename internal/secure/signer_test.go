package secure_test

import (
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/secure"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestSignerRejectsShortKey(t *testing.T) {
	if _, err := secure.NewSigner([]byte("too-short")); err == nil {
		t.Fatal("expected a short key to be rejected")
	}
}

func TestCSRFRoundTrip(t *testing.T) {
	s, err := secure.NewSigner(testKey)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.CSRF()
	if err != nil {
		t.Fatal(err)
	}
	if !s.VerifyCSRF(token) {
		t.Fatal("a freshly minted token must verify")
	}
	if !s.MatchCSRF(token, token) {
		t.Fatal("cookie and form carrying the same value must match")
	}
	if s.MatchCSRF(token, "different") {
		t.Fatal("a mismatched form value must not pass")
	}
}

func TestCSRFRejectsTampering(t *testing.T) {
	s, _ := secure.NewSigner(testKey)
	token, _ := s.CSRF()
	nonce, sig, _ := strings.Cut(token, ".")

	cases := map[string]string{
		"changed nonce":   "AAAA" + nonce[4:] + "." + sig,
		"changed sig":     nonce + "." + "AAAA" + sig[4:],
		"missing sig":     nonce + ".",
		"no separator":    nonce + sig,
		"two separators":  nonce + "." + sig + ".x",
		"empty":           "",
		"nonce not bytes": "AAAA." + sig,
	}
	for name, value := range cases {
		if s.VerifyCSRF(value) {
			t.Errorf("%s: forged token verified", name)
		}
	}
}

func TestTokenRoundTrip(t *testing.T) {
	s, _ := secure.NewSigner(testKey)
	now := time.Unix(1_700_000_000, 0).UTC()
	token, err := s.Token("pending", []byte("42"), now)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.ParseToken("pending", token, 24*time.Hour, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if string(data) != "42" {
		t.Errorf("payload = %q, want 42", data)
	}
}

func TestTokenPurposeIsBound(t *testing.T) {
	s, _ := secure.NewSigner(testKey)
	now := time.Now().UTC()
	token, _ := s.Token("pending", []byte("42"), now)
	if _, err := s.ParseToken("join", token, time.Hour, now); err == nil {
		t.Fatal("a pending token must not parse as a join ticket")
	}
}

func TestTokenExpiry(t *testing.T) {
	s, _ := secure.NewSigner(testKey)
	now := time.Unix(1_700_000_000, 0).UTC()
	token, _ := s.Token("pending", nil, now)
	if _, err := s.ParseToken("pending", token, 24*time.Hour, now.Add(24*time.Hour)); err != secure.ErrTokenExpired {
		t.Fatalf("err = %v, want ErrTokenExpired", err)
	}
	if _, err := s.ParseToken("pending", token, 24*time.Hour, now.Add(-time.Second)); err != secure.ErrTokenInvalid {
		t.Fatal("a token issued in the future must be invalid")
	}
}

func TestTokenRejectsTampering(t *testing.T) {
	s, _ := secure.NewSigner(testKey)
	now := time.Now().UTC()
	token, _ := s.Token("pending", []byte("42"), now)

	if _, err := s.ParseToken("pending", token+"x", time.Hour, now); err == nil {
		t.Error("a token with trailing data must be invalid")
	}
	if _, err := s.ParseToken("pending", "x"+token, time.Hour, now); err == nil {
		t.Error("a token with leading data must be invalid")
	}
	tampered := "A" + token
	if _, err := s.ParseToken("pending", tampered, time.Hour, now); err == nil {
		t.Error("a tampered token must be invalid")
	}
}

func TestNewTokenShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		tok, err := secure.NewToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != 43 {
			t.Fatalf("token length = %d, want 43", len(tok))
		}
		if seen[tok] {
			t.Fatal("token repeated")
		}
		seen[tok] = true
		if len(secure.HashToken(tok)) != 32 {
			t.Fatal("hash must be 32 raw bytes")
		}
	}
}

func TestEqualIsLengthAware(t *testing.T) {
	if !secure.Equal("abc", "abc") {
		t.Error("equal strings must match")
	}
	if secure.Equal("abc", "abcd") {
		t.Error("different lengths must not match")
	}
	if secure.Equal("abc", "abd") {
		t.Error("different content must not match")
	}
}
