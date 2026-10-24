package participation

// token_test.go pins the two PAR bearer formats (PAR-001..020): the identity
// token shape and derivation, plus the join ticket mint/parse contract. These
// are pure functions, so the suite needs no database.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func signedPayload(payload string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ticketContext + payload))
	return identityPrefix +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestTicketRoundTrip(t *testing.T) {
	key := []byte("ticket-round-trip-key")
	now := readTestNow

	for _, tc := range []struct {
		name   string
		method string
		secret string
	}{
		{"code", MethodCode, "123456"},
		{"invite", MethodInvite, "invite-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := mintTicket(key, 42, tc.method, tc.secret, now)
			if err != nil {
				t.Fatalf("mintTicket: %v", err)
			}
			parts := strings.Split(raw, ".")
			if len(parts) != 3 || parts[0] != "v1" {
				t.Fatalf("ticket parts = %d %q", len(parts), parts[0])
			}
			got, err := parseTicket(key, raw, now)
			if err != nil {
				t.Fatalf("parseTicket: %v", err)
			}
			if got.QuizID != 42 || got.Method != tc.method || got.Secret != tc.secret {
				t.Fatalf("ticket = %+v, want quiz 42 %s %q", got, tc.method, tc.secret)
			}
			if !got.Issued.Equal(now) {
				t.Fatalf("issued = %v, want %v", got.Issued, now)
			}
			if len(got.Nonce) != 32 {
				t.Fatalf("nonce = %q, want 32 hex characters", got.Nonce)
			}
			for _, c := range got.Nonce {
				if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
					t.Fatalf("nonce = %q, want lowercase hex", got.Nonce)
				}
			}
		})
	}
}

func TestTicketRejectsTamperingAndWrongKey(t *testing.T) {
	key := []byte("ticket-tamper-key")
	now := readTestNow
	raw, err := mintTicket(key, 7, MethodCode, "123456", now)
	if err != nil {
		t.Fatalf("mintTicket: %v", err)
	}

	// Change the first signature character, not the last: a raw-base64 tail can
	// carry unused padding bits, so rewriting it may decode to the same bytes
	// and is not a tamper at all. The first character always changes the MAC.
	parts := strings.Split(raw, ".")
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	parts[2] = string(sig)
	tampered := strings.Join(parts, ".")
	if tampered == raw {
		t.Fatal("tamper did not change the ticket")
	}
	if _, err := parseTicket(key, tampered, now); err == nil {
		t.Fatal("tampered signature parsed")
	}
	if _, err := parseTicket([]byte("another-key"), raw, now); err == nil {
		t.Fatal("wrong signing key parsed")
	}
	for _, bad := range []string{"", "v1", "v2.abc.def", "v1.abc", "v1..", "v1.abc.def.ghi"} {
		if _, err := parseTicket(key, bad, now); err == nil {
			t.Errorf("malformed ticket %q parsed", bad)
		}
	}
}

func TestTicketWindow(t *testing.T) {
	key := []byte("ticket-window-key")
	now := readTestNow
	raw, err := mintTicket(key, 9, MethodInvite, "invite-window", now)
	if err != nil {
		t.Fatalf("mintTicket: %v", err)
	}

	if _, err := parseTicket(key, raw, now.Add(29*time.Minute)); err != nil {
		t.Fatalf("ticket inside the window rejected: %v", err)
	}
	if _, err := parseTicket(key, raw, now.Add(30*time.Minute+time.Second)); err == nil {
		t.Fatal("ticket older than 30 minutes parsed")
	}
	if _, err := parseTicket(key, raw, now.Add(-59*time.Second)); err != nil {
		t.Fatalf("ticket inside the 60-second skew rejected: %v", err)
	}
	if _, err := parseTicket(key, raw, now.Add(-61*time.Second)); err == nil {
		t.Fatal("ticket more than 60 seconds in the future parsed")
	}
}

func TestParseTicketPayloadFields(t *testing.T) {
	now := readTestNow
	iat := "iat=" + itoa(uint64(now.Unix()))
	nonce := "n=0123456789abcdef0123456789abcdef"
	valid := "v1|q=5|m=code|s=123456|" + iat + "|" + nonce
	if _, err := parseTicketPayload(valid, now); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}

	for name, payload := range map[string]string{
		"empty":              "",
		"wrong version":      "v2|q=5|m=code|s=123456|" + iat + "|" + nonce,
		"missing version":    "q=5|m=code|s=123456|" + iat + "|" + nonce,
		"bare field":         "v1|q=5|m=code|s=123456|" + iat + "|" + nonce + "|oops",
		"zero quiz":          "v1|q=0|m=code|s=123456|" + iat + "|" + nonce,
		"quiz not numeric":   "v1|q=x|m=code|s=123456|" + iat + "|" + nonce,
		"unknown method":     "v1|q=5|m=other|s=123456|" + iat + "|" + nonce,
		"code wrong shape":   "v1|q=5|m=code|s=12345|" + iat + "|" + nonce,
		"invite wrong shape": "v1|q=5|m=invite|s=bad token|" + iat + "|" + nonce,
		"bad issue time":     "v1|q=5|m=code|s=123456|iat=soon|" + nonce,
		"short nonce":        "v1|q=5|m=code|s=123456|" + iat + "|n=abcd",
		"uppercase nonce":    "v1|q=5|m=code|s=123456|" + iat + "|n=0123456789ABCDEF0123456789abcdef",
		"duplicate key":      "v1|q=5|q=6|m=code|s=123456|" + iat + "|" + nonce,
		"missing field":      "v1|q=5|m=code|s=123456|" + iat,
		"extra field":        valid + "|x=1",
	} {
		if _, err := parseTicketPayload(payload, now); err == nil {
			t.Errorf("%s payload parsed: %q", name, payload)
		}
	}
}

func TestIdentityTokenDerivation(t *testing.T) {
	key := []byte("identity-derivation-key")
	first := newIdentityToken(key, "0123456789abcdef0123456789abcdef")
	second := newIdentityToken(key, "0123456789abcdef0123456789abcdef")
	other := newIdentityToken(key, "fedcba9876543210fedcba9876543210")
	if first != second {
		t.Fatal("the same nonce produced two identity tokens")
	}
	if first == other {
		t.Fatal("different nonces produced the same identity token")
	}
	if first == newIdentityToken([]byte("another-key"), "0123456789abcdef0123456789abcdef") {
		t.Fatal("different keys produced the same identity token")
	}
	if !validToken(first) {
		t.Fatalf("identity token shape = %q", first)
	}
}

func TestSignedPayloadHelperMatchesParser(t *testing.T) {
	// Keep the test helper honest: a payload signed through it must parse, so a
	// future payload-format change fails here rather than inside a join test.
	key := []byte("signed-payload-key")
	now := readTestNow
	payload := "v1|q=11|m=invite|s=invite-signed|iat=" + itoa(uint64(now.Unix())) +
		"|n=0123456789abcdef0123456789abcdef"
	raw := signedPayload(payload, key)
	if _, err := parseTicket(key, raw, now); err != nil {
		t.Fatalf("signed helper payload rejected: %v", err)
	}
}
