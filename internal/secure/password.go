package secure

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is pinned by IDN-059. Twelve rounds is roughly a quarter second;
// it is the cost the whole login path is sized around.
const BcryptCost = 12

// Password-policy outcomes. Handlers map both to copy; ErrPasswordTooLong gets
// its own sentence so a user is not told to add an uppercase letter to a
// password that can never be stored whole (IDN-058).
var (
	ErrPasswordPolicy  = errors.New("secure: password policy")
	ErrPasswordTooLong = errors.New("secure: password too long")
	ErrPasswordHash    = errors.New("secure: password hash failed")
)

// ValidatePassword enforces IDN-057 on the raw submitted bytes. It never trims:
// a leading space is a character the user typed and must be rejected, not
// silently discarded.
func ValidatePassword(plain string) error {
	if len(plain) > 72 {
		return ErrPasswordTooLong
	}
	if utf8.RuneCountInString(plain) < 8 {
		return ErrPasswordPolicy
	}
	var digit, upper bool
	for _, r := range plain {
		if unicode.IsSpace(r) {
			return ErrPasswordPolicy
		}
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case r >= 'A' && r <= 'Z':
			upper = true
		}
	}
	if !digit || !upper {
		return ErrPasswordPolicy
	}
	return nil
}

// HashPassword returns the bcrypt string stored in accounts.password_hash.
func HashPassword(plain string) (string, error) {
	sum, err := bcrypt.GenerateFromPassword([]byte(plain), BcryptCost)
	if err != nil {
		return "", ErrPasswordHash
	}
	return string(sum), nil
}

// VerifyPassword compares a submitted password with a stored bcrypt hash.
// A malformed hash is a mismatch, never an error: the caller's only decision is
// whether the credentials are good.
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// dummyHash is a real cost-12 bcrypt string used for unknown emails, so the
// login path spends the same time whether or not the account exists (IDN-060).
// It hashes a value that is not a credential and is never accepted: the
// compare it participates in always fails.
const dummyHash = "$2a$12$5X/rFt/jW7W9uoEHdCHFy.kyNYKCrjyNwTI04CctZfFV3HERDGQpC"

// SpendDummyCompare performs exactly one bcrypt comparison at the real cost.
// Call it once on the unknown-email branch of login, before returning the same
// message and status a wrong password gets.
func SpendDummyCompare(plain string) {
	_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(plain))
}

// Email outcomes, mapped to copy by IDN-056.
var (
	ErrEmailRequired = errors.New("secure: email required")
	ErrEmailInvalid  = errors.New("secure: email invalid")
)

// NormalizeEmail applies the single normalisation every email input uses
// (IDN-053): trim, reject interior space/control/non-ASCII, lowercase ASCII,
// enforce RFC widths, and require a plausible dotted domain. The stored value
// is always the normalised one; uniqueness then holds case-insensitively.
func NormalizeEmail(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrEmailRequired
	}
	if len(s) > 254 {
		return "", ErrEmailInvalid
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf || c <= 0x20 || c == 0x7f {
			return "", ErrEmailInvalid
		}
		if c >= 'A' && c <= 'Z' {
			// Lowercase into a fresh byte slice below.
			continue
		}
	}
	lower := make([]byte, len(s))
	at := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '@' {
			if at >= 0 {
				return "", ErrEmailInvalid
			}
			at = i
			lower[i] = c
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		lower[i] = c
	}
	if at <= 0 || at == len(lower)-1 {
		return "", ErrEmailInvalid
	}
	local, domain := string(lower[:at]), string(lower[at+1:])
	if len(local) > 64 {
		return "", ErrEmailInvalid
	}
	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") ||
		strings.Contains(local, "..") {
		return "", ErrEmailInvalid
	}
	if !strings.Contains(domain, ".") ||
		strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") ||
		strings.Contains(domain, "..") {
		return "", ErrEmailInvalid
	}
	return string(lower), nil
}

// NormalizeName trims a display name and rejects control characters. Names are
// not identities (R-02), so no further canonicalisation is applied.
func NormalizeName(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}
