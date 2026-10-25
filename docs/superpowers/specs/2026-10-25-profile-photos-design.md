# Profile photos — design

**Date:** 2026-10-25
**Status:** revision 3, after builder/API and cross-spec checks. The 50 MP product
value is confirmed. Implementation-ready; nothing is built.
**Scope:** a self-uploaded profile photo for both teachers and students, compressed
server-side to WebP, shown wherever a teacher sees a student and wherever a person
sees themselves.
**Rule prefix:** `AVT-###` (new; does not collide with IDN/AUT/PAR/ATT/OPS).
**Related specs:** `2026-10-23-business-logic-01-identity-and-access.md` (accounts,
profile), `2026-10-23-business-logic-02-authoring-and-lifecycle.md` (existing image
upload rules, reused in spirit), `2026-10-23-frontend-data-contract-design.md`
(screens and forms), `2026-10-23-schema-and-migrations-design.md` (migration shape).

---

## 0. Decision summary

| Decision | Value |
|---|---|
| Upload cap | **200,000,000 bytes for the entire multipart request**, checked from `Content-Length` before parsing and before anything is staged; over → `413` |
| Client-side check | none; the server is authoritative, the form only advertises the limit |
| Chunked | rejected before parsing; no stream-counted fallback |
| Lying `Content-Length` | every body reader is independently bounded at its route cap + 1 byte; `c.Body()` never measures a body |
| Pixel ceiling | **50,000,000 pixels**, max side **16,000 px** |
| HEIC | accepted; `go-libheif` HEIC → JPEG (q95) → common decode → WebP |
| WebP codec | `github.com/kolesa-team/go-webp` (cgo, libwebp) |
| Output | 512×512 WebP, quality 80, method 4, **≤ 5,000,000 bytes** |
| Concurrency | 4 goroutine workers, 8 in-flight, channel-based; no shared-state mutexes |
| New routes | `POST /profile/photo`, `POST /profile/photo/remove`, `GET /avatars/:name`, `GET /error/413` |
| Surface pins | 77 → **80** contract routes, 79 → **83** authz classes |
| Build | `CGO_ENABLED=1`; Alpine build/runtime cgo dependencies |

The 200 MB cap is deliberately a **request** cap, not a per-file cap. It is stricter
than a per-file cap and guarantees that no request can make the server read, stage,
decode, or process more than 200 MB. A 200 MB file inside a multipart body therefore
must be a little under 200 MB; that is the only user-visible consequence, and it is
why the help copy says "Maksimal 200 MB" rather than promising an exact-byte file
limit.

---

## 1. Intent

**AVT-001.** A teacher should be able to recognise a student by sight instead of
guessing from a name. The photo is an identification aid, not a social feature.

**AVT-002.** Both roles upload their own photo. Nobody uploads for anyone else,
and no teacher can edit or remove a student's photo. (Product call, 2026-10-25:
"leave it as is".)

**AVT-003.** The photo is optional forever. A missing photo never blocks a quiz,
a registration, a login, a grade, or a result. The initials badge already in the
UI is the permanent fallback.

**AVT-004.** Photos are private in effect even though the file URL is public: the
student UI never renders another student's identity, so a student only sees their
own photo. Teachers see students' and other teachers' photos. Anonymous
participants have no account and therefore no photo.

## 2. Goals and anti-goals

**Goals.**

- One photo per account, both roles, on the existing uploads volume.
- Accept the raw original from the client, up to the 200 MB request cap, including
  HEIC from an iPhone.
- Server-side decode, orientation fix, square crop, downscale, WebP re-encode.
  The client does no processing and no size policing.
- Keep conversion time predictable: reject over the request cap before parsing,
  reject over the pixel cap before decoding pixels, and run conversions on a
  bounded worker pool rather than one at a time or unbounded.
- Never hold the whole request in memory: stream the multipart body to a temp file.
- Replacing or removing a photo never leaves a dangling row or an orphaned file
  that still resolves.
- Every surface that shows a person shows that person's current photo, with
  initials as the fallback.

**Non-goals.** Crop/filter/rotate UI, multiple photos, teacher moderation, photos
for anonymous participants, upload at registration, object storage, on-the-fly
resizing, deleting the file when an account is soft-deleted, client-side JavaScript
validation, preserving EXIF/GPS/original bytes.

**Anti-goals.** A request that can fill the disk or the heap, a decode that can run
for minutes, a quiz page that gets slower because an avatar is missing, stored
originals with GPS metadata, or sideways faces because EXIF was ignored.

## 3. What exists today

- `accounts` holds both roles; `students` holds grade and major. Profile screens
  are `GET/POST /profile` (student) and `GET/POST /teacher/profile` (teacher); the
  write is role-agnostic (`identity.SaveProfile`).
- `viewmodel.CurrentUser` carries `Initials`; `.app-avatar` is a 32 px circle
  defined in `templates/partials/navbar-style.html`.
- Initials render in six chrome spots (both navbars, both drawers, both profile
  headers) and in the live monitor, grading queue, grading detail, results, recap,
  student roster, student detail, teacher directory, and the teacher dashboard's
  `TopScores`.
- `internal/authoring/images.go` is the upload precedent: sniffed MIME, generated
  128-bit names, temp file + fsync + rename, traversal-guarded delete. It does
  **not** re-encode.
- `pipeline.go` parses the form **before** it checks the body cap, and the cap is
  measured with `len(c.Body())`. The 10 MiB `BodyLimit` in `server.go` is what has
  been hiding that ordering.
- `GET /uploads/:name` serves question images after a database check; it is an
  asset route outside the contract-route pin.
- The error taxonomy has no 413. `templates/pages/error/` has 403, 404, 422, 429,
  and 500; each has a preview route under `/error/`.
- The Dockerfile builds `CGO_ENABLED=0`; adding libwebp/libheif makes cgo
  mandatory.
- Conformance pins: 77 method+path pairs and 79 authorization classes. The
  conformance test renders every GET page but has no separate screen-count pin;
  the old “74-screen” comments are stale remnants of the earlier 74-route
  surface.

## 4. Storage

### 4.1 Migration `migrations/0010_profile_photo.up.sql`

```sql
ALTER TABLE accounts
  ADD COLUMN IF NOT EXISTS avatar_name VARCHAR(37)
    CHARACTER SET ascii COLLATE ascii_bin NULL;

ALTER TABLE accounts
  ADD UNIQUE KEY IF NOT EXISTS uq_accounts_avatar (avatar_name, deleted_at);
```

Forward-only and idempotent, matching `0009`. MariaDB 12.3 supports `IF NOT EXISTS`
on both statements, including `ADD UNIQUE KEY IF NOT EXISTS`; a file that stopped
after the column but before the key replays cleanly, and a fully applied file is a
silent no-op. The column is appended at the end of the table, as schema §9.1
requires, so `ALGORITHM=INSTANT` stays available. The migration fixture that pins
the good files gains one.

**AVT-010.** `avatar_name` holds a generated name (`[a-f0-9]{32}.webp`) or `NULL`.
Never a URL, path, MIME type, size, or client filename. The extension is always
`.webp` because the server always re-encodes.

**AVT-011.** The unique key is `(avatar_name, deleted_at)`, matching schema §5.3's
rule that every unique key includes the sentinel. It is the serving index and
forbids two live accounts sharing a file. MariaDB allows many `NULL`s, so most
accounts can have no photo.

**AVT-012.** The column is `ascii_bin`: names are lowercase hex and a dot, and a
case-insensitive collation would let `ABC…` alias a file the grammar rejects.

### 4.2 Files

- Root: `UPLOADS_DIR/avatars/`, mode `0750`, created lazily on first write.
- Name: 16 random bytes from `crypto/rand`, lowercase hex, `.webp` (`internal/avatarid`).
- Write: temp file `.<random>.tmp` in the same directory, `Sync`, close, `Rename`,
  `Chmod 0640`. A crash leaves a dot-prefixed temp file at worst, never a
  half-written avatar under a live name. The `.tmp` suffix matches OPS-237's
  startup cleanup; the leading dot keeps it out of the avatar grammar.
- A changed photo always produces a new name, so the URL changes and no
  cache-busting is needed.
- Storage order: write and fsync the new file → rename to its final name → commit
  the `accounts.avatar_name` update → unlink the old file. If the commit fails, the
  new file is unlinked and the old photo is untouched. If the unlink fails, the old
  name is no longer referenced and its URL 404s; the file is an orphan to collect
  later.
- The output is read into memory (≤ 5 MB) before the response headers are written,
  matching the authoring upload route. A file whose size is zero or above 5 MB is
  treated as a tampered file and is not served.

## 5. The upload path

Two product facts drive this section: the client posts the raw original, and the
server compresses. The server therefore cannot rely on a small body, and the
current pipeline cannot carry a large body as written.

### 5.1 One hard cap, checked before parsing

**AVT-020.** `maxAvatarRequest = 200_000_000` bytes. It applies to the **entire
multipart request**. `prepare` reads the declared `Content-Length` and rejects a
body above the cap **before** parsing, before resolving the session, and before
anything is staged:

```go
declared := c.Request().Header.ContentLength()
if declared < 0 || declared > maxAvatarRequest ||
    (declared == 0 && c.Request().BodyStream() == nil) {
    c.Set(fiber.HeaderConnection, "close")
    s.respondError(c, apperr.TooLargef().WithRule("AVT-020"))
    return nil, false
}
```

The response is `413` with the new 413 page and `Berkas terlalu besar. Maksimal
200 MB.` Closing the connection is deliberate: the client may still be sending, and
the server must not try to drain the rest to reuse the socket.

One transport detail qualifies "before anything is staged": `StreamRequestBody`
makes fasthttp pre-read up to 8 KiB of any positive-length body before it invokes
the handler, and `Expect: 100-continue` additionally answers `100 Continue` first.
The declared check is therefore the first thing the application does, but not
necessarily the first read on the socket. Nothing reaches the staging file, and
the cap reader in 5.3 is bounded at `maxAvatarRequest+1`; a browser form does not
send `Expect`.

**AVT-021.** The check is server-side only. The form's `accept` attribute and the
help text are hints, not enforcement; there is no JavaScript size check, so a
client that bypasses the UI gets the same 413. This is the "easier approach": one
rule, one place, no duplicated client logic.

**AVT-022.** A chunked request has no declared size, so it is rejected before
parsing: `413` on the avatar route, the existing `422`/`400` elsewhere. A request
with neither `Content-Length` nor `Transfer-Encoding` is not a chunked upload:
fasthttp normalises it to an empty body before the handler, exposed as a nil
stream. The avatar route rejects that state as `413` because the form requires a
body; other routes see an empty form. Browsers send `Content-Length` for
multipart forms, so the supported client never hits either branch. The post-parse
`len(c.Body())` check is removed: `c.Body()` reads a streamed body to EOF and must
never be called on the avatar route. Every other body reader is independently
bounded at its cap + 1 byte, so a lying `Content-Length` cannot bypass the cap.
On the avatar route the cap reader treats the extra byte as the same `413` and
`Connection: close` outcome: a client that lies in its declaration is stopped at
the same number as one that declares the truth.

The route-aware cap is:

| Route family | Declared cap | No length (empty body) | Chunked | Over cap |
|---|---|---|---|---|
| `POST /profile/photo` (avatar) | 200,000,000 | `413` | `413` | `413` |
| authoring multipart POSTs | 16 MiB | empty form; route validation decides | `422` (AUT-034) | `422` (AUT-034) |
| ordinary form POSTs | 1 MiB | empty form; route validation decides | `422` generic form-invalid | `422` generic form-invalid |
| `POST /attempt/:id/beacon` | 4 KiB | empty body; `400` | `400` | `400` |

Only the avatar row uses the 413 page. The other rows keep the response the
current route already gives, so this change adds one new failure outcome rather
than reclassifying every form.

**AVT-023.** `server.go` becomes:

```go
BodyLimit:                    maxUploadBody, // 16 MiB, the authoring parser cap
StreamRequestBody:            true,
DisablePreParseMultipartForm: true,
ReadTimeout:                  10 * time.Minute,
IdleTimeout:                  60 * time.Second,
```

`DisablePreParseMultipartForm` is mandatory: without it fasthttp writes multipart
temp files during its own pre-parse, before the avatar handler can require the
CSRF token as the first part. `StreamRequestBody` makes every body a stream; the
first 8 KiB are buffered by fasthttp, the rest is read on demand. In this mode
fasthttp does **not** reject a declared length above `BodyLimit`; it hands the
body over as a stream instead. `BodyLimit` is therefore not the avatar backstop,
and raising it to 200 MB would be wrong twice over: it would not enforce the
avatar cap, and it would raise the multipart parser cap for authoring routes.
`ReadTimeout` is the transfer budget, not the conversion budget.

The caps are enforced by the declared check (AVT-020), the bounded staging copy
(5.3), and a bounded reader on every other route:

- For `application/x-www-form-urlencoded` bodies, `parseForm` stops using
  `PostArgs()`, which calls `bodyBytes()` and reads a streamed body to EOF. It
  copies at most `maxFormBody+1` bytes from `BodyStream()` and reports over-cap
  when the extra byte arrives, then parses with `url.ParseQuery`. Any other
  non-multipart content type (the JSON beacon) is left unread for its handler.
- Authoring multipart keeps `c.MultipartForm()`. It normalises the
  `Content-Type` and passes `BodyLimit` (exactly the 16 MiB authoring cap) to the
  parser, which returns `ErrBodyTooLarge` when a lying length crosses it. That is
  the second reason `BodyLimit` must not be raised to the avatar cap.
- The beacon handler reads at most `maxBeaconBody+1` bytes before
  `attempts.ParseBeacon`; `x.c.Body()` disappears from that route.

**AVT-024.** `prepare` order becomes:

1. path params and query;
2. declared `Content-Length` check against the route cap;
3. `parseForm` for ordinary routes;
4. session resolution;
5. CSRF check for ordinary POSTs;
6. must-change guard;
7. flash take;
8. route class.

Steps 3, 5, and 6 are keyed on `route.Stream`, never on a path literal: a streamed
route owns its body and its in-body token, so the pipeline must not parse a form it
cannot see. `Stream: true` is valid on exactly one route, `POST /profile/photo`;
the route-validation block that already rejects a non-beacon `CSRFExempt` also
rejects `Stream` on any other route. The avatar handler validates the leading CSRF
token itself (5.2), then applies the must-change guard, then reads the file part.
The declared-length check runs first because it reads only the `Content-Length`
header, never the body. Session and class checks still precede every body read, so
an anonymous request is redirected to `/login` and never reaches the multipart
reader. This is the one place where authorization precedes CSRF (OPS-012),
deliberately, because reading the token would mean reading the body of an
unauthorized request.

**AVT-025.** `PageRoute` gains `Stream bool` and `Deadline time.Duration`.
`serveRoute` uses `route.Deadline` when non-zero, otherwise `commandDeadline`
(10 s). The avatar upload is the only route with `Stream: true` and a 10-minute
deadline; the removal route and every other POST keep the 10-second command budget.

**AVT-026.** The handler parses the stream itself:

```go
stream := c.Request().BodyStream() // includes fasthttp's 8 KiB pre-read
bounded := newCapReader(stream, maxAvatarRequest) // sentinel error past the cap
mr := multipart.NewReader(bounded, boundaryFromContentType(c))
```

There is no `Request.MultipartReader` in fasthttp 1.75; the stdlib reader over
`BodyStream()` is the supported path. `BodyStream()` includes the first 8 KiB
fasthttp pre-buffered and reads the rest from the connection; do not fall back to
`c.Body()`. A nil stream with a declared length of 0 means the request carried no
`Content-Length` and no `Transfer-Encoding`; on this route it is the missing-length
case and takes `413` (AVT-022). An explicit `Content-Length: 0` yields a non-nil,
empty stream and takes the `photo-empty` flash. A positive declared length always
has a stream; if it does not, the request is broken and takes the generic `422`.

### 5.2 Parsing order and CSRF

**AVT-030.** The form renders `csrf_token` **before** the file input, and the
handler requires it to be the **first multipart part**. A first part that is not
`csrf_token`, a token that fails the signature check, or a token from another
session is a `403` and nothing is written. Requiring the token first is what makes
the promise real: no attacker-controlled file bytes are read before authentication.
Browsers serialise multipart parts in DOM order, so the UI is deterministic. An
empty multipart body — `NextPart` returns `io.EOF` before any part — is not a token
failure; it takes the `photo-empty` flash of AVT-032.

The token is checked with the same signer comparison the pipeline uses (extracted
into one helper shared by `checkCSRF` and this handler). A failure reissues the CSRF
cookie and answers `403`, exactly as `checkCSRF` does. The token part is capped at
4 KiB and is never logged.

**AVT-031.** Expected fields: `csrf_token` and one `photo` file part. Unknown
fields are ignored up to 16 parts; a second file part, a missing boundary, a
malformed body, or an over-long field is `422` through the existing generic
form-invalid page. The client filename and `Content-Type` are ignored; only the
bytes matter.

**AVT-032.** A zero-byte part, the browser's empty "no file chosen" placeholder, or
a multipart body with no parts at all is a user-fixable miss: `303` back to the
profile with the flash `Pilih foto terlebih dahulu.` An empty body is not a CSRF
attempt; a non-empty first part that is not `csrf_token` remains `403` (AVT-030).

**AVT-033.** After the token is valid, the handler applies the must-change guard:
a `must_change_password=1` session gets the same `303 /change-password` as every
other command, and no file bytes are read. A cross-site POST from a must-change
session therefore fails at the token check with 403, not with a redirect.

Every path that leaves the avatar upload before the body is consumed sets
`Connection: close`, and the route sets it unconditionally before session
resolution, including the success path. Uploads are rare, and the alternative is
trusting that every early return (`403`, busy/must-change `303`, malformed `422`,
anonymous/class redirect) consumed the whole multipart body. fasthttp otherwise
drains an unread request body so it can reuse the keep-alive connection, which
would read the very bytes the rejection was meant to avoid.

### 5.3 Concurrency and bounded staging

**AVT-040.** One channel-based pool, no shared-state mutexes:

- `slots chan struct{}` with capacity **8**. The handler acquires non-blockingly
  **before** reading the source. A full pool answers `303` with
  `Server sedang sibuk. Coba lagi sebentar.` and never reads the file part.
- `jobs chan job` with capacity **8**, consumed by **4 workers**. Each worker
  probes, decodes, crops, scales, and encodes one photo at a time. The handler
  waits on that job's result channel. With 8 slots, 4 jobs are processing and at
  most 4 are queued, so the send never blocks.
- Shutdown uses `sync.Once` to close `stop` once, then a `sync.WaitGroup` joins
  the workers. `jobs` is **never closed**, so a handler can never panic by sending
  on a closed channel. A worker selects on `stop` and `jobs`; a queued job is
  abandoned, while a job already taken runs to completion. A handler also selects
  on `stop`, so a request waiting for a result unblocks immediately during
  shutdown. Its `defer` still removes the temp source file. The process is
  terminating, so a worker whose source was removed during that race simply
  fails the job without writing an avatar.

**AVT-041.** The body stream is wrapped in a cap reader before
`multipart.NewReader`, so every part, field, and boundary scan is bounded:
reading byte `maxAvatarRequest+1` returns a sentinel error that maps to the same
`413` and `Connection: close` as an over-declared request. 8 × 200 MB = 1.6 GB of
temp disk is the absolute worst case, and 8 × a few MB for real phone photos. The
volume must keep at least 2 GB free; the handler maps a write failure to
`photo-failed`, never to a half-written avatar. A disk-space pre-check is not
built in v1.

**AVT-042.** The temp source file is removed on every path — success, bad image,
wrong token, client disconnect — with a single `defer`. A client that disappears
mid-upload is caught by the read error and cleans up the same way. The HEIC
intermediate JPEG is removed by the worker with its own `defer`.

**AVT-043.** A per-IP limiter is deliberately **not** used. A class behind one
school NAT shares an address, and a rate limit would block the thirtieth student
because of the first twenty-nine. The 200 MB cap bounds one request; the 8-slot
pool bounds the process.

**AVT-044.** Peak temp disk is dominated by the request cap, not by the pixel cap.
A 200 MB garbage upload occupies 200 MB of staging until the sniff rejects it; the
source is never kept after the request. Typical storage is 20–40 KB per account
(512×512 WebP).

### 5.4 Decode and compress

**AVT-050.** Sniff the first 512 bytes of the temp file. Accept:

- `image/jpeg` and `image/png` by `http.DetectContentType`;
- `image/webp` by the RIFF/WEBP signature;
- HEIC/HEIF by the ISO-BMFF `ftyp` box and a compatible brand (`heic`, `heix`,
  `hevc`, `heim`, `heis`, `hevm`, `hevs`, `mif1`). These are the brands
  `strukturag/libheif` registers; `hevx` and `msf1` are not accepted in v1. AVIF
  brands (`avif`, `avis`) are rejected in v1; the product copy says "JPG, PNG,
  WebP, atau HEIC".

Anything else, including SVG, GIF, and AVIF, is a `303` flash
`Format foto harus JPG, PNG, WebP, atau HEIC.` The client filename, extension, and
`Content-Type` are never trusted.

**AVT-051.** Pixel ceiling: **50,000,000 pixels** and max side **16,000 px**.
Dimensions are probed before any pixel decode:

| Format | Probe | Cost |
|---|---|---|
| JPEG / PNG | `image.DecodeConfig` | header only |
| WebP | `kolesa/decoder.NewDecoder` + `GetFeatures()` | reads the file (≤ 64 MiB guard) |
| HEIC | `strukturag/libheif` `NewContext` + `ReadFromFile` + `GetPrimaryImageHandle` + `GetWidth/GetHeight` | container metadata only (≤ 64 MiB guard) |

WebP and HEIC are the only formats whose libraries read the whole compressed
file into memory. A source over **64 MiB** is refused with
`Foto terlalu besar untuk diproses.` before either probe. That guard is separate
from the 50 MP ceiling and applies even when the pixel count would be small.

Over the ceiling → `303` flash `Foto terlalu besar untuk diproses.` This is the
memory/time guard; it is not a file-size rule.

The 50 MP default accepts a 48 MP iPhone photo (8064×6048 = 48.8 MP). A 108 MP or
200 MP Android photo is deliberately refused by pixel count even when its file
fits in 200 MB, because decoding it needs roughly 400–800 MB of pixels and several
seconds per image (product decision, §14).

**AVT-052.** JPEG and PNG decode with the standard library. A decode failure is
`303` flash `Foto tidak dapat dibaca. Coba file lain.`

**AVT-053.** WebP is decoded by `github.com/kolesa-team/go-webp/decoder`
(`NewDecoder(r, nil)` reads the whole input, so a **64 MiB** input guard runs
first). `GetFeatures()` gives the dimensions for AVT-051; `Decode()` returns
`*image.NRGBA`. A WebP original over 64 MiB is refused with the same "too big to
process" flash before decoding; a 64 MiB WebP avatar is not a real use case.

**AVT-054.** HEIC path:

1. Refuse a source over **64 MiB** before `go-libheif` reads it all. Then probe
   dimensions with `strukturag/libheif` (AVT-051); `go-libheif` exposes no
   dimension probe, but it already depends on that package. No pixel decode yet.
2. `go-libheif.HeifToJpeg(src, tmpJPEG, 95)`; the library opens the file, decodes
   it through `image.Decode` (registered by the blank-imported
   `strukturag/libheif/go/heif`, whose decoder applies the container's
   `irot`/`imir` transforms), and re-encodes it as JPEG at quality 95.
3. Decode `tmpJPEG` with the standard JPEG decoder and feed the common crop/scale
   path. The intermediate JPEG has no EXIF, so there is no double rotation.
4. Delete `tmpJPEG` on every path.

This is two pixel decodes (HEIC → image, then JPEG → image) plus one JPEG encode.
That is the cost of the requested HEIC → JPEG → WebP path; quality 95 keeps the
double loss invisible at 512 px. The `libheif` HEVC decoder is present in Alpine's
`libheif` (it depends on `libde265`), so an iPhone HEIC decodes in the runtime
image. A HEIC that libheif cannot read is `photo-unreadable`, not `photo-type`.

**AVT-055.** Apply EXIF orientation **before** cropping. A small in-repo reader
handles the JPEG APP1 `0x0112` tag and the WebP `EXIF` RIFF chunk (both endian
TIFF variants, bounds-checked, malformed EXIF ignored). Orientations 1–8 are
supported (identity, flips, 90/180/270 rotations, transpose/transverse). The
output has no EXIF, so the stored WebP is upright and metadata-free. PNG `eXIf` is
ignored in v1; phone PNGs rarely carry orientation and a PNG with `eXIf` is still
read correctly, just not auto-rotated.

**AVT-056.** Centre-crop the square (short side), then scale to `512×512` with
`golang.org/x/image/draw` CatmullRom. Composite alpha onto white: fill the
destination `*image.NRGBA` with white, then `draw.CatmullRom.Scale(..., draw.Over)`
from the source crop. The result is opaque; no alpha survives into the WebP.

**AVT-057.** Encode with `github.com/kolesa-team/go-webp/encoder`:

```go
opts, _ := encoder.NewLossyEncoderOptions(encoder.PresetPhoto, 80)
opts.Method = 4
enc, _ := encoder.NewEncoder(canvas, opts) // canvas is *image.NRGBA
var buf bytes.Buffer
err := enc.Encode(&buf)
```

`NewEncoder` accepts any `image.Image` and converts a non-NRGBA source to NRGBA
internally; the canvas is already `*image.NRGBA`, so that conversion is skipped.
No other WebP encoder is linked. `Method` 4 is the quality/speed sweet spot for a
512 px avatar.

**AVT-058.** The output must be **≤ 5,000,000 bytes**. At 512×512 it is 15–40 KB,
so the ceiling is asserted rather than approached. If exceeded, re-encode at
quality 60; if it still fails, flash `Foto gagal disimpan. Coba lagi.` and write
nothing.

**AVT-059.** Conversion time budget on the reference 2-vCPU container: 12 MP JPEG
≈ 0.3–0.8 s; 48 MP JPEG ≈ 1.5–3 s; 48 MP HEIC ≈ 3–6 s (two decodes); the 512 px
WebP encode is ≈ 10 ms. Four workers keep four uploads moving; the fifth through
eighth wait behind a worker, and the ninth gets the busy flash before its file part
is read. The caps, not a wall-clock timeout, bound the work: a cgo decode cannot be
pre-empted safely, so the design refuses over-limit work before it starts rather
than pretending a context can kill it.

## 6. Routes, authorization, and 413

**AVT-060.** `POST /profile/photo` — class `any-auth`, `Stream: true`,
`Deadline: 10m`, `HandlerCtx`, no template. It is not `CSRFExempt` (that flag
stays reserved for the beacon); `Stream` is what routes it around the pipeline's
form parse and token check, and the handler validates the token per AVT-030. One
handler for both roles. Success redirects to `/profile` (student) or
`/teacher/profile` (teacher) with `Foto profil diperbarui.`

**AVT-061.** `POST /profile/photo/remove` — class `any-auth`, ordinary `Act`, 1 MiB
declared cap. A separate urlencoded form, so "remove" never competes with a
selected file in one multipart body. Success redirects with
`Foto profil dihapus.` Removing a photo that does not exist succeeds (idempotent).

**AVT-062.** `GET /avatars/:name` — class `public`, an asset route registered like
`GET /uploads/:name`, outside the contract-route pin. The handler does its own
live-account check; there is no session work.

**AVT-063.** `GET /error/413` — class `public`, a preview page registered with the
other error previews. It renders the same template and copy as a real 413.

**AVT-064.** Surface pins move together:

- 77 → **80 pinned** contract method+path pairs (the two POSTs and `/error/413`);
  the audited OPS-220 surface is 83 including `/healthz`, `/uploads/:name`, and
  `/avatars/:name`.
- 79 → **83** authorization classes (the 80 pinned routes plus those three
  operational/asset routes).

`authz.go`'s "74 pinned routes" comment and `conformance_test.go`'s "74-route
surface" and 77-entry comments are stale even before this change. Update them to
**80 pinned pairs** and **83 classes** in the same commit. There is no independent
GET-screen count to move.

**AVT-065.** The shared upload handler always writes to the session account. There
is no id in either route. Anonymous requests get the pipeline's normal
`303 /login`; a must-change session is redirected by the handler after the token
check.

**AVT-066.** 413 taxonomy:

- `apperr.TooLarge` is a new kind after `RateLimited`, with `Status() == 413`,
  `String() == "too_large"`, and `TooLargef()` returning `MsgTooLarge`.
- `MsgTooLarge = "Berkas terlalu besar. Maksimal 200 MB."`
- `errorTemplate` maps it to `pages/error/413.html`;
  `errorPage` gives it the title `Berkas terlalu besar` and `HomeHref: "/"`.
  Today `errorPage` hardcodes `/login`; the mapping is per-kind so the 413 page
  and its preview share the action, and an already-signed-in uploader is not sent
  back to a login screen.
- `templates/pages/error/413.html` mirrors the other error pages with a short
  "cara memperbaikinya" list (reduce the file, keep it under 200 MB, retry) and a
  neutral `Kembali` action pointing at `/`. It does not use the 404 page's
  "Kembali ke halaman masuk" label, because a signed-in uploader is already past
  the login screen.
- The `apperr` package comment changes from "exactly four user-visible outcomes"
  to five.
- The OPS prose moves with it: OPS-011 (status table), OPS-200 (taxonomy), OPS-201
  (non-303 POST outcomes), OPS-217 (preview parity), OPS-218 (request caps),
  OPS-220 (audited surface), OPS-230/§13 (ten migrations and 83 routes), and
  OPS-262 (the mechanical count). OPS-220 now reads **83 = 80 pinned +
  `/healthz` + `/uploads/:name` + `/avatars/:name`**.

## 7. Serving files

**AVT-070.** `GET /avatars/:name` accepts only `^[a-f0-9]{32}\.webp$`, then
verifies a live account still references it:

```sql
SELECT 1 FROM accounts
 WHERE avatar_name = ? AND deleted_at = ? AND status = 'active'
 LIMIT 1
```

with `store.LiveAt` as the `deleted_at` parameter. A malformed name is `404` before
any I/O. A name with no live, active owner is `404`, so a soft-deleted, pending, or
replaced photo stops resolving as soon as the row changes. The unique key on
`avatar_name` makes this one indexed lookup. One query per avatar request is
acceptable: browsers cache the image, and the extra query is far cheaper than a
decode.

**AVT-071.** The file must be a regular file (Lstat, no symlink following), size
between 1 and 5,000,000 bytes. Missing, non-regular, empty, or oversize is `404`
and logged once (`avatar_missing` / `avatar_invalid`), never a 500.

**AVT-072.** Headers: `Content-Type: image/webp`, `Cache-Control: private,
max-age=31536000, immutable`, `ETag: "<name>"`, `Last-Modified` from mtime,
`Content-Disposition: inline`, `X-Content-Type-Options: nosniff`, explicit
`Content-Length`. `If-None-Match` (weak comparison, list, or `*`) returns `304`.
`If-Modified-Since` is not evaluated in v1: every cacheable response carries an
`ETag`, and a client that has one sends `If-None-Match`, which takes precedence
(RFC 9110 §13.1.3). `Last-Modified` remains informational for operators and
proxies. A new name on every upload makes `immutable` safe. `private` keeps shared
caches from storing student photos.

**AVT-073.** The file is read fully before headers are written, so a read failure
is a clean 500 and a truncated 200 is impossible. No range requests, no transform,
no directory listing.

**AVT-074.** `/avatars/` reads only `UPLOADS_DIR/avatars/`; `/uploads/` keeps
reading only the authoring images. The temp names are dot-prefixed and fail the
grammar, so they cannot be requested even if a crash leaves one behind.

## 8. View layer

**AVT-080.** `viewmodel.Avatar{URL, Initials, Class}` plus:

```go
func NewAvatar(name, fullName, class string) Avatar
func AvatarURL(name string) string // "" for an empty/malformed name
```

`Class` is an optional, server-chosen extra CSS class (never user data): the
monitor passes `monitor-avatar` to keep its existing 36 px styling, roster and
result rows pass `app-avatar--sm`, and a profile header may pass
`app-avatar--lg`. `AvatarURL` validates with `internal/avatarid` before composing
`/avatars/…`, so a bad database value can never become a bad URL.

`authn.Session` gains `AvatarName`; the session query selects `a.avatar_name` from
the `accounts` join it already has. `CurrentUser()` maps that name through
`AvatarURL`; `CurrentUser` gains `AvatarURL` and a method `Avatar() Avatar` that
pairs it with `Initials` and the empty class. The chrome needs no extra query.
Rows gain an `Avatar Avatar` field filled in the same query that already joins
`accounts`.

**AVT-081.** New partial `templates/partials/avatar.html`:

```html
<span class="app-avatar{{with .Class}} {{.}}{{end}}{{if .URL}} app-avatar--photo{{end}}" aria-hidden="true">
  {{- if .URL}}<img src="{{.URL}}" alt="" width="32" height="32" loading="lazy" decoding="async">{{else}}{{.Initials}}{{end -}}
</span>
```

The name is always visible next to it, so `alt=""` is correct. The initials
variant stays `aria-hidden`. `.app-avatar--photo` adds `overflow:hidden` and
`img { width:100%; height:100%; object-fit:cover; display:block }`; the 28 px table
variant is `.app-avatar--sm`, and the profile header may use `.app-avatar--lg`. The
monitor passes `Class: "monitor-avatar"` so its existing 36 px rule still applies.
Styles go in the shared style partial (`ui-style.html` or the existing
`navbar-style.html`), never inline in a page.

**AVT-082.** The fallback is server-rendered only: the partial emits initials when
the server knows there is no photo. There is no `onerror` handler (banned by the
conformance scan) and no JavaScript dependency. An image that vanishes between
render and fetch is an operational 404, not a state the UI models.

**AVT-083.** Photos appear in this order of importance:

| Surface | Model / query | Notes |
|---|---|---|
| Both navbars + both drawers | `CurrentUser` | replaces the initials badge |
| `GET /profile`, `GET /teacher/profile` | `CurrentUser` | header card doubles as the preview |
| `GET /teacher/students` roster | `StudentRow` + `a.avatar_name` | 28 px, before the name |
| `GET /teacher/students/:id` | `StudentForm` | header; the new-student form has no avatar |
| `GET /teacher/teachers` directory | `TeacherRow` | active rows; pending rows keep initials |
| Live monitor rows + SSE | `ParticipantRow` / `participantSnapshot` | 36 px `monitor-avatar`; SSE `avatar` is the URL string (AVT-084) |
| `GET /teacher/grading` queue | `GradingRow` | 28 px |
| Grading detail | `EssayAnswer` + `AttemptHeader` | 32 px; teacher sees the student |
| `GET /teacher/quizzes/:id/results` | `ResultsRow` | 28 px |
| Teacher recap | `RecapAttempt` | 32 px |
| Teacher dashboard `TopScores` | `QuizTopScore` | 28 px; anonymous rows keep initials |
| Student's own attempt, result, review | `AttemptHeader` | own photo from the already-joined account |

Every query that already joins `accounts a` adds `a.avatar_name`; no per-row query
and no N+1. `AttemptHeader` carries the participant's avatar in both the
student-own pages and the teacher's grading detail: `attempts/read.go` already
joins `accounts`, and the grading literal is built from the same join. Anonymous
participants have no account, so their rows keep initials.

**AVT-084.** The monitor's full-row payloads gain `avatar`: `streamParticipant`
(used by `snapshot` and `participant.added`) carries
``Avatar string `json:"avatar"` ``, the same value as `Avatar.URL` and `""` when
there is no photo. The `participant.updated` payload keeps its ATT-194 subset and
deliberately carries no identity fields, so it carries no `avatar` either.
`diffMonitorSnapshot` compares `Avatar.URL` alongside `Name`, `Initials`, and
`ClassLabel`; a change forces a full `snapshot` rather than an incremental update.
In `static/js/monitor.js`, `applySnapshot` compares `participant.avatar` with the
`src` of the row's `.app-avatar img` (or with its absence); a mismatch calls the
existing `scheduleReload()`, so the server render supplies the new `<img>`. There
is no new DOM-patch branch and no client-side initials fallback. The initial HTML
and the SSE contract therefore stay the only renderers of the avatar cell.

**AVT-085.** No layout shift: the span keeps its size in both variants, the image
is square, and the `width`/`height` attributes match. The image fills the circle
with `object-fit: cover`.

## 9. Forms and copy

**AVT-090.** Each profile page gains a photo card with its own
`<form method="post" action="/profile/photo" enctype="multipart/form-data">` and
`csrf_token` as the **first** field. It is not nested in the name/email form, which
stays urlencoded and untouched.

**AVT-091.** Fields: file input (`name="photo"`,
`accept="image/jpeg,image/png,image/webp,image/heic,image/heif,.heic,.heif"`), help
`Foto dipotong persegi dan diperkecil otomatis. Maksimal 200 MB.`, submit
`Unggah foto`. The file input must have an accessible label; no `data-validate` or
client-side size check.

**AVT-092.** Removing a photo is a second, separate urlencoded form posting
`/profile/photo/remove`, rendered only when a photo exists. Its button is
`Hapus foto` and it carries the same CSRF token.

**AVT-093.** No client-side crop or preview in v1. The photo appears after the
redirect. Every user-fixable failure is a `303` back to the profile with a flash;
the file input is never repopulated, which is the same limitation authoring
already documents. Transport-level garbage (malformed multipart, missing
boundary) keeps the generic `422` form-invalid page. An over-cap request gets the
413 page, not a flash, because the server has not read the form.

**AVT-094.** Copy, Indonesian, reused verbatim by handlers and tests:

| Key | Text | Kind |
|---|---|---|
| `photo-empty` | `Pilih foto terlebih dahulu.` | error |
| `photo-type` | `Format foto harus JPG, PNG, WebP, atau HEIC.` | error |
| `photo-big` | `Foto terlalu besar untuk diproses.` | error |
| `photo-unreadable` | `Foto tidak dapat dibaca. Coba file lain.` | error |
| `photo-failed` | `Foto gagal disimpan. Coba lagi.` | error |
| `photo-busy` | `Server sedang sibuk. Coba lagi sebentar.` | error |
| `photo-remove-failed` | `Foto gagal dihapus. Coba lagi.` | error |
| `photo-saved` | `Foto profil diperbarui.` | success |
| `photo-removed` | `Foto profil dihapus.` | success |

`viewmodel.FlashFor` gains exactly these keys. A key not in the set stays ignored,
as today. The 413 page copy is `Berkas terlalu besar. Maksimal 200 MB.` from
`apperr.MsgTooLarge`, not a flash key.

**AVT-095.** The form works with JavaScript off: it is a plain multipart POST. The
`accept` attribute is an upload-dialog hint only; the server decides by magic
bytes.

## 10. Failure modes

| Case | Result |
|---|---|
| No file chosen | `303` + `photo-empty` |
| Empty multipart body (no parts) | `303` + `photo-empty` (not `403`) |
| 200,000,000-byte request or less | accepted; staged, sniffed, capped by pixels |
| `Content-Length` > 200,000,000 | `413` page, connection closed, nothing staged |
| Chunked | `413` on the avatar route, before parsing |
| No `Content-Length` and no `Transfer-Encoding` | `413` on the avatar route (empty body); an explicit `Content-Length: 0` takes `photo-empty` |
| 100 MB JPEG | accepted if ≤ 50 MP; stored as a ~25 KB WebP |
| 60 MP image | `303` + `photo-big` |
| 108 MP / 200 MP phone photo | `303` + `photo-big` (see §14) |
| WebP original > 64 MiB | `303` + `photo-big` before decode |
| HEIC original > 64 MiB | `303` + `photo-big` before libheif reads it |
| HEIC from an iPhone | accepted, HEIC → JPEG 95 → WebP |
| HEIC that libheif cannot decode | `303` + `photo-unreadable` |
| AVIF / SVG / GIF / text | `303` + `photo-type` |
| File part before the CSRF token | `403`, nothing written |
| Wrong/expired token | `403`, CSRF cookie reissued, nothing written |
| Staging pool full | `303` + `photo-busy`, file part not read |
| Disk full during staging | temp removed, `303` + `photo-failed` |
| Client disconnects mid-upload | read error, temp removed, no DB write |
| DB write fails after the new file is written | new file removed, old avatar untouched |
| Unlink of the old file fails | logged; the old name is unreferenced, so its URL 404s |
| `avatar_name` points at a missing/tampered file | `404` + log, never a 500 or a partial image |
| Photo of a non-face | nothing; there is no moderation by design (AVT-002) |
| Malformed multipart | `422` generic form-invalid page |
| Process crashes between rename and commit | orphan file, no dangling row; collect later |

## 11. Tests

**Compression unit — `internal/avatar/processor_test.go` (cgo).**

- JPEG → 512×512 WebP, under 5 MB, starts `RIFF....WEBP`.
- Portrait and landscape sources both give a centred square.
- EXIF orientations 1, 3, 6, 8 (and 2/4/5/7 if cheap) come out upright.
- PNG with alpha composites onto white; the decoded output is opaque.
- WebP input round-trips through decode + re-encode; EXIF-bearing WebP is upright.
- HEIC fixture → WebP at 512×512; a rotated HEIC fixture is upright (libheif
  transforms applied; no double rotation).
- 51 MP image, a side > 16,000 px, a 65 MiB WebP, and a 65 MiB HEIC are rejected
  with the right error before pixel decode.
- Text, SVG, GIF, AVIF, and zero bytes are rejected with the right error.
- A 12 MB JPEG is accepted.
- The output cap is enforced; a forced oversize encode fails without writing.

**Pool — `internal/avatar/pool_test.go`.**

- 4 workers process 4 jobs concurrently; the 5th waits, the 9th slot is refused.
- `Close` closes `stop` once, abandons queued jobs, unblocks waiters, joins the
  workers, and leaks no goroutines; `jobs` is never closed and no send panics.
- `-race` is clean; temp files are gone after every success/failure path.

**Identity store — `internal/identity/avatar_store_test.go`.**

- Replace writes the new file, commits the name, unlinks the old.
- A failed transaction removes the new file and leaves the column unchanged.
- Remove clears and unlinks; removing nothing succeeds.
- Generated names match the grammar and are unique across 1,000 calls.

**Pipeline — `internal/httpserver/pipeline_test.go`.**

- A 12 MB urlencoded form is rejected before parsing (declared length).
- A chunked POST to a normal route is rejected; a chunked avatar POST is 413.
- A bodyless avatar POST is 413; an explicit `Content-Length: 0` is the empty
  upload flash.
- A body whose declared length lies (under the cap, stream over it) is stopped at
  cap + 1 on the avatar, form, authoring, and beacon routes.
- A 12 MB multipart to the authoring route is under the 16 MiB transport guard and
  still gets the 10 MiB image rule.
- The avatar route alone accepts a 12 MB body.
- A declared 200,000,001-byte avatar POST is 413 before anything is staged.
- Existing multipart authoring tests still pass with
  `DisablePreParseMultipartForm` on (this is the regression that matters).
- The upload route's 10-minute deadline does not leak into other POSTs.

**Avatar HTTP — `internal/httpserver/avatar_http_test.go`.**

- Student upload: `303 /profile`, flash `photo-saved`, column and file agree.
- Teacher upload: `303 /teacher/profile`.
- Missing token, token after the file part, and a token from another session are
  each `403` with no temp file left behind; no file bytes are written before the
  token is validated.
- Must-change session with a valid token: `303 /change-password`; with a bad token:
  `403`.
- Empty part, an empty multipart body with no parts, text bytes, an oversized-pixel
  image, an undecodable HEIC, and a full pool give the documented flash and status.
  The no-parts body is `photo-empty`, not `403`.
- A declared over-cap body gets the 413 page and `Connection: close`; the real page
  and `GET /error/413` carry identical copy and `HomeHref: "/"`.
- Remove clears the column, unlinks, and renders initials.
- `GET /avatars/:name`: `image/webp` + `private, immutable`, `304` on
  `If-None-Match`, `200` for `If-Modified-Since` alone, `404` for bad grammar,
  orphan name, soft-deleted owner, directory, symlink, empty file, and oversize
  file.
- No dot-prefixed temp file survives any failure path.

**Render.** Profile pages show initials with no photo and one `<img src="/avatars/…">`
with one, never both. Roster, student detail, monitor, results, grading, directory,
recap, and `TopScores` rows carry the photo when set; anonymous rows keep initials.
The monitor SSE payload carries `avatar` and the change detector notices a change.

**Conformance.** Route-surface, authorization-class, template-parse, error-preview,
and CSS/JS ban tests move in the same commit: 77 → 80 pairs, 79 → 83 classes, one
new migration, one new error template.

## 12. Operations and build

**AVT-100.** About 20–40 KB per account; a 1,000-account school is well under
50 MB on the volume that already holds question images. Back up
`UPLOADS_DIR/avatars/` with the uploads volume.

**AVT-101.** Peak memory is dominated by decoded pixels, not by file size. A
50 MP JPEG decodes to roughly 75–200 MB depending on subsampling; a 50 MP PNG or
WebP is ~200 MB NRGBA; HEIC is the worst case because HEIC decode + JPEG encode +
JPEG decode can overlap, so budget ~300–450 MB per HEIC job. Four workers can
therefore reach ~1–2 GB. Run the container with at least 2 GB of memory, 4 GB
recommended, or lower the worker count in code if the deployment is smaller. The
pool is the hard bound; the pixel cap is the per-job bound.

**AVT-102.** The 200 MB request cap, 50 MP ceiling, 64 MiB WebP guard, 64 MiB
HEIC guard, and 5 MB output ceiling are logged as structured warnings with the
account id and measured values. No request body, client filename, or image bytes
are logged.

**AVT-103.** No new environment variables. `UPLOADS_DIR` keeps its meaning; the
avatar subdirectory is derived. The 200 MB and 50 MP values are constants, not
configuration, so the documented behaviour cannot drift per deployment.

**AVT-104.** Docker:

```dockerfile
FROM golang:1.26-alpine3.22 AS build
RUN apk add --no-cache build-base pkgconf libwebp-dev libheif-dev
# ...
RUN CGO_ENABLED=1 go build -o /out/server ./cmd/server \
 && CGO_ENABLED=1 go build -o /out/migrate ./cmd/migrate

FROM alpine:3.22
RUN apk add --no-cache libwebp libheif
# ...
```

Build and runtime Alpine versions must match (`3.22`) so the musl/libwebp/libheif
sonames agree. Alpine 3.22's `libheif` 1.19.8 depends on `libde265` and `libx265`,
so HEVC HEIC decode works in the runtime image. `libheif-dev` is in `community`.
The binary is dynamically linked; `ldd /app/server` must show `libwebp.so.7` and
`libheif.so.1`. The image grows by roughly 20–30 MB.

**AVT-105.** Windows dev: cgo requires a C toolchain and pkg-config with libwebp
and libheif. Use MSYS2 MinGW64 (`mingw-w64-x86_64-gcc`,
`mingw-w64-x86_64-pkgconf`, `mingw-w64-x86_64-libwebp`,
`mingw-w64-x86_64-libheif`) with `CGO_ENABLED=1`, or use WSL/Docker. A bare
Windows shell cannot run `go test ./...`; `CGO_ENABLED=0` builds fail by design.
CI runs on Linux with `libwebp-dev` and `libheif-dev`.

**AVT-106.** Orphan files can exist after a crash between rename and commit, or
after a failed unlink. They are unreferenced and therefore unservable. A one-shot
reconciliation (list `avatars/*.webp`, subtract live `avatar_name` values, delete
older than a day) is an ops task, not built into the request path.

**AVT-107.** Both asset routes go through the app, so there is no directory
listing and no MIME guessing. The avatar route never serves a file that no live
account references.

## 13. Left to the builder

- Exact package split between `internal/avatar` (pool, decode, crop, encode, EXIF)
  and `internal/identity` (the only writer of `accounts.avatar_name` and the
  avatars directory). The invariants in §4.2 and §5 are fixed; the file layout is not.
- `golang.org/x/image/draw` versus a hand-rolled sampler for crop and scale, if the
  filter is at least CatmullRom.
- The EXIF reader (in-repo parser or dependency), if it reads only orientation and
  never fails a valid photo.
- Where the photo card sits on each profile page, and the exact `.app-avatar--lg` /
  `--sm` sizes, subject to `DESIGN.md`.
- How `monitor.js` detects an avatar change (compare the SSE value with the
  rendered image or a row data attribute), provided it uses the existing reload
  path and has no `onerror` handler.
- How the HEIC test fixture is produced (generated once with `heif-enc` or
  `ImageToHeif` and committed under `testdata/` is fine).
- Whether the pipeline's declared-length helper lives in `checkBodyCap` or a
  sibling, provided the order (declared length → parse) is pinned by a test, the
  avatar staging copy is bounded at cap + 1, every other body reader is bounded,
  and no route calls `c.Body()` to measure or parse a body.
- How the handler tells an explicit `Content-Length: 0` (the empty-upload flash)
  from a request with neither `Content-Length` nor `Transfer-Encoding` (413),
  provided a probe test pins both. Do not infer the distinction from a parsed
  integer alone; if fasthttp collapses the two, read the raw header presence or
  document the collapse and make both `photo-empty`.

## 14. Product confirmation

**50 MP default — confirmed 2026-10-25.** This design ships **50,000,000 pixels**.
It accepts every current iPhone photo (48 MP) and typical Android photos, while
keeping one decode under ~200 MB and a few seconds. A 108 MP or 200 MP phone photo
is deliberately refused with `Foto terlalu besar untuk diproses.` even though its
file fits in 200 MB: the request cap bounds the upload, and the pixel cap bounds
the decode. The product decision is to keep the ceiling. If the school later
adopts such phones, the options are to raise the ceiling and the container memory,
or to add a cgo JPEG downscaler before the full decode; neither is v1. No other
product value is open.

**200 MB is a request cap.** A file at exactly 200 MB cannot fit with multipart
overhead; the form says "Maksimal 200 MB", which is close enough for users and
strict for the server. If an exact 200 MB *file* must be accepted, the request cap
would need to be 200 MB plus a small overhead, weakening the "processed data ≤
200 MB" guarantee. The current choice keeps the guarantee.

**No client-side check.** The server rejects over-cap requests from the first
header, so the browser does not have to upload the whole file. A JavaScript check
would improve the message for honest users but would duplicate the rule and could
drift. If the product wants an instant client-side warning before the request
leaves the browser, add it later as a pure UX hint; the server remains
authoritative.
