package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"quiz-v2/internal/store"
)

func TestOpenInvalidDSNReturnsError(t *testing.T) {
	_, err := store.Open("not-a-dsn")
	if err == nil {
		t.Fatal("expected error for malformed DSN")
	}
}

func TestPingUnreachableReturnsError(t *testing.T) {
	db, err := store.Open("user:pass@tcp(127.0.0.1:1)/none?timeout=1s")
	if err != nil {
		t.Skipf("driver rejected DSN setup: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := store.Ping(ctx, db); err == nil {
		t.Fatal("expected ping error against closed port")
	}
}

// TestSentinel pins the soft-delete epoch. Every read filters on this exact
// value, and the unique keys that include deleted_at rely on it colliding for
// live rows (schema spec §5.3).
func TestSentinel(t *testing.T) {
	if store.LiveAt != "1970-01-01 00:00:00" {
		t.Errorf("LiveAt = %q", store.LiveAt)
	}
	want := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	if !store.LiveAtTime.Equal(want) {
		t.Errorf("LiveAtTime = %v, want %v", store.LiveAtTime, want)
	}
}

// TestCheckDSN proves the startup guard rejects a DSN that would silently break
// time scanning or widen an injection into a script (OPS-232, OPS-233).
func TestCheckDSN(t *testing.T) {
	good := "quiz:quiz@tcp(db:3306)/quiz?parseTime=true&loc=UTC"
	if err := store.CheckDSN(good); err != nil {
		t.Fatalf("valid DSN rejected: %v", err)
	}

	bad := map[string]string{
		"missing parseTime": "quiz:quiz@tcp(db:3306)/quiz?loc=UTC",
		"multiStatements":   "quiz:quiz@tcp(db:3306)/quiz?parseTime=true&multiStatements=true",
		"no database":       "quiz:quiz@tcp(db:3306)/?parseTime=true",
		"malformed":         "not-a-dsn",
	}
	if _, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		bad["non-UTC location"] = "quiz:quiz@tcp(db:3306)/quiz?parseTime=true&loc=Asia%2FJakarta"
	}

	for name, dsn := range bad {
		if err := store.CheckDSN(dsn); err == nil {
			t.Errorf("%s: DSN accepted", name)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := map[uint16]store.Class{
		1062: store.Duplicate,
		1213: store.Deadlock,
		1205: store.LockWait,
		1452: store.FKMissing,
		1406: store.TooLong,
		3819: store.CheckViolation,
		9999: store.ClassOther,
	}
	for number, want := range cases {
		err := &mysql.MySQLError{Number: number, Message: "x"}
		if got := store.Classify(err); got != want {
			t.Errorf("Classify(%d) = %v, want %v", number, got, want)
		}
	}
}

func TestClassifyNonDriverError(t *testing.T) {
	if got := store.Classify(errors.New("boom")); got != store.ClassOther {
		t.Errorf("Classify(plain) = %v, want ClassOther", got)
	}
	if got := store.Classify(nil); got != store.ClassOther {
		t.Errorf("Classify(nil) = %v, want ClassOther", got)
	}
}

func TestClassifyWrappedDriverError(t *testing.T) {
	wrapped := errors.Join(errors.New("context"), &mysql.MySQLError{Number: 1062, Message: "dup"})
	if !store.IsDuplicate(wrapped) {
		t.Error("IsDuplicate must see through wrapping")
	}
}

func TestCode(t *testing.T) {
	if got := store.Code(&mysql.MySQLError{Number: 1452}); got != 1452 {
		t.Errorf("Code = %d, want 1452", got)
	}
	if got := store.Code(errors.New("boom")); got != 0 {
		t.Errorf("Code(plain) = %d, want 0", got)
	}
}
