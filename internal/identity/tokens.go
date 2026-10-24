package identity

import (
	"encoding/binary"
	"encoding/json"
	"time"

	"quiz-v2/internal/secure"
)

// Cookie names owned by this domain (IDN-067). They are distinct from the
// session and CSRF cookies so a purpose can never be replayed as another.
const (
	// CookiePending carries the 24-hour teacher-registration token.
	CookiePending = "quiz_pending"
	// CookieReveal carries the one-shot reset-token panel payload.
	CookieReveal = "quiz_reset_reveal"
)

// Cookie lifetimes in seconds. The session cookie deliberately has none.
const (
	// PendingCookieMaxAge is R-23's 24 hours.
	PendingCookieMaxAge = 86400
	// RevealCookieMaxAge is IDN-105's five minutes.
	RevealCookieMaxAge = 300
)

// NewPendingToken signs the account id with the shared signer. The payload is
// eight big-endian bytes so it can never be confused with a JSON payload from
// another purpose (R-23, IDN-088).
func NewPendingToken(signer *secure.Signer, accountID int64, now time.Time) (string, error) {
	var payload [8]byte
	binary.BigEndian.PutUint64(payload[:], uint64(accountID))
	return signer.Token(pendingTokenPurpose, payload[:], now)
}

// ParsePendingToken verifies signature, purpose, issue time, and the 24-hour
// age, then returns the account id. Every failure is one opaque error; the
// pending page renders the same `expired` state for all of them (IDN-C12).
func ParsePendingToken(signer *secure.Signer, token string, now time.Time) (int64, error) {
	data, err := signer.ParseToken(pendingTokenPurpose, token, PendingTokenTTL, now)
	if err != nil {
		return 0, err
	}
	if len(data) != 8 {
		return 0, secure.ErrTokenInvalid
	}
	id := int64(binary.BigEndian.Uint64(data))
	if id <= 0 {
		return 0, secure.ErrTokenInvalid
	}
	return id, nil
}

// Reveal is the signed payload of the one-shot reset-token panel (IDN-105).
// It is JSON because it carries the raw token the teacher must copy.
type Reveal struct {
	Purpose   string `json:"purpose"`
	Teacher   int64  `json:"teacher_account_id"`
	Student   int64  `json:"student_id"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// NewRevealToken signs a reveal payload with the reset-reveal purpose.
func NewRevealToken(signer *secure.Signer, r Reveal, now time.Time) (string, error) {
	r.Purpose = revealTokenPurpose
	payload, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return signer.Token(revealTokenPurpose, payload, now)
}

// ParseRevealToken verifies the signature, purpose, and five-minute age, then
// decodes the payload. A malformed or foreign cookie is one opaque error; the
// list suppresses the panel and clears the cookie without an oracle (IDN-105).
func ParseRevealToken(signer *secure.Signer, token string, now time.Time) (*Reveal, error) {
	data, err := signer.ParseToken(revealTokenPurpose, token, RevealTokenTTL, now)
	if err != nil {
		return nil, err
	}
	var r Reveal
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, secure.ErrTokenInvalid
	}
	if r.Purpose != revealTokenPurpose || r.Teacher <= 0 || r.Student <= 0 ||
		r.Token == "" || r.ExpiresAt <= 0 {
		return nil, secure.ErrTokenInvalid
	}
	return &r, nil
}
