package avatar

// Pool tests for AVT-040. The conversion function is injected (newPool
// takes it as an argument), so everything here runs under CGO_ENABLED=0:
// slots, queue, workers, shutdown, panic containment, and context
// cancellation are pure channel logic.

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// waitStarted polls until want jobs have entered the injected process
// function, failing the test after a generous bound.
func waitStarted(t *testing.T, started *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if started.Load() >= want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("only %d of %d jobs started within 3s", started.Load(), want)
}

// waitResult reads one job result, failing instead of hanging forever.
func waitResult(t *testing.T, ch chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("job did not deliver a result within 3s")
		return nil
	}
}

// newTestProcessor builds a Processor around an injected conversion
// function. New() itself needs the cgo codecs, so pool and slot tests go
// through the struct directly — the pool wiring under test is identical.
func newTestProcessor(process func(context.Context, string, string) error) *Processor {
	p := &Processor{dir: "pool-tests-do-not-touch-disk"}
	p.pl = newPool(workerCount, slotCount, queueDepth, process)
	return p
}

// TestPoolGeometryAndBusySlots pins the AVT-040 numbers (8 slots, queue of
// 8, 4 workers), the non-blocking TryAcquire contract — the ninth
// reservation is (nil, ErrBusy) so the caller can fail before reading the
// upload — and Release's idempotence.
func TestPoolGeometryAndBusySlots(t *testing.T) {
	if workerCount != 4 || slotCount != 8 || queueDepth != 8 {
		t.Fatalf("pool geometry = %d/%d/%d, want 4 workers, 8 slots, 8 queued (AVT-040)",
			workerCount, slotCount, queueDepth)
	}
	p := newTestProcessor(func(context.Context, string, string) error { return nil })
	defer p.Close()
	if got := cap(p.pl.slots); got != slotCount {
		t.Errorf("slots capacity = %d, want %d", got, slotCount)
	}
	if got := cap(p.pl.jobs); got != queueDepth {
		t.Errorf("jobs capacity = %d, want %d", got, queueDepth)
	}

	held := make([]*Slot, 0, slotCount)
	for i := 0; i < slotCount; i++ {
		s, err := p.TryAcquire()
		if err != nil {
			t.Fatalf("TryAcquire %d: %v", i, err)
		}
		held = append(held, s)
	}
	if s, err := p.TryAcquire(); s != nil || !errors.Is(err, ErrBusy) {
		t.Fatalf("ninth TryAcquire = (%v, %v), want (nil, ErrBusy)", s, err)
	}

	// Release is idempotent: a second call must not hand a token back, and
	// it must not block on the (now empty) slots channel either.
	s := held[0]
	s.Release()
	done := make(chan struct{})
	go func() {
		s.Release()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second Release blocked: the slot was released twice")
	}
	if _, err := p.TryAcquire(); err != nil {
		t.Fatalf("after one real Release, one slot must be free: %v", err)
	}
	if s2, err := p.TryAcquire(); s2 != nil || !errors.Is(err, ErrBusy) {
		t.Errorf("second Release returned a phantom slot: (%v, %v)", s2, err)
	}

	// A nil Slot releases safely (the documented `defer slot.Release()`).
	var nilSlot *Slot
	nilSlot.Release()

	// Process on a released slot, or on a nil slot, is an error rather
	// than a panic or a job smuggled into the pool.
	if err := s.Process(context.Background(), "src", "dst"); !errors.Is(err, ErrFailed) {
		t.Errorf("Process after Release: got %v, want ErrFailed", err)
	}
	if err := nilSlot.Process(context.Background(), "src", "dst"); !errors.Is(err, ErrFailed) {
		t.Errorf("Process on nil slot: got %v, want ErrFailed", err)
	}
}

// TestPoolRunsJobs: every queued job reaches a worker and its result comes
// back on the buffered channel; a panic inside the conversion (bytes from
// the internet reach cgo codecs) fails that one job with ErrFailed instead
// of taking the server down.
func TestPoolRunsJobs(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	process := func(_ context.Context, src, dst string) error {
		if src == "boom" {
			panic("codec exploded")
		}
		mu.Lock()
		defer mu.Unlock()
		seen[src] = dst
		return nil
	}
	pl := newPool(workerCount, slotCount, queueDepth, process)
	defer pl.close()

	const n = 12 // more than the queue: workers drain while the test submits
	results := make([]chan error, n)
	for i := 0; i < n; i++ {
		j := job{
			ctx:     context.Background(),
			srcPath: fmt.Sprintf("s%d", i),
			dstPath: fmt.Sprintf("d%d", i),
			result:  make(chan error, 1),
		}
		select {
		case pl.jobs <- j:
		case <-time.After(3 * time.Second):
			t.Fatalf("send of job %d blocked", i)
		}
		results[i] = j.result
	}
	for i, ch := range results {
		if err := waitResult(t, ch); err != nil {
			t.Errorf("job %d: %v", i, err)
		}
	}
	mu.Lock()
	if len(seen) != n {
		t.Errorf("process ran for %d jobs, want %d", len(seen), n)
	}
	for i := 0; i < n; i++ {
		if got := seen[fmt.Sprintf("s%d", i)]; got != fmt.Sprintf("d%d", i) {
			t.Errorf("job %d: src=%q, want dst %q", i, got, fmt.Sprintf("d%d", i))
		}
	}
	mu.Unlock()

	// Panic containment.
	j := job{ctx: context.Background(), srcPath: "boom", result: make(chan error, 1)}
	pl.jobs <- j
	err := waitResult(t, j.result)
	if !errors.Is(err, ErrFailed) || !strings.Contains(err.Error(), "panic") {
		t.Errorf("panicking job: got %v, want ErrFailed wrapping a panic", err)
	}
}

// TestPoolWorkerCountBounded: with every worker inside a gated
// conversion, the fifth job stays in the queue — exactly workerCount
// conversions run at once (AVT-040). Opening the gate lets the last job
// through.
func TestPoolWorkerCountBounded(t *testing.T) {
	gate := make(chan struct{})
	var started atomic.Int32
	process := func(context.Context, string, string) error {
		started.Add(1)
		<-gate
		return nil
	}
	pl := newPool(workerCount, slotCount, queueDepth, process)
	defer pl.close() // registered first: runs last, after the gate opens

	var release sync.Once
	openGate := func() { release.Do(func() { close(gate) }) }
	defer openGate() // never leave workers blocked if the test fails early

	const jobs = workerCount + 1
	results := make([]chan error, jobs)
	for i := range results {
		j := job{ctx: context.Background(), result: make(chan error, 1)}
		select {
		case pl.jobs <- j:
		case <-time.After(3 * time.Second):
			t.Fatalf("send of job %d blocked", i)
		}
		results[i] = j.result
	}

	waitStarted(t, &started, workerCount)
	time.Sleep(100 * time.Millisecond) // give a fifth worker every chance to exist
	if got := started.Load(); got != workerCount {
		t.Errorf("started = %d, want %d: the fifth job must stay queued", got, workerCount)
	}

	openGate()
	for i, ch := range results {
		if err := waitResult(t, ch); err != nil {
			t.Errorf("job %d: %v", i, err)
		}
	}
	if got := started.Load(); got != jobs {
		t.Errorf("started = %d, want %d after the gate opened", got, jobs)
	}
}

// TestPoolCloseUnblocksWaitersAndAbandonsQueue is the shutdown contract of
// AVT-040: Close stops the pool exactly once, every handler waiting on a
// result unblocks with ErrFailed at once, jobs still queued are abandoned
// (their conversions never start), the workers join, and because jobs is
// never closed no send on it can panic afterwards.
func TestPoolCloseUnblocksWaitersAndAbandonsQueue(t *testing.T) {
	gate := make(chan struct{})
	var started atomic.Int32
	process := func(context.Context, string, string) error {
		started.Add(1)
		<-gate
		return nil
	}
	pl := newPool(workerCount, slotCount, queueDepth, process)

	var release sync.Once
	openGate := func() { release.Do(func() { close(gate) }) }

	// Eight handlers: four jobs are inside the workers (blocked on the
	// gate), four sit in the queue. Their waits mirror Slot.Process.
	const handlers = slotCount
	outs := make([]chan error, handlers)
	for i := range outs {
		out := make(chan error, 1)
		outs[i] = out
		go func(out chan error) {
			j := job{ctx: context.Background(), result: make(chan error, 1)}
			select {
			case pl.jobs <- j:
			case <-pl.stop:
				out <- fmt.Errorf("%w: processor is shutting down", ErrFailed)
				return
			}
			select {
			case err := <-j.result:
				out <- err
			case <-pl.stop:
				out <- fmt.Errorf("%w: processor is shutting down", ErrFailed)
			}
		}(out)
	}

	waitStarted(t, &started, workerCount)

	closeDone := make(chan struct{})
	go func() {
		pl.close()
		close(closeDone)
	}()

	for i, out := range outs {
		select {
		case err := <-out:
			if !errors.Is(err, ErrFailed) {
				t.Errorf("handler %d after Close: got %v, want ErrFailed", i, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("handler %d still blocked 3s after Close", i)
		}
	}

	openGate()
	select {
	case <-closeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("workers did not join after Close")
	}
	if got := started.Load(); got != workerCount {
		t.Errorf("started = %d, want %d: queued jobs must be abandoned on Close", got, workerCount)
	}

	// jobs is never closed, so a handler sending after Close cannot panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("send on jobs after Close panicked: %v", r)
			}
		}()
		pl.jobs <- job{ctx: context.Background(), result: make(chan error, 1)}
	}()

	pl.close() // idempotent: a second Close just waits again
}

// TestSlotProcessDeliversResult: the public path — a held Slot's Process
// runs the injected conversion on a worker and returns its error.
func TestSlotProcessDeliversResult(t *testing.T) {
	type call struct{ src, dst string }
	var got call
	p := newTestProcessor(func(_ context.Context, src, dst string) error {
		got = call{src, dst}
		return nil
	})
	defer p.Close()

	s, err := p.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	defer s.Release()
	if err := s.Process(context.Background(), "staged.jpg", "abc.webp"); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got.src != "staged.jpg" || got.dst != "abc.webp" {
		t.Errorf("conversion got (%q, %q), want (staged.jpg, abc.webp)", got.src, got.dst)
	}
}

// TestSlotProcessContextCancelled: a dead client unblocks Process with
// ctx's own error (the conversion observes the same cancellation).
func TestSlotProcessContextCancelled(t *testing.T) {
	process := func(ctx context.Context, _, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}
	p := newTestProcessor(process)
	defer p.Close()

	s, err := p.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	defer s.Release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Process(ctx, "staged.jpg", "abc.webp"); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled Process: got %v, want context.Canceled", err)
	}
}

// TestSlotProcessAfterCloseFails: once Close has run, an in-flight or new
// Process returns ErrFailed instead of blocking forever or panicking.
func TestSlotProcessAfterCloseFails(t *testing.T) {
	p := newTestProcessor(func(context.Context, string, string) error { return nil })
	s, err := p.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	defer s.Release()
	p.Close()
	for i := 0; i < 2; i++ {
		if err := s.Process(context.Background(), "staged.jpg", "abc.webp"); !errors.Is(err, ErrFailed) {
			t.Errorf("Process after Close #%d: got %v, want ErrFailed", i+1, err)
		}
	}
}

// TestPoolGoroutinesJoinOnClose: every worker goroutine is gone shortly
// after Close returns — the WaitGroup join really happens.
func TestPoolGoroutinesJoinOnClose(t *testing.T) {
	runtime.Gosched()
	baseline := runtime.NumGoroutine()

	pl := newPool(workerCount, slotCount, queueDepth, func(context.Context, string, string) error {
		return nil
	})
	j := job{ctx: context.Background(), result: make(chan error, 1)}
	pl.jobs <- j
	if err := waitResult(t, j.result); err != nil {
		t.Fatalf("warm-up job: %v", err)
	}
	pl.close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("goroutines = %d after Close, baseline = %d: workers did not join",
		runtime.NumGoroutine(), baseline)
}
