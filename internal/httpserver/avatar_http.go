package httpserver

// avatar_http.go implements the profile-photo routes end to end: the streamed
// upload (AVT-020…AVT-059), the idempotent removal (AVT-061), and the asset
// route that serves the stored WebP (AVT-070…AVT-074). The binding design is
// docs/superpowers/specs/2026-10-25-profile-photos-design.md.
//
// The upload route is the one Stream route in the app: prepare() has already
// checked the declared Content-Length and resolved the session, and hands this
// handler an untouched body stream. Everything that touches the body — the
// first-part CSRF token, the staging copy, the conversion — lives here.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"

	"quiz-v2/internal/apperr"
	"quiz-v2/internal/authoring"
	"quiz-v2/internal/avatar"
	"quiz-v2/internal/avatarid"
	"quiz-v2/internal/identity"
)

// Flash keys of AVT-094. The copy itself lives in viewmodel.FlashFor, which is
// the single place a message becomes visible; these constants are only the
// keys, so a handler and a test cannot spell one two ways.
const (
	flashPhotoEmpty        = "photo-empty"
	flashPhotoType         = "photo-type"
	flashPhotoBig          = "photo-big"
	flashPhotoUnreadable   = "photo-unreadable"
	flashPhotoFailed       = "photo-failed"
	flashPhotoBusy         = "photo-busy"
	flashPhotoRemoveFailed = "photo-remove-failed"
	flashPhotoSaved        = "photo-saved"
	flashPhotoRemoved      = "photo-removed"
)

// avatarCacheControl is AVT-072's one cache policy: immutable because every
// upload mints a fresh name, private because shared caches must never store a
// student photo.
const avatarCacheControl = "private, max-age=31536000, immutable"

// PhotoProcessor is the codec seam the upload route needs: reserve one of the
// eight in-flight slots, then convert one staged file. *avatar.Processor
// satisfies it through PhotoProcessorOf. A build that wires no processor has a
// nil seam and the route answers 500 rather than guessing (AVT-055).
type PhotoProcessor interface {
	TryAcquire() (PhotoSlot, error)
}

// PhotoSlot is one reserved conversion slot, held until Release (AVT-040).
type PhotoSlot interface {
	Process(ctx context.Context, srcPath, dstPath string) error
	Release()
}

// photoProcessor adapts *avatar.Processor to the seam. Go interfaces do not
// covary, so TryAcquire's concrete *avatar.Slot result needs this wrapper.
type photoProcessor struct{ p *avatar.Processor }

// TryAcquire forwards to the codec's non-blocking slot reservation.
func (w photoProcessor) TryAcquire() (PhotoSlot, error) {
	slot, err := w.p.TryAcquire()
	if err != nil {
		return nil, err
	}
	return photoSlot{s: slot}, nil
}

// photoSlot adapts *avatar.Slot to PhotoSlot.
type photoSlot struct{ s *avatar.Slot }

// Process forwards the conversion to the codec.
func (s photoSlot) Process(ctx context.Context, srcPath, dstPath string) error {
	return s.s.Process(ctx, srcPath, dstPath)
}

// Release returns the slot to the pool.
func (s photoSlot) Release() { s.s.Release() }

// PhotoProcessorOf wraps the codec package's processor in the seam. A nil
// processor stays nil, so a build without one fails closed at request time.
func PhotoProcessorOf(p *avatar.Processor) PhotoProcessor {
	if p == nil {
		return nil
	}
	return photoProcessor{p: p}
}

// AvatarStore is the account-scoped photo store: the only writer of
// accounts.avatar_name and of UPLOADS_DIR/avatars (design §4.2).
// *identity.AvatarStore satisfies it.
type AvatarStore interface {
	Dir() string
	Commit(ctx context.Context, accountID int64, srcPath, newName string) error
	Remove(ctx context.Context, accountID int64) error
	HasLiveOwner(ctx context.Context, name string) (bool, error)
}

// avatarStore is the photo store for one request: an injected one when the
// build supplies it, otherwise identity's store over the configured upload
// root. Building it here mirrors serveUpload, which builds authoring's image
// service per request (AVT-074).
func (s *server) avatarStore() AvatarStore {
	if s.Avatars != nil {
		return s.Avatars
	}
	return identity.NewAvatarStore(s.db, s.cfg.UploadsDir).WithLogger(s.logger)
}

// errAvatarTooLarge is the cap reader's sentinel: the request produced more
// than maxAvatarRequest bytes. It maps to the same 413 and Connection: close
// as an over-declared request, so a client that lies about its length is
// stopped at the same number as one that tells the truth (AVT-041).
var errAvatarTooLarge = errors.New("httpserver: avatar request exceeds the cap")

// errFieldTooLong is a form part wider than the field cap: transport garbage
// with a valid shape (AVT-031).
var errFieldTooLong = errors.New("httpserver: multipart field exceeds its cap")

// errStageWrite separates "the disk refused the upload" (photo-failed) from
// "the body could not be read" (422) when the staging copy stops early.
var errStageWrite = errors.New("httpserver: avatar staging write failed")

// errNoBoundary is a multipart Content-Type without a usable boundary.
var errNoBoundary = errors.New("httpserver: multipart boundary missing")

// capReader bounds a request body at limit bytes. The byte after the limit is
// the probe: a body that reaches it returns errAvatarTooLarge instead of that
// byte, while a body that ends exactly at the limit still reports EOF, so an
// upload of exactly maxAvatarRequest bytes stays legal.
type capReader struct {
	src       io.Reader
	remaining int64
}

// newCapReader bounds src at limit bytes, not limit+1: reading byte limit+1 is
// what trips the sentinel.
func newCapReader(src io.Reader, limit int64) *capReader {
	return &capReader{src: src, remaining: limit}
}

// Read copies up to the remaining allowance, then probes for one extra byte.
func (r *capReader) Read(p []byte) (int, error) {
	if r.remaining > 0 {
		if int64(len(p)) > r.remaining {
			p = p[:r.remaining]
		}
		n, err := r.src.Read(p)
		r.remaining -= int64(n)
		return n, err
	}
	// The allowance is gone. A probe byte means the body is over cap; EOF
	// means it ended exactly on the cap and is fine.
	var probe [1]byte
	n, err := r.src.Read(probe[:])
	if n > 0 {
		return 0, errAvatarTooLarge
	}
	return 0, err
}

// profileHome is the page both photo routes always return to: the session's
// role decides, never a user-supplied target (AVT-060, AVT-065).
func profileHome(x *Ctx) string {
	if x.IsTeacher() {
		return "/teacher/profile"
	}
	return "/profile"
}

// avatarUpload is POST /profile/photo (AVT-060). prepare() has checked the
// declared cap, set Connection: close, and resolved the session; this handler
// owns the body from here on.
func avatarUpload(x *Ctx) error {
	c := x.c
	target := profileHome(x)

	// Connection: close was set before session resolution, so every early
	// return below — 403, 303, 413, 422, 500 — leaves a connection whose
	// unread body is never drained for keep-alive reuse (AVT-044).

	processor := x.server.Photos
	if processor == nil {
		return x.server.respondError(c, apperr.Internalf(nil).WithRule("AVT-055"))
	}
	if x.Session == nil {
		// The class check guarantees a session; this keeps a future mistake
		// from becoming a nil dereference.
		return x.server.redirectFlash(c, "/login", identity.FlashSessionExpired)
	}

	// AVT-026: the stream is the request's body in streaming mode. A positive
	// declared length always has one; a broken request without it takes the
	// generic 422 rather than an empty success.
	stream := c.Request().BodyStream()
	if stream == nil {
		return respondFormInvalid(x)
	}
	boundary, err := boundaryOf(c)
	if err != nil {
		return respondFormInvalid(x)
	}
	mr := multipart.NewReader(newCapReader(stream, maxAvatarRequest), boundary)

	// AVT-030: the token must be the first part, so no attacker-controlled
	// file byte is read before the request is authenticated.
	first, err := mr.NextPart()
	if errors.Is(err, io.EOF) {
		// A multipart body with no parts at all is a user miss, not an
		// attempted CSRF (AVT-032).
		return x.server.redirectFlash(c, target, flashPhotoEmpty)
	}
	if err != nil {
		return partError(x, err)
	}
	if first.FormName() != "csrf_token" {
		return csrfFailure(x)
	}
	token, err := readField(first, csrfTokenCap)
	if err != nil {
		return partError(x, err)
	}
	// verifyFormCSRF answers 403 and reissues the cookie itself, exactly like
	// an ordinary form POST (AVT-030).
	if !x.server.verifyFormCSRF(c, x, string(token)) {
		return nil
	}

	// AVT-033: only after the token is valid does the guard run, so a
	// cross-site POST from a must-change session fails 403, not a redirect.
	if x.Session.MustChangePassword {
		return x.server.redirectFlash(c, "/change-password", identity.FlashMustChange)
	}

	// AVT-040: a slot is reserved before a single file byte is read, so a full
	// pool answers photo-busy without staging anything.
	slot, err := processor.TryAcquire()
	if err != nil {
		if errors.Is(err, avatar.ErrBusy) {
			return x.server.redirectFlash(c, target, flashPhotoBusy)
		}
		return x.server.respondError(c, apperr.Internalf(err).WithRule("AVT-055"))
	}
	defer slot.Release()

	// Both temp files live in the avatars root: it is the filesystem the new
	// name will be renamed onto, and its dot-prefixed names can never satisfy
	// the avatar grammar, so a crash's leftovers are unrequestable (AVT-074).
	store := x.server.avatarStore()
	dir := store.Dir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		x.server.logger.Error("avatar dir create failed", "rule", "AVT-041", "error", err)
		return x.server.redirectFlash(c, target, flashPhotoFailed)
	}
	src, err := os.CreateTemp(dir, ".upload-*.tmp")
	if err != nil {
		x.server.logger.Error("avatar stage create failed", "rule", "AVT-041", "error", err)
		return x.server.redirectFlash(c, target, flashPhotoFailed)
	}
	srcPath := src.Name()
	// One defer removes the staged source on every path: success, bad image,
	// wrong token, client disconnect (AVT-042).
	defer func() { _ = os.Remove(srcPath) }()

	// The destination name is minted before the conversion so the storage
	// order of §4.2 holds: write and fsync, rename to the final name, commit
	// the column, unlink the old file.
	newName, err := avatarid.Generate()
	if err != nil {
		_ = src.Close()
		return x.server.respondError(c, apperr.Internalf(err).WithRule("AVT-010"))
	}
	dstPath := filepath.Join(dir, newName)
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(dstPath)
		}
	}()

	staged, err := stagePhoto(mr, src)
	if cerr := src.Close(); cerr != nil && err == nil {
		err = fmt.Errorf("%w: %w", errStageWrite, cerr)
	}
	if err != nil {
		return partError(x, err)
	}
	if staged == 0 {
		return x.server.redirectFlash(c, target, flashPhotoEmpty)
	}

	// The conversion budget is the route's own 10-minute deadline (AVT-025);
	// a cancelled request unblocks the worker wait immediately.
	if err := slot.Process(x.Context(), srcPath, dstPath); err != nil {
		x.server.logger.Warn("avatar_convert", "rule", "AVT-102",
			"account_id", x.Session.AccountID, "error", err)
		return x.server.redirectFlash(c, target, photoFlashFor(err))
	}

	if err := store.Commit(x.Context(), x.Session.AccountID, dstPath, newName); err != nil {
		if errors.Is(err, identity.ErrSessionGone) {
			// The account vanished between resolution and the write: the
			// session is dead, exactly like every other account-row write.
			return x.server.redirectFlash(c, "/login", identity.FlashSessionExpired)
		}
		x.server.logger.Error("avatar commit failed", "rule", "AVT-010",
			"account_id", x.Session.AccountID, "error", err)
		return x.server.redirectFlash(c, target, flashPhotoFailed)
	}
	committed = true
	return x.server.redirectFlash(c, target, flashPhotoSaved)
}

// stagePhoto copies the file part into the staged source, bounded by the cap
// reader around the whole body. It returns the byte count, or the first
// failure: errAvatarTooLarge past the request cap, errStageWrite when the disk
// refuses, and the raw read error for a truncated or disconnected body.
func stagePhoto(mr *multipart.Reader, dst *os.File) (int64, error) {
	var total int64
	seen := false
	// The CSRF part was already consumed by the caller, so the count starts at
	// one: the body may carry at most fifteen more parts (AVT-031).
	parts := 1
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
		parts++
		if parts > avatarPartsCap {
			return total, errFieldTooLong
		}

		if part.FormName() == "photo" {
			if seen {
				// A second file part is not a field the handler can ignore
				// (AVT-031). The flag, not the byte count, is what makes a
				// zero-byte first photo count as seen.
				return total, errSecondPhoto
			}
			seen = true
			n, err := copyPart(dst, part)
			total += n
			if err != nil {
				return total, err
			}
			continue
		}
		// An unknown field is read only far enough to be bounded and skipped;
		// its value is never kept (AVT-031).
		if _, err := readField(part, csrfTokenCap); err != nil {
			return total, err
		}
	}
}

// errSecondPhoto is a second `photo` part in one upload (AVT-031).
var errSecondPhoto = errors.New("httpserver: more than one photo part")

// copyPart streams one file part into dst. Read and write failures are
// separated so the caller maps a broken body to 422 and a full disk to
// photo-failed.
func copyPart(dst io.Writer, part *multipart.Part) (int64, error) {
	buf := make([]byte, 64<<10)
	var total int64
	for {
		n, rerr := part.Read(buf)
		if n > 0 {
			w, werr := dst.Write(buf[:n])
			total += int64(w)
			if werr != nil {
				return total, fmt.Errorf("%w: %w", errStageWrite, werr)
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return total, nil
			}
			return total, rerr
		}
	}
}

// readField copies one non-file part's value, bounded at limit bytes. A wider
// part is errFieldTooLong, never a truncated value that could validate.
func readField(part *multipart.Part, limit int) ([]byte, error) {
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(part, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if n > int64(limit) {
		return nil, errFieldTooLong
	}
	return buf.Bytes(), nil
}

// boundaryOf extracts the multipart boundary from the request's Content-Type.
// A missing or malformed header is a transport failure, not a 403: the body
// cannot be split into a token part and a file part (AVT-031).
func boundaryOf(c fiber.Ctx) (string, error) {
	mediaType, params, err := mime.ParseMediaType(string(c.Request().Header.ContentType()))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return "", errNoBoundary
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", errNoBoundary
	}
	return boundary, nil
}

// partError maps a failure while reading the multipart body. The request cap
// is the same 413 an over-declared request gets; every other stop is the
// generic form-invalid page (AVT-031, AVT-041).
func partError(x *Ctx, err error) error {
	switch {
	case errors.Is(err, errAvatarTooLarge):
		// Connection: close is already set; the client may still be sending
		// and the rest must not be drained to reuse the socket.
		return x.server.respondError(x.c, apperr.TooLargef().WithRule("AVT-041"))
	case errors.Is(err, errStageWrite):
		x.server.logger.Error("avatar stage write failed", "rule", "AVT-041", "error", err)
		return x.server.redirectFlash(x.c, profileHome(x), flashPhotoFailed)
	default:
		return respondFormInvalid(x)
	}
}

// respondFormInvalid answers the generic 422 the app already gives a malformed body
// (AVT-093).
func respondFormInvalid(x *Ctx) error {
	return x.server.respondError(x.c, apperr.Field("_form", authoring.MsgFormInvalid))
}

// csrfFailure reissues the CSRF cookie and answers 403 without reading any
// file byte, exactly like the pipeline's own token check (AVT-030).
func csrfFailure(x *Ctx) error {
	_ = x.server.verifyFormCSRF(x.c, x, "")
	return nil
}

// photoFlashFor maps a conversion failure to its one AVT-094 key. The codecs
// return sentinels on purpose, so no message text is ever matched.
func photoFlashFor(err error) string {
	switch {
	case errors.Is(err, avatar.ErrType):
		return flashPhotoType
	case errors.Is(err, avatar.ErrBig):
		return flashPhotoBig
	case errors.Is(err, avatar.ErrUnreadable):
		return flashPhotoUnreadable
	case errors.Is(err, avatar.ErrBusy):
		return flashPhotoBusy
	default:
		// ErrFailed, a cancelled context, and any unexpected fault all mean
		// the same thing to the user: nothing was written, try again.
		return flashPhotoFailed
	}
}

// avatarRemove is POST /profile/photo/remove (AVT-061). It is an ordinary
// urlencoded form through the normal pipeline — declared cap, parse, CSRF,
// must-change, class — so only the write is new here.
func avatarRemove(x *Ctx) (Redirect, error) {
	target := profileHome(x)
	// A build with neither a database nor an injected store has nowhere to
	// write; production always has the former and the HTTP tests inject the
	// latter (AVT-061).
	if x.server.db == nil && x.server.Avatars == nil {
		return Redirect{}, apperr.Internalf(nil).WithRule("AVT-061")
	}
	err := x.server.avatarStore().Remove(x.Context(), x.Session.AccountID)
	switch {
	case err == nil:
		return Redirect{Path: target, Flash: flashPhotoRemoved}, nil
	case errors.Is(err, identity.ErrSessionGone):
		return Redirect{}, x.SeeOther("/login", identity.FlashSessionExpired)
	default:
		// A failed unlink of the old file is already tolerated inside the
		// store; this is a database failure, reported as a retryable flash
		// rather than a dead end (IDN-C26).
		x.server.logger.Error("avatar remove failed", "rule", "AVT-061",
			"account_id", x.Session.AccountID, "error", err)
		return Redirect{Path: target, Flash: flashPhotoRemoveFailed}, nil
	}
}

// serveAvatar answers GET /avatars/:name (AVT-070…AVT-074). The grammar is
// checked before any I/O, then the name must resolve to a live, active owner,
// then the file must be a regular 1…5,000,000-byte file. A miss is 404, never
// a 500 and never a partial image.
func (s *server) serveAvatar(c fiber.Ctx) error {
	name := c.Params("name")
	if !avatarid.Valid(name) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	// A server built without a store has no live row to resolve: the orphan
	// case, not an error page (AVT-070).
	if s.db == nil && s.Avatars == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	live, err := s.avatarStore().HasLiveOwner(c.Context(), name)
	if err != nil {
		s.logger.Error("avatar owner lookup failed", "rule", "AVT-070",
			"name", name, "error", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if !live {
		return c.SendStatus(fiber.StatusNotFound)
	}
	data, status := s.readAvatar(c, s.avatarStore().Dir(), name)
	if status != 0 {
		return c.SendStatus(status)
	}
	return c.Send(data)
}

// readAvatar validates the stored file and writes the 200/304 headers. It
// returns a non-zero status when the caller must answer without a body.
func (s *server) readAvatar(c fiber.Ctx, dir, name string) ([]byte, int) {
	// The grammar guarantees one path segment, so Join cannot escape the root.
	// Lstat refuses a symlink: only a regular file is served (AVT-071).
	full := filepath.Join(dir, name)
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		s.logger.Warn("avatar_missing", "rule", "AVT-071", "name", name)
		return nil, fiber.StatusNotFound
	}
	if info.Size() < 1 || info.Size() > avatar.MaxAvatarBytes {
		s.logger.Warn("avatar_invalid", "rule", "AVT-071", "name", name,
			"bytes", info.Size())
		return nil, fiber.StatusNotFound
	}

	// Read before any header is written, so a read failure is a clean 500
	// rather than a truncated 200 (AVT-073).
	data, err := os.ReadFile(full)
	if err != nil {
		s.logger.Error("avatar read failed", "rule", "AVT-073", "name", name, "error", err)
		return nil, fiber.StatusInternalServerError
	}
	// A file swapped between Lstat and ReadFile is a size drift: 404, never a
	// body that disagrees with what was validated (AVT-071).
	if len(data) < 1 || int64(len(data)) != info.Size() || len(data) > avatar.MaxAvatarBytes {
		s.logger.Warn("avatar_invalid", "rule", "AVT-071", "name", name)
		return nil, fiber.StatusNotFound
	}

	etag := `"` + name + `"`
	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderCacheControl, avatarCacheControl)
	c.Set(fiber.HeaderLastModified, info.ModTime().UTC().Format(http.TimeFormat))
	c.Set(fiber.HeaderContentDisposition, "inline")
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")

	// If-None-Match decides revalidation (AVT-072): a weak tag, a list, or a
	// wildcard all match. If-Modified-Since is deliberately not evaluated.
	if etagMatches(c.Get(fiber.HeaderIfNoneMatch), etag) {
		return nil, fiber.StatusNotModified
	}

	c.Set(fiber.HeaderContentType, "image/webp")
	c.Set(fiber.HeaderContentLength, itoa(len(data)))
	return data, 0
}
