package secure_test

import (
	"testing"
	"time"

	"quiz-v2/internal/secure"
)

// clock is a settable time source for limiter tests.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newLimiter() (*secure.Limiter, *clock) {
	c := &clock{t: time.Unix(1_700_000_000, 0).UTC()}
	return secure.NewLimiter(c.now), c
}

func TestLimiterAllowsUpToLimit(t *testing.T) {
	l, _ := newLimiter()
	for i := 0; i < 3; i++ {
		ok, _ := l.Allow("login:1.2.3.4", 3, time.Minute)
		if !ok {
			t.Fatalf("event %d rejected, want allowed", i+1)
		}
	}
	ok, retry := l.Allow("login:1.2.3.4", 3, time.Minute)
	if ok {
		t.Fatal("fourth event allowed, want rejected")
	}
	if retry != 60 {
		t.Errorf("retryAfter = %d, want 60", retry)
	}
}

func TestLimiterWindowResets(t *testing.T) {
	l, c := newLimiter()
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("k", 2, time.Minute); !ok {
			t.Fatal("event rejected inside the allowance")
		}
	}
	if ok, _ := l.Allow("k", 2, time.Minute); ok {
		t.Fatal("over-limit event allowed")
	}
	c.advance(61 * time.Second)
	if ok, _ := l.Allow("k", 2, time.Minute); !ok {
		t.Fatal("event rejected after the window expired")
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	l, _ := newLimiter()
	if ok, _ := l.Allow("a", 1, time.Minute); !ok {
		t.Fatal("first key rejected")
	}
	if ok, _ := l.Allow("a", 1, time.Minute); ok {
		t.Fatal("second event on the same key allowed")
	}
	if ok, _ := l.Allow("b", 1, time.Minute); !ok {
		t.Fatal("a different key must have its own window")
	}
}

func TestLimiterClear(t *testing.T) {
	l, _ := newLimiter()
	if ok, _ := l.Allow("k", 1, time.Minute); !ok {
		t.Fatal("first event rejected")
	}
	if ok, _ := l.Allow("k", 1, time.Minute); ok {
		t.Fatal("over-limit event allowed")
	}
	l.Clear("k")
	if ok, _ := l.Allow("k", 1, time.Minute); !ok {
		t.Fatal("Clear did not reset the window")
	}
}

func TestLimiterRetryAfter(t *testing.T) {
	l, c := newLimiter()
	l.Allow("k", 1, time.Minute)
	if got := l.RetryAfter("k", time.Minute); got != 60 {
		t.Errorf("RetryAfter = %d, want 60", got)
	}
	c.advance(30 * time.Second)
	if got := l.RetryAfter("k", time.Minute); got != 30 {
		t.Errorf("RetryAfter = %d, want 30", got)
	}
	c.advance(31 * time.Second)
	if got := l.RetryAfter("k", time.Minute); got != 0 {
		t.Errorf("RetryAfter = %d, want 0 after the window", got)
	}
}

// TestLimiterRetryAfterIsClamped keeps the header inside the 1..900 range the
// contract promises (IDN-065).
func TestLimiterRetryAfterIsClamped(t *testing.T) {
	l, _ := newLimiter()
	l.Allow("k", 1, time.Hour)
	got := l.RetryAfter("k", time.Hour)
	if got < 1 || got > 900 {
		t.Errorf("RetryAfter = %d, want within 1..900", got)
	}
}

func TestLimiterZeroLimitRejectsEverything(t *testing.T) {
	l, _ := newLimiter()
	if ok, _ := l.Allow("k", 0, time.Minute); ok {
		t.Fatal("a zero limit must reject")
	}
}

func TestLimiterSweepDropsIdleKeys(t *testing.T) {
	l, c := newLimiter()
	l.Allow("old", 5, time.Minute)
	c.advance(6 * time.Minute)
	l.Sweep()
	if got := l.RetryAfter("old", time.Minute); got != 0 {
		t.Errorf("swept key still has a window: %d", got)
	}
}

// TestLimiterSlidingWindowFreesOneSlotAtATime pins OPS-231's sliding window:
// each accepted event leaves the window exactly one span after it was recorded,
// so the allowance returns gradually instead of all at once at a fixed boundary.
func TestLimiterSlidingWindowFreesOneSlotAtATime(t *testing.T) {
	l, c := newLimiter()
	if ok, _ := l.Allow("k", 2, time.Minute); !ok {
		t.Fatal("first event rejected")
	}
	c.advance(30 * time.Second)
	if ok, _ := l.Allow("k", 2, time.Minute); !ok {
		t.Fatal("second event rejected")
	}
	if ok, _ := l.Allow("k", 2, time.Minute); ok {
		t.Fatal("third event allowed inside the window")
	}

	// 61s after the first event, only the first has expired: one slot is free.
	c.advance(31 * time.Second)
	if ok, _ := l.Allow("k", 2, time.Minute); !ok {
		t.Fatal("slot not freed when the oldest event expired")
	}
	// The second event is still inside the window, so the next one is refused.
	if ok, _ := l.Allow("k", 2, time.Minute); ok {
		t.Fatal("two accepted events plus the second's window should be full")
	}

	// 91s after the second event, it has expired as well.
	c.advance(30 * time.Second)
	if ok, _ := l.Allow("k", 2, time.Minute); !ok {
		t.Fatal("slot not freed when the second event expired")
	}
}

// TestLimiterRejectedEventsDoNotExtendTheWindow keeps a retry storm from
// pushing the oldest accepted event out of reach.
func TestLimiterRejectedEventsDoNotExtendTheWindow(t *testing.T) {
	l, c := newLimiter()
	if ok, _ := l.Allow("k", 1, time.Minute); !ok {
		t.Fatal("first event rejected")
	}
	c.advance(59 * time.Second)
	for i := 0; i < 10; i++ {
		if ok, _ := l.Allow("k", 1, time.Minute); ok {
			t.Fatal("over-limit event allowed")
		}
	}
	c.advance(2 * time.Second)
	if ok, _ := l.Allow("k", 1, time.Minute); !ok {
		t.Fatal("rejected retries extended the penalty")
	}
}

// TestLimiterSweepKeepsLiveLongWindow protects the 15-minute login bucket from
// a five-minute sweep: a key with an event still inside its window is not idle.
func TestLimiterSweepKeepsLiveLongWindow(t *testing.T) {
	l, c := newLimiter()
	if ok, _ := l.Allow("login", 5, 15*time.Minute); !ok {
		t.Fatal("first event rejected")
	}
	c.advance(6 * time.Minute)
	l.Sweep()
	if got := l.RetryAfter("login", 15*time.Minute); got != 9*60 {
		t.Errorf("RetryAfter after sweep = %d, want 540", got)
	}
}
