package secure

import (
	"math"
	"sync"
	"time"
)

// Limiter is the process-local sliding-window rate limiter from OPS-231. It is
// a denial-of-service control, never an authorization control: a spoofed client
// address must not bypass it, and exceeding it never changes what a user is
// allowed to do.
//
// Every key keeps the timestamps of its accepted events inside the window. A
// rejected event is not recorded, so a client cannot extend its own penalty by
// retrying, and Retry-After always points at the moment the oldest accepted
// event leaves the window. State is in memory and dies with the process, which
// is why it is documented as a single-process control. A second instance would
// double every allowance.
type Limiter struct {
	mu       sync.Mutex
	entries  map[string]*window
	capacity int
	lastGC   time.Time
	now      func() time.Time
}

// window is one key's sliding event log. events is ordered oldest first and
// contains only accepted events inside the most recent span.
type window struct {
	events []time.Time
	seen   time.Time
	span   time.Duration
}

// limiterCapacity bounds the key table so a flood of distinct keys cannot grow
// it without limit.
const limiterCapacity = 10_000

// limiterGCInterval is how often expired keys are dropped.
const limiterGCInterval = 5 * time.Minute

// NewLimiter builds an empty limiter. now may be nil, in which case the wall
// clock is used; tests inject a fake clock.
func NewLimiter(now func() time.Time) *Limiter {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	nowTime := now()
	return &Limiter{
		entries:  make(map[string]*window),
		capacity: limiterCapacity,
		lastGC:   nowTime,
		now:      now,
	}
}

// Allow records one event for key and reports whether it is within limit.
// When it is not, retryAfter is the whole number of seconds until the oldest
// accepted event leaves the window, clamped to 1..900 for the Retry-After
// header.
//
// A limit of zero or less rejects every event without recording it; callers
// pass a real limit.
func (l *Limiter) Allow(key string, limit int, span time.Duration) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.gcLocked(now)

	w, ok := l.entries[key]
	if !ok {
		if len(l.entries) >= l.capacity {
			l.evictOldestLocked()
		}
		w = &window{}
		l.entries[key] = w
	}
	w.seen = now
	w.span = span
	w.trimLocked(now)

	if limit <= 0 {
		return false, 0
	}
	if len(w.events) >= limit {
		return false, retrySeconds(w.events[0].Add(span).Sub(now))
	}
	w.events = append(w.events, now)
	return true, 0
}

// trimLocked drops events that are no longer inside the window. An event at
// exactly now-span is expired, matching the fixed-window behaviour this type
// replaces.
func (w *window) trimLocked(now time.Time) {
	cutoff := now.Add(-w.span)
	i := 0
	for i < len(w.events) && !w.events[i].After(cutoff) {
		i++
	}
	if i > 0 {
		w.events = append(w.events[:0], w.events[i:]...)
	}
}

// RetryAfter reports the seconds remaining until key's oldest accepted event
// leaves the window without counting an event. It returns 0 when no event is
// still inside the window.
func (l *Limiter) RetryAfter(key string, span time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.entries[key]
	if !ok {
		return 0
	}
	now := l.now()
	w.trimLocked(now)
	if len(w.events) == 0 {
		return 0
	}
	return retrySeconds(w.events[0].Add(span).Sub(now))
}

// Clear drops a key's window. A successful login clears its (email, IP) bucket
// but never the IP bucket (IDN-064).
func (l *Limiter) Clear(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// Sweep drops every key with no live event in its window. Allow calls it
// opportunistically; it is exported so a background ticker can use it too.
func (l *Limiter) Sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcLocked(l.now())
}

// gcLocked drops keys whose event log is empty or fully outside their last
// window, and, at most every five minutes, runs the sweep. A key with a live
// event is never idle, even when its window is longer than the sweep interval
// (login uses fifteen minutes).
func (l *Limiter) gcLocked(now time.Time) {
	if now.Sub(l.lastGC) < limiterGCInterval {
		return
	}
	l.lastGC = now
	for k, w := range l.entries {
		w.trimLocked(now)
		if len(w.events) == 0 && now.Sub(w.seen) >= limiterGCInterval {
			delete(l.entries, k)
		}
	}
}

// evictOldestLocked makes room by dropping the least recently seen key. It is
// only reached at the capacity ceiling, where one eviction is enough.
func (l *Limiter) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	first := true
	for k, w := range l.entries {
		if first || w.seen.Before(oldest) {
			oldestKey, oldest, first = k, w.seen, false
		}
	}
	if !first {
		delete(l.entries, oldestKey)
	}
}

// retrySeconds converts a duration to whole seconds, rounding up, clamped to
// the 1..900 range Retry-After promises (IDN-065).
func retrySeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	s := int(math.Ceil(d.Seconds()))
	if s < 1 {
		return 1
	}
	if s > 900 {
		return 900
	}
	return s
}
