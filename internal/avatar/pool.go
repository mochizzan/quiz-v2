package avatar

import (
	"context"
	"fmt"
	"sync"
)

// Pool geometry (AVT-040): slots caps the in-flight requests at 8 (acquired
// by handlers before they read any source), jobs queues up to 8 of them,
// and 4 workers consume the queue. With 8 slots there are at most 4 jobs
// processing and 4 queued, so the handler's send on jobs never blocks.
const (
	workerCount = 4
	slotCount   = 8
	queueDepth  = 8
)

// job is one conversion handed to a worker. result is buffered for exactly
// one send, so a worker never blocks on a handler that has walked away
// (client disconnect, shutdown), and jobs itself is never closed — a
// handler can therefore never panic by sending on it (AVT-040).
type job struct {
	ctx     context.Context
	srcPath string
	dstPath string
	result  chan error
}

// pool is the channel-based conversion pool: no shared-state mutexes, only
// channels, sync.Once, and a WaitGroup (AVT-040).
type pool struct {
	slots chan struct{} // capacity slotCount: one token per in-flight request
	jobs  chan job      // capacity queueDepth: NEVER closed
	stop  chan struct{} // closed exactly once by close()
	once  sync.Once
	wg    sync.WaitGroup
}

// newPool starts workers goroutines feeding on jobs. process is the
// conversion function each worker runs; it is injected so the pool is pure
// Go and testable without cgo.
func newPool(workers, slotCap, queueCap int, process func(context.Context, string, string) error) *pool {
	pl := &pool{
		slots: make(chan struct{}, slotCap),
		jobs:  make(chan job, queueCap),
		stop:  make(chan struct{}),
	}
	pl.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go pl.work(process)
	}
	return pl
}

// work is one worker goroutine: select on stop and jobs, run the job's
// conversion, deliver the result. On shutdown a queued job is abandoned
// while a job already taken runs to completion (AVT-040); stop is checked
// before pulling new work so abandonment is the normal case rather than a
// coin flip in the select.
func (pl *pool) work(process func(context.Context, string, string) error) {
	defer pl.wg.Done()
	for {
		select {
		case <-pl.stop:
			return
		default:
		}
		select {
		case <-pl.stop:
			return
		case j := <-pl.jobs:
			j.result <- runJob(process, j)
		}
	}
}

// runJob guards one conversion: bytes from the internet reach cgo codecs,
// and a panic there must fail one job instead of killing the server.
func runJob(process func(context.Context, string, string) error, j job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: codec panic: %v", ErrFailed, r)
		}
	}()
	return process(j.ctx, j.srcPath, j.dstPath)
}

// close stops the pool once and joins the workers: sync.Once closes stop a
// single time, WaitGroup waits for all workers to return (idempotent — a
// second Close just waits again on an already-finished group).
func (pl *pool) close() {
	pl.once.Do(func() { close(pl.stop) })
	pl.wg.Wait()
}
