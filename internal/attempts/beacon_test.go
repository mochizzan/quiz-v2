package attempts

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/storetest"
	"quiz-v2/internal/viewmodel"
)

func TestParseBeacon(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		contentType string
		body        string
		want        BeaconKind
		ok          bool
	}{
		{"plain", "text/plain", "progress", BeaconProgress, true},
		{"plain trimmed", "text/plain; charset=utf-8", "  blur\n", BeaconBlur, true},
		{"plain hidden", "text/plain", "hidden", BeaconHidden, true},
		{"plain fullscreen", "text/plain", "fullscreen_exit", BeaconFullscreenExit, true},
		{"plain unknown", "text/plain", "idle", "", false},
		{"plain empty", "text/plain", "  ", "", false},
		{"json", "application/json", `{"kind":"hidden"}`, BeaconHidden, true},
		{"json extra", "application/json; charset=utf-8", `{"kind":"blur","at":"2001-01-01T00:00:00Z"}`, BeaconBlur, true},
		{"json missing", "application/json", `{}`, "", false},
		{"json unknown", "application/json", `{"kind":"idle"}`, "", false},
		{"json wrong type", "application/json", `{"kind":7}`, "", false},
		{"json malformed", "application/json", `{"kind":`, "", false},
		{"json null", "application/json", `null`, "", false},
		{"form", "application/x-www-form-urlencoded", "kind=blur", "", false},
		{"missing type", "", "blur", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseBeacon(tt.contentType, []byte(tt.body))
			if ok != tt.ok || got != tt.want {
				t.Fatalf("ParseBeacon(%q, %q) = (%q, %v), want (%q, %v)",
					tt.contentType, tt.body, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestBeaconKindBuckets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind   BeaconKind
		bucket string
		limit  int
		ok     bool
	}{
		{BeaconProgress, "progress", BeaconProgressLimit, true},
		{BeaconBlur, "focus", BeaconFocusLimit, true},
		{BeaconHidden, "focus", BeaconFocusLimit, true},
		{BeaconFullscreenExit, "focus", BeaconFocusLimit, true},
		{BeaconKind("idle"), "", 0, false},
	}
	for _, tt := range tests {
		bucket, limit, ok := tt.kind.bucket()
		if ok != tt.ok || bucket != tt.bucket || limit != tt.limit {
			t.Fatalf("%q bucket = (%q, %d, %v), want (%q, %d, %v)",
				tt.kind, bucket, limit, ok, tt.bucket, tt.limit, tt.ok)
		}
	}
}

func TestBeaconWritesProgressAndFocus(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedBeaconWorld(t, db)
	ctx := context.Background()
	svc := New(db).WithLimiter(secure.NewLimiter(nil))

	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	mustExec(t, db, `UPDATE quiz_attempts SET updated_at = ? WHERE id = ?`, old, w.attemptID)

	res, err := svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconProgress})
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	if res.Noop {
		t.Fatal("live progress was a no-op")
	}
	var updated time.Time
	if err := db.QueryRowContext(ctx, `SELECT updated_at FROM quiz_attempts WHERE id = ?`, w.attemptID).Scan(&updated); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	if !updated.After(old) {
		t.Fatalf("progress did not touch updated_at: got %v", updated)
	}

	res, err = svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconBlur})
	if err != nil {
		t.Fatalf("focus: %v", err)
	}
	if res.Noop {
		t.Fatal("live focus was a no-op")
	}
	var (
		count      int
		kind       string
		occurredAt time.Time
	)
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), MIN(kind), MIN(occurred_at) FROM focus_loss_events WHERE attempt_id = ?`,
		w.attemptID).Scan(&count, &kind, &occurredAt); err != nil {
		t.Fatalf("read focus rows: %v", err)
	}
	if count != 1 || kind != string(BeaconBlur) {
		t.Fatalf("focus rows = (%d, %q), want (1, %q)", count, kind, BeaconBlur)
	}
	if time.Since(occurredAt.UTC()) > 2*time.Minute {
		t.Fatalf("occurred_at is not server-now: %v", occurredAt)
	}

	// The beacon is informational: it never submits or scores the attempt.
	var submitted sql.NullTime
	var final sql.NullFloat64
	if err := db.QueryRowContext(ctx,
		`SELECT submitted_at, final_score FROM quiz_attempts WHERE id = ?`, w.attemptID).
		Scan(&submitted, &final); err != nil {
		t.Fatalf("read attempt: %v", err)
	}
	if submitted.Valid || final.Valid {
		t.Fatalf("beacon changed lifecycle: submitted=%v final=%v", submitted.Valid, final.Valid)
	}
}

func TestBeaconOwnership(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedBeaconWorld(t, db)
	ctx := context.Background()
	svc := New(db).WithLimiter(secure.NewLimiter(nil))

	tests := []struct {
		name  string
		id    uint32
		owner OwnerRef
	}{
		{"unknown attempt", 999999, w.studentOwner},
		{"foreign student", w.attemptID, w.foreignOwner},
		{"teacher", w.attemptID, OwnerRef{Role: viewmodel.RoleTeacher, AccountID: w.teacherAccountID}},
		{"visitor", w.attemptID, OwnerRef{}},
		{"wrong identity token", w.attemptID, OwnerRef{IdentityToken: "v1." + strings.Repeat("Z", 43)}},
		{"anonymous on student attempt", w.attemptID, w.anonymousOwner},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Beacon(ctx, BeaconInput{AttemptID: tt.id, Owner: tt.owner, Kind: BeaconBlur})
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("Beacon error = %v, want ErrNotFound", err)
			}
		})
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM focus_loss_events WHERE attempt_id = ?`, w.attemptID).Scan(&count); err != nil {
		t.Fatalf("count focus: %v", err)
	}
	if count != 0 {
		t.Fatalf("foreign beacons wrote %d focus rows", count)
	}
}

func TestBeaconAfterSubmitNoop(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedBeaconWorld(t, db)
	ctx := context.Background()

	mustExec(t, db,
		`UPDATE quiz_attempts SET submitted_at = UTC_TIMESTAMP(), final_score = 0.7500 WHERE id = ?`,
		w.attemptID)
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	mustExec(t, db, `UPDATE quiz_attempts SET updated_at = ? WHERE id = ?`, old, w.attemptID)

	// Fill both buckets. A submitted attempt must be a 204 no-op even when the
	// limiter is exhausted; otherwise a queued beacon could consume budget
	// after submit (ATT-160).
	fixed := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	limiter := secure.NewLimiter(func() time.Time { return fixed })
	for i := 0; i < BeaconProgressLimit; i++ {
		if ok, _ := limiter.Allow(testBeaconKey(w.attemptID, "progress"), BeaconProgressLimit, BeaconWindow); !ok {
			t.Fatalf("prefill progress %d rejected", i+1)
		}
	}
	for i := 0; i < BeaconFocusLimit; i++ {
		if ok, _ := limiter.Allow(testBeaconKey(w.attemptID, "focus"), BeaconFocusLimit, BeaconWindow); !ok {
			t.Fatalf("prefill focus %d rejected", i+1)
		}
	}
	svc := New(db).WithLimiter(limiter)

	res, err := svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconProgress})
	if err != nil {
		t.Fatalf("submitted progress returned %v, want no-op", err)
	}
	if !res.Noop {
		t.Fatal("submitted progress was not marked no-op")
	}
	res, err = svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconHidden})
	if err != nil {
		t.Fatalf("submitted focus returned %v, want no-op", err)
	}
	if !res.Noop {
		t.Fatal("submitted focus was not marked no-op")
	}

	var (
		updated time.Time
		count   int
	)
	if err := db.QueryRowContext(ctx, `SELECT updated_at FROM quiz_attempts WHERE id = ?`, w.attemptID).Scan(&updated); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	if !updated.Equal(old) {
		t.Fatalf("submitted progress touched updated_at: got %v", updated)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM focus_loss_events WHERE attempt_id = ?`, w.attemptID).Scan(&count); err != nil {
		t.Fatalf("count focus: %v", err)
	}
	if count != 0 {
		t.Fatalf("submitted focus wrote %d rows", count)
	}
}

func TestBeaconRateLimits(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedBeaconWorld(t, db)
	ctx := context.Background()
	fixed := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	svc := New(db).WithLimiter(secure.NewLimiter(func() time.Time { return fixed }))

	for i := 0; i < BeaconProgressLimit; i++ {
		if _, err := svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconProgress}); err != nil {
			t.Fatalf("progress %d/%d: %v", i+1, BeaconProgressLimit, err)
		}
	}
	_, err := svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconProgress})
	assertRateLimited(t, err)

	for i := 0; i < BeaconFocusLimit; i++ {
		if _, err := svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconBlur}); err != nil {
			t.Fatalf("focus %d/%d: %v", i+1, BeaconFocusLimit, err)
		}
	}
	// All three focus kinds share one bucket.
	_, err = svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconFullscreenExit})
	assertRateLimited(t, err)

	// A second attempt has its own bucket and is unaffected by the first.
	if _, err := svc.Beacon(ctx, BeaconInput{AttemptID: w.secondAttemptID, Owner: w.studentOwner, Kind: BeaconProgress}); err != nil {
		t.Fatalf("second attempt progress: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM focus_loss_events WHERE attempt_id = ?`, w.attemptID).Scan(&count); err != nil {
		t.Fatalf("count focus: %v", err)
	}
	if count != BeaconFocusLimit {
		t.Fatalf("focus rows = %d, want %d", count, BeaconFocusLimit)
	}
}

func TestBeaconClosedQuizAccepted(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedBeaconWorld(t, db)
	ctx := context.Background()
	mustExec(t, db, `UPDATE quizzes SET state = 'closed', closed_at = UTC_TIMESTAMP() WHERE id = ?`, w.quizID)
	svc := New(db).WithLimiter(secure.NewLimiter(nil))

	if _, err := svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconProgress}); err != nil {
		t.Fatalf("closed-quiz progress: %v", err)
	}
	if _, err := svc.Beacon(ctx, BeaconInput{AttemptID: w.attemptID, Owner: w.studentOwner, Kind: BeaconBlur}); err != nil {
		t.Fatalf("closed-quiz focus: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM focus_loss_events WHERE attempt_id = ?`, w.attemptID).Scan(&count); err != nil {
		t.Fatalf("count focus: %v", err)
	}
	if count != 1 {
		t.Fatalf("closed-quiz focus rows = %d, want 1", count)
	}
}

func TestBeaconAnonymousOwner(t *testing.T) {
	db := storetest.DB(t)
	storetest.Reset(t, db)
	w := seedBeaconWorld(t, db)
	ctx := context.Background()
	svc := New(db).WithLimiter(secure.NewLimiter(nil))

	if _, err := svc.Beacon(ctx, BeaconInput{AttemptID: w.anonymousAttemptID, Owner: w.anonymousOwner, Kind: BeaconHidden}); err != nil {
		t.Fatalf("anonymous focus: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM focus_loss_events WHERE attempt_id = ?`, w.anonymousAttemptID).Scan(&count); err != nil {
		t.Fatalf("count focus: %v", err)
	}
	if count != 1 {
		t.Fatalf("anonymous focus rows = %d, want 1", count)
	}
}

func assertRateLimited(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind() != apperr.RateLimited {
		t.Fatalf("error = %v, want apperr.RateLimited", err)
	}
	if appErr.RetryAfter() != 60 {
		t.Fatalf("RetryAfter = %d, want 60", appErr.RetryAfter())
	}
}

func testBeaconKey(attemptID uint32, bucket string) string {
	return "beacon:" + strconv.FormatUint(uint64(attemptID), 10) + ":" + bucket
}

type beaconWorld struct {
	attemptID          uint32
	secondAttemptID    uint32
	anonymousAttemptID uint32
	quizID             uint32
	teacherAccountID   int64
	studentOwner       OwnerRef
	foreignOwner       OwnerRef
	anonymousOwner     OwnerRef
}

func seedBeaconWorld(t *testing.T, db *store.DB) beaconWorld {
	t.Helper()
	teacherID := insertAccount(t, db, "Guru", "guru@beacon.test", viewmodel.RoleTeacher)
	majorID := insertID(t, db, `INSERT INTO majors (name, code) VALUES ('Beacon Major', 'BMC')`)

	studentAccountID := insertAccount(t, db, "Siswa", "siswa@beacon.test", viewmodel.RoleStudent)
	studentID := insertID(t, db,
		`INSERT INTO students (account_id, grade_level, major_id) VALUES (?, 10, ?)`,
		studentAccountID, majorID)
	foreignAccountID := insertAccount(t, db, "Siswa Lain", "lain@beacon.test", viewmodel.RoleStudent)
	insertID(t, db,
		`INSERT INTO students (account_id, grade_level, major_id) VALUES (?, 11, ?)`,
		foreignAccountID, majorID)

	quizID := insertID(t, db,
		`INSERT INTO quizzes (owner_account_id, title, visibility, state, code, invite_token, published_at)
		 VALUES (?, 'Kuis Beacon', 'public', 'open', '123456', 'beacon-invite-token', UTC_TIMESTAMP())`,
		teacherID)
	registrationID := insertID(t, db,
		`INSERT INTO registrations (quiz_id, student_id) VALUES (?, ?)`,
		quizID, studentID)
	attemptID := insertID(t, db,
		`INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed) VALUES (?, 1, 42)`,
		registrationID)
	secondAttemptID := insertID(t, db,
		`INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed) VALUES (?, 2, 43)`,
		registrationID)

	token := "v1." + strings.Repeat("A", 43)
	anonID := insertID(t, db,
		`INSERT INTO anonymous_participants (display_name, grade_level, major_id, cookie_token_hash)
		 VALUES ('Peserta', 10, ?, ?)`,
		majorID, secure.HashToken(token))
	anonRegistrationID := insertID(t, db,
		`INSERT INTO registrations (quiz_id, anonymous_participant_id) VALUES (?, ?)`,
		quizID, anonID)
	anonAttemptID := insertID(t, db,
		`INSERT INTO quiz_attempts (registration_id, attempt_no, shuffle_seed) VALUES (?, 1, 44)`,
		anonRegistrationID)

	return beaconWorld{
		attemptID:          uint32(attemptID),
		secondAttemptID:    uint32(secondAttemptID),
		anonymousAttemptID: uint32(anonAttemptID),
		quizID:             uint32(quizID),
		teacherAccountID:   teacherID,
		studentOwner:       OwnerRef{Role: viewmodel.RoleStudent, AccountID: studentAccountID},
		foreignOwner:       OwnerRef{Role: viewmodel.RoleStudent, AccountID: foreignAccountID},
		anonymousOwner:     OwnerRef{IdentityToken: token},
	}
}

func insertAccount(t *testing.T, db *store.DB, name, email string, role viewmodel.Role) int64 {
	t.Helper()
	var approved any
	if role == viewmodel.RoleTeacher {
		approved = time.Now().UTC()
	}
	return insertID(t, db,
		`INSERT INTO accounts (full_name, email, password_hash, role, status, approved_at, must_change_password)
		 VALUES (?, ?, 'test', ?, 'active', ?, 0)`,
		name, email, string(role), approved)
}

func insertID(t *testing.T, db *store.DB, query string, args ...any) int64 {
	t.Helper()
	res, err := db.Exec(query, args...)
	if err != nil {
		t.Fatalf("seed: %v\nquery: %s", err, query)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seed last id: %v", err)
	}
	return id
}

func mustExec(t *testing.T, db *store.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec: %v\nquery: %s", err, query)
	}
}
