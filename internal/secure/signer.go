// Package secure holds the credential primitives every domain shares: random
// tokens and their hashes, the signed CSRF double-submit token, the HMAC token
// used for pending-teacher and join links, password hashing and policy, email
// normalisation, and the in-process rate limiter.
//
// Nothing here touches the database or HTTP. The rules are business-logic spec
// IDN-049 (CSRF), IDN-050 (key), IDN-053 (email), IDN-057…IDN-060 (password),
// IDN-062…IDN-065 (rate limits), and R-23 (pending token).
package secure

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// TokenBytes is the raw entropy of every bearer token: session, reset,
// pending, join. 32 bytes is the schema's BINARY(32) hash width.
const TokenBytes = 32

// MinKeyBytes is the minimum SESSION_SIGNING_KEY length (IDN-050).
const MinKeyBytes = 32

// ErrKeyTooShort is returned by NewSigner for a key shorter than 32 bytes.
var ErrKeyTooShort = errors.New("secure: signing key must be at least 32 bytes")

// ErrTokenInvalid covers a malformed, mis-signed, or wrong-purpose token.
var ErrTokenInvalid = errors.New("secure: token invalid")

// ErrTokenExpired covers a well-formed token past its window.
var ErrTokenExpired = errors.New("secure: token expired")

// NewToken returns 32 cryptographically random bytes, base64url encoded
// without padding (43 characters). Only its SHA-256 is ever stored.
func NewToken() (string, error) {
	b := make([]byte, TokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the raw SHA-256 of a token, ready for a BINARY(32) column.
// The token itself is never stored; a database read cannot reconstruct it.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Equal compares two strings in constant time. A length difference is not
// secret, so the length check before the compare leaks nothing useful.
func Equal(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Signer signs the CSRF cookie and every out-of-band token. One key
// (SESSION_SIGNING_KEY) serves all of them; the purpose string separates the
// domains so a pending token can never be replayed as a join ticket.
type Signer struct {
	key []byte
}

// NewSigner builds a signer. A key shorter than 32 bytes is rejected rather
// than stretched: a short key is a misconfiguration, and a process-random
// fallback would silently invalidate every open form on restart (IDN-050).
func NewSigner(key []byte) (*Signer, error) {
	if len(key) < MinKeyBytes {
		return nil, ErrKeyTooShort
	}
	k := make([]byte, len(key))
	copy(k, key)
	return &Signer{key: k}, nil
}

// mac computes base64url(HMAC-SHA256(key, context)).
func (s *Signer) mac(context string) string {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(context))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// CSRF returns a fresh CSRF token: a random nonce and its signature. The whole
// value is both the cookie value and the form field value.
func (s *Signer) CSRF() (string, error) {
	nonce, err := NewToken()
	if err != nil {
		return "", err
	}
	return nonce + "." + s.mac(csrfContext+nonce), nil
}

// csrfContext is the domain separator for CSRF signatures.
const csrfContext = "csrf.v1|"

// VerifyCSRF reports whether a cookie value is well formed and correctly
// signed. It does not compare it to a submitted field; use MatchCSRF for that.
func (s *Signer) VerifyCSRF(value string) bool {
	nonce, sig, ok := splitOnce(value)
	if !ok {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(nonce)
	if err != nil || len(raw) != TokenBytes {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(s.mac(csrfContext+nonce)))
}

// MatchCSRF verifies the cookie value and then compares it in constant time to
// the submitted form value. Both checks are required: the signature proves the
// cookie was minted here, the comparison proves the form saw the same token.
func (s *Signer) MatchCSRF(cookie, form string) bool {
	return s.VerifyCSRF(cookie) && Equal(cookie, form)
}

// tokenPayload is the wire shape of an out-of-band token. Data is a short
// opaque blob (an account id, a ticket); it is never user-visible text.
type tokenPayload struct {
	Purpose  string `json:"p"`
	IssuedAt int64  `json:"iat"`
	Data     []byte `json:"d,omitempty"`
}

// tokenContext is the domain separator for out-of-band tokens.
const tokenContext = "tok.v1|"

// Token signs a purpose-bound payload with its issue time. maxAge is enforced
// on parse, not here, so the issuer does not silently choose the reader's rule.
func (s *Signer) Token(purpose string, data []byte, issuedAt time.Time) (string, error) {
	body, err := json.Marshal(tokenPayload{Purpose: purpose, IssuedAt: issuedAt.Unix(), Data: data})
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	return encoded + "." + s.mac(tokenContext+purpose+"|"+encoded), nil
}

// ParseToken verifies a token's signature, purpose, and age. An invalid
// signature or purpose is ErrTokenInvalid; an old token is ErrTokenExpired.
// now is a parameter so the window is testable without sleeping.
func (s *Signer) ParseToken(purpose, token string, maxAge time.Duration, now time.Time) ([]byte, error) {
	encoded, sig, ok := splitOnce(token)
	if !ok {
		return nil, ErrTokenInvalid
	}
	if !hmac.Equal([]byte(sig), []byte(s.mac(tokenContext+purpose+"|"+encoded))) {
		return nil, ErrTokenInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var p tokenPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, ErrTokenInvalid
	}
	if p.Purpose != purpose {
		return nil, ErrTokenInvalid
	}
	issued := time.Unix(p.IssuedAt, 0).UTC()
	if issued.After(now) {
		return nil, ErrTokenInvalid
	}
	if now.Sub(issued) >= maxAge {
		return nil, ErrTokenExpired
	}
	return p.Data, nil
}

// splitOnce splits on exactly one dot. Zero dots is malformed; more than one is
// malformed too, so a signature cannot smuggle a second separator.
func splitOnce(v string) (head, tail string, ok bool) {
	i := strings.IndexByte(v, '.')
	if i <= 0 || i == len(v)-1 {
		return "", "", false
	}
	if strings.IndexByte(v[i+1:], '.') >= 0 {
		return "", "", false
	}
	return v[:i], v[i+1:], true
}
