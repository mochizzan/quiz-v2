package authoring

import (
	"database/sql"
	"time"
)

// maxDateTime is the MariaDB DATETIME ceiling.
var maxDateTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

// NextStamp computes one delete-event timestamp T_del for a set of rows that
// share unique keys. It is max(T, 1s + max(deleted_at) over the colliding key
// family) (AUT-031). false means the DATETIME ceiling would be exceeded and the
// caller must abort as an integrity fault.
func NextStamp(t time.Time, maxima []sql.NullTime) (time.Time, bool) {
	stamp := t.UTC()
	for _, max := range maxima {
		if !max.Valid {
			continue
		}
		candidate := max.Time.UTC().Add(time.Second)
		if candidate.After(stamp) {
			stamp = candidate
		}
	}
	if stamp.After(maxDateTime) {
		return time.Time{}, false
	}
	return stamp, true
}

// PlanShift returns the positive offset for the two-phase position rewrite:
// every live row moves above every existing position, then final 1..n is
// written. The offset is strictly greater than max(position)+n, and the
// resulting temporary position must fit SMALLINT UNSIGNED (AUT-036).
func PlanShift(maxPos, n int) (int, bool) {
	if n <= 0 {
		return 0, true
	}
	if maxPos < 0 {
		maxPos = 0
	}
	offset := maxPos + n + 1
	if maxPos+offset > 65535 || offset > 65535 {
		return 0, false
	}
	return offset, true
}
