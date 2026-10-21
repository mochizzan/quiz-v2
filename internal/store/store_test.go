package store_test

import (
	"context"
	"testing"
	"time"

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
