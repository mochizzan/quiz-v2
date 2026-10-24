package migrate

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"quiz-v2/internal/secure"
	"quiz-v2/internal/store"
	"quiz-v2/internal/store/testlock"
)

// testDB opens the pool integration tests share with the app. This file lives
// in package migrate, so it cannot import internal/store/storetest — that
// package imports migrate to bring an empty database current before its own
// tests. The helper is the same idea: TEST_DB_DSN decides whether a database
// exists at all.
func testDB(t *testing.T) *store.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("integration test: set TEST_DB_DSN to run")
	}
	// Serialise with every other database-using test binary: they share one
	// schema, and a migration test must not run beside a seed or a reset.
	lockCtx, lockCancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer lockCancel()
	if err := testlock.Acquire(lockCtx); err != nil {
		t.Fatalf("testlock: %v", err)
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open %s: %v", dsn, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.Ping(ctx, db); err != nil {
		t.Fatalf("ping %s: %v", dsn, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

var (
	testTeacherHashOnce sync.Once
	testTeacherHash     string
	testTeacherHashErr  error
)

// teacherHash is the bcrypt hash of a handoff password no test verifies again
// (§9.5: the runner takes a hash, never a password). Built once per package
// because cost 12 is deliberate slowness.
func teacherHash(t *testing.T) string {
	t.Helper()
	testTeacherHashOnce.Do(func() {
		testTeacherHash, testTeacherHashErr = secure.HashPassword("Guru#Sekolah2026")
	})
	if testTeacherHashErr != nil {
		t.Fatalf("built-in hash: %v", testTeacherHashErr)
	}
	return testTeacherHash
}

// emptyLedger clears the ledger so the next run replays every file, as if the
// database were empty. Operational soft-delete would re-enqueue files just as
// safely (§6.15), but it cannot stamp the same name to one timestamp twice —
// the (name, deleted_at) unique key forbids that. Tests may hard-delete: the
// ledger's append-only rule is an operations rule, not a test one.
func emptyLedger(t *testing.T, db *store.DB) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		"DELETE FROM schema_migrations"); err != nil {
		t.Fatalf("empty ledger: %v", err)
	}
}

// teacherRow reads the bootstrapped account back: one row, the right shape.
func teacherRow(t *testing.T, db *store.DB) (role, status string, mustChange int, approved, deleted any) {
	t.Helper()
	if err := db.QueryRowContext(context.Background(),
		"SELECT role, status, must_change_password, approved_at, deleted_at FROM accounts WHERE email = 'guru@sekolah.sch.id'").
		Scan(&role, &status, &mustChange, &approved, &deleted); err != nil {
		t.Fatalf("read teacher: %v", err)
	}
	return role, status, mustChange, approved, deleted
}

// dropTeacher removes the bootstrap test's own leftover account. No other
// migrate test creates referenced children, so the single-row delete is
// order-safe without replicating the full storetest reset order.
func dropTeacher(t *testing.T, db *store.DB) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		"DELETE FROM accounts WHERE email = 'guru@sekolah.sch.id'"); err != nil {
		t.Fatalf("drop teacher: %v", err)
	}
}

// liveLedgerNames returns the live ledger's names in insertion order.
func liveLedgerNames(t *testing.T, db *store.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		"SELECT name FROM schema_migrations WHERE deleted_at = '1970-01-01 00:00:00' ORDER BY id")
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan ledger: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return names
}

// TestApplyTwiceOnEmptyDatabase is schema verification A-01: the seven files
// run against an empty database twice, the second run changes nothing, and
// the ledger afterwards holds exactly seven live rows with unique names.
func TestApplyTwiceOnEmptyDatabase(t *testing.T) {
	db := testDB(t)
	emptyLedger(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	first, err := Apply(ctx, db)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if len(first) != 7 {
		t.Fatalf("first apply = %d migrations, want 7: %v", len(first), first)
	}

	second, err := Apply(ctx, db)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second apply changed %v, want a no-op", second)
	}

	names := liveLedgerNames(t, db)
	if len(names) != 7 {
		t.Fatalf("ledger holds %d live rows, want 7: %v", len(names), names)
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Errorf("ledger lists %q twice", name)
		}
		seen[name] = true
	}
}

// TestBrokenFileLeavesNoLedgerRow is A-01's third clause: one file failing
// after others succeed records the survivors only — the broken file is never
// in the ledger, and a later clean run finishes the job.
func TestBrokenFileLeavesNoLedgerRow(t *testing.T) {
	db := testDB(t)
	emptyLedger(t, db) // run the good files for real, not as skips

	broken := migration{
		name:   "9999_broken",
		source: "ALTER TABLE no_such_table ADD COLUMN c int;\n",
	}

	files, err := embeddedMigrations()
	if err != nil {
		t.Fatalf("embeddedMigrations: %v", err)
	}
	// The broken file rides last so the seven good ones really do apply.
	files = append(files, broken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("pin connection: %v", err)
	}
	defer conn.Close()
	applied, err := applyAll(ctx, conn, files)
	if err == nil {
		t.Fatal("applyAll succeeded on a broken file, want an error")
	}
	if len(applied) != 7 {
		t.Fatalf("applied %d before the failure, want the 7 good files: %v", len(applied), applied)
	}

	for _, name := range liveLedgerNames(t, db) {
		if name == broken.name {
			t.Errorf("ledger contains %q after its run failed", name)
		}
	}
	if got := len(liveLedgerNames(t, db)); got != 7 {
		t.Fatalf("ledger holds %d live rows after the failure, want 7", got)
	}

	// Recovery is fix-forward (§9.6): removing the broken file and re-running
	// finishes the set with no ledger surgery.
	if _, err := applyAll(ctx, conn, files[:7]); err != nil {
		t.Fatalf("re-run after the failure: %v", err)
	}
}

// TestBootstrapTeacherIdempotent creates the first teacher twice and expects
// one row for the effort: active, approved, forced to change the password on
// first login (§9.5).
func TestBootstrapTeacherIdempotent(t *testing.T) {
	db := testDB(t)
	dropTeacher(t, db)
	hash := teacherHash(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	created, err := BootstrapTeacher(ctx, db, "Guru Pertama Sekolah", "guru@sekolah.sch.id", hash)
	if err != nil {
		t.Fatalf("bootstrap teacher: %v", err)
	}
	if !created {
		t.Fatal("first bootstrap created nothing")
	}

	created, err = BootstrapTeacher(ctx, db, "Guru Pertama Sekolah", "guru@sekolah.sch.id", hash)
	if err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	if created {
		t.Fatal("second bootstrap created a second account")
	}

	role, status, mustChange, approvedAt, deletedAt := teacherRow(t, db)
	if role != "teacher" || status != "active" || mustChange != 1 {
		t.Errorf("teacher account = %s/%s/must_change=%d, want teacher/active/1", role, status, mustChange)
	}
	if approvedAt == nil {
		t.Error("approved_at is NULL, want CURRENT_TIMESTAMP")
	}
	if deletedAt == nil || deletedAt.(time.Time).Format("2006-01-02 15:04:05") != store.LiveAt {
		t.Errorf("deleted_at = %v, want the sentinel %s", deletedAt, store.LiveAt)
	}
}

// TestBootstrapTeacherRefusesNonTeacherEmail covers the post-condition the
// deployment depends on: success means a live teacher exists for the address.
// An address already used by a student must fail loudly, not report "nothing
// created" and leave the deployment with no way in.
func TestBootstrapTeacherRefusesNonTeacherEmail(t *testing.T) {
	db := testDB(t)
	dropTeacher(t, db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	hash := teacherHash(t)

	if _, err := db.ExecContext(ctx,
		"INSERT INTO accounts (full_name, email, password_hash, role, status) VALUES (?, ?, ?, 'student', 'active')",
		"Siswa Contoh", "guru@sekolah.sch.id", hash); err != nil {
		t.Fatalf("seed student: %v", err)
	}
	t.Cleanup(func() { dropTeacher(t, db) })

	if _, err := BootstrapTeacher(ctx, db, "Guru Pertama Sekolah", "guru@sekolah.sch.id", hash); err == nil {
		t.Fatal("bootstrap reported success for an address owned by a student")
	}
}

// TestApplyRefusesWhenAnotherRunnerHoldsTheLock covers §9.3 step 3. GET_LOCK
// answers with a value, so an applier that only checked the statement's error
// would sail past a held lock and interleave DDL with the runner that owns it.
func TestApplyRefusesWhenAnotherRunnerHoldsTheLock(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	holder, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("pin holder connection: %v", err)
	}
	defer holder.Close()

	var got sql.NullInt64
	if err := holder.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, 1).Scan(&got); err != nil {
		t.Fatalf("holder GET_LOCK: %v", err)
	}
	if !got.Valid || got.Int64 != 1 {
		t.Fatalf("holder GET_LOCK = %v, want 1", got)
	}
	defer func() {
		_, _ = holder.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName)
	}()

	if _, err := apply(ctx, db, 1); err == nil {
		t.Fatal("apply proceeded while another runner held the lock")
	} else if !strings.Contains(err.Error(), "not granted") {
		t.Fatalf("apply error = %v, want a lock refusal", err)
	}
}
