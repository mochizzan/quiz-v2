package participation

// token.go owns the two bearer tokens PAR issues (§5.1, §5.2). Both are opaque
// ASCII values the browser stores; only their SHA-256 or their signature ever
// reaches the server's storage. Neither ever appears in a URL, a log line, or
// HTML (PAR-004, 8.4.8).
//
// Rules carried here:
//   - identity cookie: PAR-001–006 (shape, attributes are applied by the HTTP
//     adapter, hash lookup lives in actor.go);
//   - join ticket: PAR-013–020 (mint, parse, window, identity derivation).

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Cookie names, paths and lifetimes are fixed by PAR-002 and PAR-014.
const (
	// IdentityCookie holds the anonymous participant token.
	IdentityCookie = "quiz_participant"
	// TicketCookie holds the short-lived join ticket.
	TicketCookie = "quiz_join_ticket"

	// IdentityPath is the identity cookie's scope: the whole participant area.
	IdentityPath = "/"
	// TicketPath is the ticket's scope: the identify screens only.
	TicketPath = "/join"

	// IdentityMaxAge is 365 days (PAR-002). It slides on every successful
	// identify so a returning retaker is never logged out.
	IdentityMaxAge = 31536000
	// TicketMaxAge is the ticket's 30-minute window (PAR-014).
	TicketMaxAge = 1800
)

// ticketMethod marks how a ticket was earned (PAR-015 `m=`).
const (
	MethodCode   = "code"
	MethodInvite = "invite"
)

const (
	// identityPrefix marks a v1 identity token.
	identityPrefix = "v1."
	// identityContext separates PAR's key use from every other signer.
	identityContext = "participant|"
	// ticketContext separates the ticket signature from identity tokens.
	ticketContext = "join-ticket|"

	// ticketTTL is how long a ticket is redeemable (PAR-014, PAR-016).
	ticketTTL = 30 * time.Minute
	// ticketSkew tolerates at most 60 seconds of signed-in-the-future issue
	// time so a small server clock step does not strand a form.
	ticketSkew = 60 * time.Second
)

// tokenShape accepts exactly "v1." plus 43 unpadded base64url characters: the
// encoding of 32 bytes (PAR-001). Anything else is a visitor, never an error
// and never a fuzzing oracle (PAR-006).
var tokenShape = regexp.MustCompile(`^v1\.[A-Za-z0-9_-]{43}$`)

// newNonce returns 128 crypto-random bits as 32 lowercase hex characters
// (PAR-015). The nonce is part of the ticket payload and seeds the derived
// identity token (PAR-019).
func newNonce() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// newIdentityToken derives the identity token from a ticket nonce (PAR-019).
// HMAC-SHA256 output is exactly 32 bytes, so the rule's truncation is a no-op
// and the value matches the 43-character shape. Two submissions of the same
// ticket compute the same token, which is what makes a double submit idempotent
// through uq_anon_cookie (PAR-060, 9.2.2).
func newIdentityToken(key []byte, nonce string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(identityContext + nonce))
	return identityPrefix + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// validToken reports whether raw has the exact cookie shape (PAR-005). No
// trimming, no case folding, no prefix fallback: an altered value must not
// merely lose the tail lookup cheaply (PAR-003).
func validToken(raw string) bool { return tokenShape.MatchString(raw) }

// ticket is a parsed join ticket.
type ticket struct {
	QuizID uint32
	Method string
	Secret string
	Nonce  string
	Issued time.Time
}

// mintTicket produces the cookie value for one resolution (PAR-015). now is
// the app clock: a token window is not a storage stamp, so it does not use
// UTC_TIMESTAMP (compare OPS-103: the DB clock owns row writes only).
func mintTicket(key []byte, quizID uint32, method, secret string, now time.Time) (string, error) {
	if err := validTicketSecret(secret); err != nil {
		return "", err
	}
	nonce, err := newNonce()
	if err != nil {
		return "", err
	}
	payload := strings.Join([]string{
		"v1",
		"q=" + strconv.FormatUint(uint64(quizID), 10),
		"m=" + method,
		"s=" + secret,
		"iat=" + strconv.FormatInt(now.Unix(), 10),
		"n=" + nonce,
	}, "|")
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ticketContext + payload))
	return identityPrefix + body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// validTicketSecret rejects a secret that would carry the payload's field
// separator (PAR-029, PAR-037). Real codes are six digits and invite tokens are
// [A-Za-z0-9_-], so rejection is the honest answer for anything else.
func validTicketSecret(secret string) error {
	switch methodOf(secret) {
	case MethodCode, MethodInvite:
		return nil
	default:
		return errors.New("participation: ticket secret is not an entry capability shape")
	}
}

// methodOf keeps secret classification next to its one caller.
func methodOf(secret string) string {
	if validCode(secret) {
		return MethodCode
	}
	if validInvite(secret) {
		return MethodInvite
	}
	return ""
}

// validSecret wraps ticketSecretValue's error as a boolean for parse paths.
func validSecret(secret string) bool { return methodOf(secret) != "" }

// parseTicket verifies one cookie value: shape, signature, age, and field
// syntax (PAR-016). Every failure is one error; the caller answers 404 and
// never distinguishes forged from expired (§3.5).
func parseTicket(key []byte, value string, now time.Time) (ticket, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "v1" || parts[1] == "" || parts[2] == "" {
		return ticket{}, errors.New("participation: ticket shape")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payload) == 0 {
		return ticket{}, errors.New("participation: ticket payload encoding")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return ticket{}, errors.New("participation: ticket signature encoding")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(append([]byte(ticketContext), payload...))
	if subtle.ConstantTimeCompare(sig, mac.Sum(nil)) != 1 {
		return ticket{}, errors.New("participation: ticket signature")
	}
	return parseTicketPayload(string(payload), now)
}

// parseTicketPayload reads the ASCII fields. iat is checked against the server
// clock only, so a client clock change cannot extend the window (PAR-020).
func parseTicketPayload(payload string, now time.Time) (ticket, error) {
	chunks := strings.Split(payload, "|")
	if len(chunks) == 0 || chunks[0] != "v1" {
		return ticket{}, errors.New("participation: ticket version marker")
	}
	fields := map[string]string{}
	for _, chunk := range chunks[1:] {
		key, value, ok := strings.Cut(chunk, "=")
		if !ok || key == "" {
			return ticket{}, errors.New("participation: ticket field")
		}
		if _, dup := fields[key]; dup {
			return ticket{}, errors.New("participation: ticket field")
		}
		fields[key] = value
	}
	if len(fields) != 5 {
		return ticket{}, errors.New("participation: ticket fields")
	}
	q, err := strconv.ParseUint(fields["q"], 10, 32)
	if err != nil || q == 0 {
		return ticket{}, errors.New("participation: ticket quiz")
	}
	if _, err := strconv.ParseInt(fields["iat"], 10, 64); err != nil {
		return ticket{}, errors.New("participation: ticket issue time")
	}
	issued := unix(fields["iat"])
	// The window is server-side only (PAR-016, PAR-020): a well-signed ticket
	// older than 30 minutes is expired, and one signed ahead by more than the
	// skew is rejected outright.
	switch {
	case issued.After(now.Add(ticketSkew)):
		return ticket{}, errors.New("participation: ticket from the future")
	case issued.Before(now.Add(-ticketTTL)):
		return ticket{}, errors.New("participation: ticket expired")
	}
	if len(fields["n"]) != 32 {
		return ticket{}, errors.New("participation: ticket nonce")
	}
	for _, c := range fields["n"] {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return ticket{}, errors.New("participation: ticket nonce")
		}
	}
	method := fields["m"]
	if method != MethodCode && method != MethodInvite {
		return ticket{}, errors.New("participation: ticket method")
	}
	if method == MethodCode && !validCode(fields["s"]) ||
		method == MethodInvite && !validInvite(fields["s"]) {
		return ticket{}, errors.New("participation: ticket secret")
	}
	return ticket{
		QuizID: uint32(q),
		Method: method,
		Secret: fields["s"],
		Nonce:  fields["n"],
		Issued: issued,
	}, nil
}

// unix re-parses the wall value already digit-checked by the caller.
func unix(seconds string) time.Time {
	n, _ := strconv.ParseInt(seconds, 10, 64)
	return time.Unix(n, 0).UTC()
}

// ticketQuery is the canonical identify target (PAR-033, PAR-042).
func ticketQuery(quizID uint32) string {
	v := url.Values{}
	v.Set("quiz", strconv.FormatUint(uint64(quizID), 10))
	return v.Encode()
}
