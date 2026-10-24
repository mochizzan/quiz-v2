package participation

// majors.go holds every read of the majors catalogue. Writes live in
// teacherops.go; IDN owns the student-form reads. The liveness of one major
// (and its shared lock) is checked inside the transaction that writes a
// reference to it (PAR-055, PAR-098, PAR-198), so a soft delete can never
// slide under an in-flight form.

import (
	"context"
	"database/sql"
	"errors"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/store"
	"quiz-v2/internal/viewmodel"
)

// majorRow is one live majors row.
type majorRow struct {
	ID   int64
	Name string
}

// readMajors lists the live majors alphabetically for every select the
// participation surfaces render (PAR-047, PAR-110, PAR-180: name ascending
// with the column collation, tie id ascending).
func readMajors(ctx context.Context, q store.Querier) ([]viewmodel.MajorRef, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT id, name FROM majors WHERE deleted_at = ? ORDER BY name, id",
		store.LiveAt)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []viewmodel.MajorRef{}
	for rows.Next() {
		var ref viewmodel.MajorRef
		if err := rows.Scan(&ref.ID, &ref.Name); err != nil {
			return nil, internal(err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// readMajorByID loads one live major, optionally locked. Used for the shared
// lock a reference writer takes before writing major_id (PAR-198) and the
// exclusive lock rename/delete take (§9.1).
func readMajorByID(ctx context.Context, q store.Querier, id int64, lock string) (majorRow, bool, error) {
	var m majorRow
	err := q.QueryRowContext(ctx,
		"SELECT id, name FROM majors WHERE id = ? AND deleted_at = ?"+lock,
		id, store.LiveAt).Scan(&m.ID, &m.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return majorRow{}, false, nil
	}
	if err != nil {
		return majorRow{}, false, internal(err)
	}
	return m, true, nil
}

// readMajorNameByName looks up a live major by its exact (case-insensitive,
// column collation) name. It backs the duplicate-name checks of create and
// rename (PAR-110, PAR-115).
func readMajorByName(ctx context.Context, q store.Querier, name string) (majorRow, bool, error) {
	var m majorRow
	err := q.QueryRowContext(ctx,
		"SELECT id, name FROM majors WHERE name = ? AND deleted_at = ?",
		name, store.LiveAt).Scan(&m.ID, &m.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return majorRow{}, false, nil
	}
	if err != nil {
		return majorRow{}, false, internal(err)
	}
	return m, true, nil
}

// majorTakenBy is the field-error branch of a duplicate name: the existing id
// is the winner's row (PAR-110). self is the id a rename must ignore or 0.
func majorTaken(ctx context.Context, q store.Querier, name string, self int64) error {
	m, ok, err := readMajorByName(ctx, q, name)
	if err != nil {
		return err
	}
	if ok && m.ID != self {
		return apperr.Field("name", ErrMajorDup)
	}
	return nil
}

// majorReferenceCounts answers the three live-reference counts of PAR-121 in
// one pass each: students, anonymous identities, and quiz targets (any state,
// drafts included; a closed quiz pins its target forever, §OQ-12).
func majorReferenceCounts(ctx context.Context, q store.Querier, majorID int64, idInPlaceholders string) (students, anons, quizzes int, err error) {
	var n int
	if err := q.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM students WHERE major_id = ? AND deleted_at = ?",
		majorID, store.LiveAt).Scan(&n); err != nil {
		return 0, 0, 0, internal(err)
	}
	students = n
	if err := q.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM anonymous_participants WHERE major_id = ? AND deleted_at = ?",
		majorID, store.LiveAt).Scan(&n); err != nil {
		return 0, 0, 0, internal(err)
	}
	anons = n
	// idInPlace placeholders string is emitted by the caller only for tests
	// that reuse the count; production counts one major.
	if err := q.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM quizzes WHERE target_major_id = ? AND deleted_at = ?",
		majorID, store.LiveAt).Scan(&n); err != nil {
		return 0, 0, 0, internal(err)
	}
	quizzes = n
	return students, anons, quizzes, nil
}
