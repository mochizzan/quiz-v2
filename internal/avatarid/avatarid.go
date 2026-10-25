// Package avatarid generates and checks the file names that store profile
// photos (AVT-010, AVT-070). A name is 16 bytes of crypto/rand rendered as
// lowercase hex plus the .webp suffix the server always re-encodes to. Nothing
// else — never a URL, path, MIME type, size, or client filename — may stand in
// for one, so every read of accounts.avatar_name passes through Valid before
// it becomes a URL or a file.
package avatarid

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
)

// Pattern is the one grammar a stored or requested avatar name obeys
// (AVT-070): exactly 32 lowercase hex characters and the .webp extension. It
// is exported so routes, queries and tests pin the same shape.
const Pattern = `^[a-f0-9]{32}\.webp$`

// Suffix is the extension every generated name carries: the server always
// re-encodes an upload to WebP, so no other extension can be honest (AVT-010).
const Suffix = ".webp"

// nameRe is Pattern compiled once for the validation path.
var nameRe = regexp.MustCompile(Pattern)

// Valid reports whether name is a well-formed avatar file name. A malformed
// value — an uppercase alias of a real file, a wrong length, another
// extension, or a traversal attempt — is rejected before any I/O, so a bad
// database value can never become a bad URL or a bad path (AVT-070).
func Valid(name string) bool { return nameRe.MatchString(name) }

// Generate returns a fresh avatar file name: 16 random bytes from crypto/rand,
// lowercase hex, plus .webp. A changed photo always gets a new name, so its
// URL changes and no cache-busting is needed (§4.2). The error is crypto/rand's;
// callers surface it as a failed save, never as a fallback name.
func Generate() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]) + Suffix, nil
}
