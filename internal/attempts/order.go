package attempts

import (
	"bytes"
	"crypto/sha256"
	"sort"
	"strconv"
)

// order.go implements DerivedQuestionOrder and DerivedOptionOrder
// (ATT-112, ATT-113): key(seed, ns, id) = SHA-256 over
// ascii(decimal(seed)) | "|" | ns | "|" | ascii(decimal(id)), the 32 digest
// bytes compared as unsigned lexicographic bytes, ties by id ascending. The
// function is total and pure: it never mutates its inputs and never reads the
// database (ATT-114). With randomization off the passed order is kept; the
// caller supplies the rows pre-sorted by position, then id.

const (
	// namespaceQuestion prefixes the question-order keys (ns = "q").
	namespaceQuestion = "q"
	// namespaceOption prefixes the option-order keys (ns = "o").
	namespaceOption = "o"
)

// KeyOrder returns the derived shuffle key of one item id (ATT-112). decimal
// is base-10 with no sign and no leading zeros, so 0 renders "0" and an id is
// never negative here.
func KeyOrder(seed uint32, ns string, id int64) [32]byte {
	var b bytes.Buffer
	b.WriteString(strconv.FormatUint(uint64(seed), 10))
	b.WriteByte('|')
	b.WriteString(ns)
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(id, 10))
	return sha256.Sum256(b.Bytes())
}

// lessOrder compares two digest/id pairs: digest lexicographically, ties by
// id ascending (ATT-112).
func lessOrder(a [32]byte, aID int64, b [32]byte, bID int64) bool {
	if a != b {
		for i := range a {
			if a[i] != b[i] {
				return a[i] < b[i]
			}
		}
	}
	return aID < bID
}

// deriveOrder orders items by the derived key when randomization is on; the
// relative order of equal keys is its own id, and the base order (position asc,
// id asc) is preserved as the tie of last resort when randomization is off,
// because the digest is equal only for the same id.
func deriveOrder[T any](items []T, seed uint32, random bool, idOf func(T) int64) []T {
	out := make([]T, len(items))
	copy(out, items)
	if !random {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := KeyOrder(seed, namespaceQuestion, idOf(out[i])), KeyOrder(seed, namespaceQuestion, idOf(out[j]))
		return lessOrder(a, idOf(out[i]), b, idOf(out[j]))
	})
	return out
}

// deriveOptionOrder is deriveOrder with the option namespace.
func deriveOptionOrder[T any](items []T, seed uint32, random bool, idOf func(T) int64) []T {
	out := make([]T, len(items))
	copy(out, items)
	if !random {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := KeyOrder(seed, namespaceOption, idOf(out[i])), KeyOrder(seed, namespaceOption, idOf(out[j]))
		return lessOrder(a, idOf(out[i]), b, idOf(out[j]))
	})
	return out
}

// derivedIndex returns the 1-based display position of one id inside the
// ordered slice. A 0 return means the id is not part of the attempt, which the
// callers treat as their own not-found path (never a renumbering, ATT-254).
func indexOfID(orderedIDs []int64, id int64) int {
	for i, v := range orderedIDs {
		if v == id {
			return i + 1
		}
	}
	return 0
}
