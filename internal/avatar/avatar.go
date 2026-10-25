// Package avatar converts a self-uploaded profile photo into the stored
// 512×512 WebP avatar (spec 2026-10-25-profile-photos-design, rules AVT-040
// and AVT-050..AVT-059). The HTTP layer stages the raw original to a temp
// file and generates the destination name; this package owns the format
// sniff, the pixel ceiling, the decode, the EXIF orientation fix, the square
// crop, the WebP re-encode, and the bounded worker pool that runs them.
//
// Every failure Process returns matches exactly one of the sentinel errors
// below, which the caller maps to the flash keys of AVT-094: ErrType to
// photo-type, ErrBig to photo-big, ErrUnreadable to photo-unreadable,
// ErrFailed to photo-failed, and ErrBusy (signalled by TryAcquire) to
// photo-busy. No message text is ever mapped; always use errors.Is.
//
// The image codecs are cgo (libwebp, libheif). Under CGO_ENABLED=0 the
// package still compiles and tests its pure-Go parts, but New reports the
// missing cgo support instead of a Processor.
package avatar

import (
	"context"
	"errors"
	"fmt"
)

// OutputSize is the edge length of every stored avatar in pixels: sources are
// centre-cropped square on the short side and scaled here (AVT-056).
const OutputSize = 512

// MaxAvatarBytes is the hard ceiling on one encoded avatar (AVT-058). The
// serving route re-checks it (AVT-071); export it so both pin the same
// number. A 512×512 WebP is 15–40 KB, so the ceiling is asserted, not
// approached.
const MaxAvatarBytes = 5_000_000

// MaxPixels and MaxSide are the pixel ceiling (AVT-051): dimensions are
// probed from headers or container metadata BEFORE any pixel decode, and a
// source over either limit is refused with ErrBig. A 108 MP or 200 MP phone
// photo is deliberately rejected even though its file fits in the 200 MB
// request cap (§14).
const (
	MaxPixels = 50_000_000
	MaxSide   = 16_000
)

// MaxSourceBytes is the pre-probe guard for WebP and HEIC only (AVT-053,
// AVT-054): those are the two formats whose libraries read the whole
// compressed file into memory, so a source larger than 64 MiB is refused
// with ErrBig before either library sees it. It is independent of MaxPixels
// and applies even when the pixel count would be small.
const MaxSourceBytes = 64 << 20

// Sentinel outcomes of a conversion. Exactly one of them (or a wrapped
// error matching one of them) is what Process returns; ErrBusy is what
// TryAcquire reports when the pool is full. Map them with errors.Is to the
// flash keys of AVT-094 — never by message text.
var (
	// ErrType: the bytes are not a JPEG, PNG, WebP, or HEIC the sniff
	// accepts (AVT-050). Flash key photo-type.
	ErrType = errors.New("avatar: unsupported image type")
	// ErrBig: over MaxPixels/MaxSide, or a WebP/HEIC source over
	// MaxSourceBytes (AVT-051/053/054). Flash key photo-big.
	ErrBig = errors.New("avatar: image too large to process")
	// ErrUnreadable: the format is right but the file cannot be probed or
	// decoded (AVT-052/054) — including a HEIC libheif cannot read, which
	// is NOT ErrType. Flash key photo-unreadable.
	ErrUnreadable = errors.New("avatar: image cannot be decoded")
	// ErrFailed: an internal failure — I/O, an oversized encode after the
	// quality-60 retry, a panic in a codec, or a shutting-down pool
	// (AVT-058). Flash key photo-failed; nothing is written.
	ErrFailed = errors.New("avatar: conversion failed")
	// ErrBusy: every pool slot is taken (AVT-040). Flash key photo-busy;
	// the caller must answer busy WITHOUT having read the file part.
	ErrBusy = errors.New("avatar: conversion pool is full")
)

// Processor owns the avatar directory and the conversion worker pool
// (AVT-040): four goroutine workers, eight in-flight slots, channel-based,
// no shared-state mutexes. It is safe for concurrent use and must be
// created once per process; call Close at shutdown.
type Processor struct {
	dir string
	pl  *pool
}

// New returns a Processor that writes avatars into dir. The directory is
// created lazily on the first write with mode 0750 (§4.2), so New does not
// touch the filesystem. Under CGO_ENABLED=0 New fails with an error saying
// cgo image support requires CGO_ENABLED=1 — the codecs (libwebp, libheif)
// cannot exist without it (AVT-105).
func New(dir string) (*Processor, error) {
	if err := checkCodec(); err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, errors.New("avatar: avatars directory must not be empty")
	}
	p := &Processor{dir: dir}
	p.pl = newPool(workerCount, slotCount, queueDepth, p.convert)
	return p, nil
}

// convert is the pool's process function: the whole pipeline for one staged
// source file, run by a worker goroutine.
func (p *Processor) convert(ctx context.Context, srcPath, dstPath string) error {
	return convertFile(ctx, p.dir, srcPath, dstPath)
}

// TryAcquire reserves one of the processor's eight in-flight slots without
// blocking. It must be called BEFORE the caller reads any file bytes: when
// all slots are taken it returns (nil, ErrBusy) and the request must fail
// with photo-busy having staged nothing (AVT-040). On success the caller
// holds the slot until it calls Release — typically `defer slot.Release()`
// taken right here — and should then stage the upload and call
// Slot.Process. TryAcquire is safe for concurrent use.
func (p *Processor) TryAcquire() (*Slot, error) {
	select {
	case p.pl.slots <- struct{}{}:
		return &Slot{p: p}, nil
	default:
		return nil, ErrBusy
	}
}

// Close shuts the pool down: it closes the stop channel exactly once
// (sync.Once) and joins the four workers (WaitGroup). A queued job is
// abandoned, a job already taken runs to completion, and any handler
// waiting on a result unblocks immediately (AVT-040). jobs is never
// closed, so no concurrent send can panic. Close is idempotent; in-flight
// Process calls return ErrFailed once the pool has stopped.
func (p *Processor) Close() {
	p.pl.close()
}

// Slot is one reserved in-flight conversion slot (AVT-040). A Slot belongs
// to the goroutine that acquired it: Process and Release must not be called
// from multiple goroutines on the same Slot.
type Slot struct {
	p        *Processor
	released bool
}

// Process converts the staged source file at srcPath into a 512×512 WebP
// written to dstPath and returns nil. The caller generates dstPath's name
// and owns the source file's cleanup (its defer removes the temp source on
// every path); this package owns decode, orientation, crop, scale, encode,
// and the atomic write: a temp file in the avatar directory, fsync, rename,
// mode 0640 (§4.2). On any error nothing is written at dstPath.
//
// Process submits the job to the pool and waits for a worker, selecting on
// the pool's stop channel (so shutdown unblocks it immediately) and on ctx
// (so a dead client unblocks it too; a decode already running finishes and
// may leave an orphan file, which §4.2/AVT-106 tolerates). Errors match the
// sentinels above; a cancelled ctx returns ctx's error unchanged.
//
// Process must only be called on a live Slot, before Release.
func (s *Slot) Process(ctx context.Context, srcPath, dstPath string) error {
	if s == nil || s.p == nil || s.released {
		return fmt.Errorf("%w: conversion slot is not held", ErrFailed)
	}
	pl := s.p.pl
	j := job{
		ctx:     ctx,
		srcPath: srcPath,
		dstPath: dstPath,
		result:  make(chan error, 1), // buffered: the worker's single send never blocks
	}
	select {
	case pl.jobs <- j:
	case <-pl.stop:
		return fmt.Errorf("%w: processor is shutting down", ErrFailed)
	}
	select {
	case err := <-j.result:
		return err
	case <-pl.stop:
		// Shutdown: unblock at once. The worker's buffered send cannot
		// block even though nobody waits for the result any more.
		return fmt.Errorf("%w: processor is shutting down", ErrFailed)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release returns the slot to the pool. It is idempotent and must be called
// exactly once per acquired Slot, typically deferred. A nil Slot is a
// no-op, so `defer slot.Release()` after a nil check stays simple.
func (s *Slot) Release() {
	if s == nil || s.released {
		return
	}
	s.released = true
	<-s.p.pl.slots
}
