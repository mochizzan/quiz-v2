//go:build cgo

package avatar

// End-to-end tests for the cgo half of the pipeline (AVT-053..AVT-058):
// real libwebp encode/decode and libheif probing, driven through the
// public Processor/Slot API. Under CGO_ENABLED=0 this file is excluded and
// the pure-Go suite above still covers the sniff, the ceilings, the EXIF
// reader, the transform, the encode policy, and the pool.

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// splitImage is the orientation fixture: 400×200, left half red, right
// half blue.
func splitImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				img.SetNRGBA(x, y, bandRed)
			} else {
				img.SetNRGBA(x, y, bandBlue)
			}
		}
	}
	return img
}

// solidImage fills an NRGBA with one colour.
func solidImage(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// jpegWithOrientation encodes img and injects an EXIF APP1 orientation
// segment straight after SOI — the shape a phone camera actually emits
// (AVT-055).
func jpegWithOrientation(t *testing.T, img image.Image, orientation uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	app1 := exifApp1(t, orientation)
	out := make([]byte, 0, 2+len(app1)+len(b)-2)
	out = append(out, b[0], b[1]) // SOI
	out = append(out, app1...)
	out = append(out, b[2:]...)
	return out
}

// webpWithOrientation encodes a WebP through the package encoder and
// appends an EXIF RIFF chunk (fixing the RIFF size), the WebP twin of the
// JPEG APP1 above (AVT-055).
func webpWithOrientation(t *testing.T, img *image.NRGBA, orientation uint16) []byte {
	t.Helper()
	encoded, err := encodeWebP(img, 90)
	if err != nil {
		t.Fatal(err)
	}
	chunk := riffChunk("EXIF", append([]byte("Exif\x00\x00"), buildTIFF(true, orientation, 3)...))
	out := append(append([]byte{}, encoded...), chunk...)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out
}

// runConversion stages data under name and runs it through the public path
// — TryAcquire, Slot.Process — exactly as the HTTP handler will. It
// returns the stored bytes on success and the sentinel error on failure,
// asserting that a failed conversion writes nothing.
func runConversion(t *testing.T, p *Processor, staging, name string, data []byte) ([]byte, error) {
	t.Helper()
	src := filepath.Join(staging, name)
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(p.dir, name+".webp")
	s, err := p.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	defer s.Release()
	convErr := s.Process(context.Background(), src, dst)
	if convErr != nil {
		if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
			t.Errorf("failed conversion left %s behind", dst)
		}
		return nil, convErr
	}
	stored, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	assertNoAvatarTemps(t, p.dir)
	return stored, nil
}

// mustProcess runs a conversion that is expected to succeed.
func mustProcess(t *testing.T, p *Processor, staging, name string, data []byte) []byte {
	t.Helper()
	stored, err := runConversion(t, p, staging, name, data)
	if err != nil {
		t.Fatalf("Process(%s): %v", name, err)
	}
	return stored
}

// decodeStoredWebP checks the container contract of the stored avatar —
// RIFF/WEBP magic and the size ceiling (AVT-057/058) — and decodes it back
// to pixels through the package's own libwebp path.
func decodeStoredWebP(t *testing.T, data []byte) image.Image {
	t.Helper()
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		t.Fatalf("stored avatar does not start with RIFF/WEBP: %q", data[:min(12, len(data))])
	}
	if len(data) > MaxAvatarBytes {
		t.Errorf("stored avatar is %d bytes, over the %d ceiling", len(data), MaxAvatarBytes)
	}
	tmp, err := os.CreateTemp("", "avatar-stored-*.webp")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}
	img, err := decodeWebP(name)
	if err != nil {
		t.Fatalf("decoding the stored avatar: %v", err)
	}
	if b := img.Bounds(); b != (image.Rect(0, 0, OutputSize, OutputSize)) {
		t.Fatalf("stored avatar bounds = %v, want %dx%d", b, OutputSize, OutputSize)
	}
	return img
}

// assertHue checks that one channel dominates both others by a healthy
// margin (JPEG and WebP are lossy; dominance survives, exact values do
// not) and that the pixel is opaque.
func assertHue(t *testing.T, img image.Image, x, y int, want byte, what string) {
	t.Helper()
	b := img.Bounds()
	c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
	if c.A != 255 {
		t.Errorf("%s at (%d,%d): alpha = %d, want 255", what, x, y, c.A)
	}
	r, g, bl := int(c.R), int(c.G), int(c.B)
	dominates := func(v, a, o int) bool { return v >= 120 && v-a >= 60 && v-o >= 60 }
	var ok bool
	switch want {
	case 'R':
		ok = dominates(r, g, bl)
	case 'G':
		ok = dominates(g, r, bl)
	case 'B':
		ok = dominates(bl, r, g)
	}
	if !ok {
		t.Errorf("%s at (%d,%d) = %v, want channel %c to dominate", what, x, y, c, want)
	}
}

// TestNewContract: New rejects an empty directory, does not touch the
// filesystem, and hands back a usable Processor joined by Close.
func TestNewContract(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Error(`New("") must fail`)
	}
	dir := filepath.Join(t.TempDir(), "not-created-yet")
	p, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("New must create the directory lazily (stat: %v)", err)
	}
}

// TestProcessJPEGEndToEnd: a banded JPEG becomes a 512×512 WebP whose
// centre is the source's centre band — sniff, probe, decode, crop, scale,
// and encode all at once (AVT-050..AVT-058).
func TestProcessJPEGEndToEnd(t *testing.T) {
	landscape := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	fillBands(landscape, [2]int{100, 300}, true)
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, landscape, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}

	p, err := New(filepath.Join(t.TempDir(), "avatars"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	staging := t.TempDir()

	img := decodeStoredWebP(t, mustProcess(t, p, staging, "photo.jpg", jpg.Bytes()))
	for _, pt := range [][2]int{{40, 40}, {256, 256}, {470, 470}, {40, 470}, {470, 40}} {
		assertHue(t, img, pt[0], pt[1], 'G', "landscape centre crop")
	}
}

// TestProcessPortraitCropsOnShortSide: the short side is height for a
// portrait, so the crop keeps the middle horizontal band.
func TestProcessPortraitCropsOnShortSide(t *testing.T) {
	portrait := image.NewNRGBA(image.Rect(0, 0, 200, 400))
	fillBands(portrait, [2]int{100, 300}, false)
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, portrait, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}

	p, err := New(filepath.Join(t.TempDir(), "avatars"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	img := decodeStoredWebP(t, mustProcess(t, p, t.TempDir(), "portrait.jpg", jpg.Bytes()))
	for _, pt := range [][2]int{{40, 40}, {256, 256}, {470, 470}} {
		assertHue(t, img, pt[0], pt[1], 'G', "portrait centre crop")
	}
}

// TestProcessAppliesEXIFOrientation: AVT-055 end to end. The EXIF display
// transform runs BEFORE the crop, so a rotated phone photo is upright when
// the square is taken: orientation 6 puts the source's left (red) half on
// top, orientation 8 flips that, and an unrotated source keeps its
// left/right split.
func TestProcessAppliesEXIFOrientation(t *testing.T) {
	p, err := New(filepath.Join(t.TempDir(), "avatars"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	staging := t.TempDir()

	cases := []struct {
		name        string
		orientation uint16
		top, bottom byte // dominant channel above/below the split
	}{
		{"unrotated keeps left-right split", 1, 0, 0}, // handled separately
		{"orientation 6 rotates 90 CW", 6, 'R', 'B'},
		{"orientation 8 rotates 90 CCW", 8, 'B', 'R'},
	}
	for _, tc := range cases {
		if tc.orientation == 1 {
			src := jpegWithOrientation(t, splitImage(400, 200), 1)
			img := decodeStoredWebP(t, mustProcess(t, p, staging, "o1.jpg", src))
			assertHue(t, img, 120, 256, 'R', "orientation 1 left half")
			assertHue(t, img, 390, 256, 'B', "orientation 1 right half")
			continue
		}
		src := jpegWithOrientation(t, splitImage(400, 200), tc.orientation)
		img := decodeStoredWebP(t, mustProcess(t, p, staging, "o.jpg", src))
		assertHue(t, img, 256, 120, tc.top, tc.name+" top")
		assertHue(t, img, 256, 390, tc.bottom, tc.name+" bottom")
	}
}

// TestProcessWebpSource: a WebP source round-trips through the same
// pipeline, including an EXIF RIFF chunk (AVT-055's second reader).
func TestProcessWebpSource(t *testing.T) {
	p, err := New(filepath.Join(t.TempDir(), "avatars"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	staging := t.TempDir()

	split := splitImage(400, 200)
	plain := mustProcess(t, p, staging, "plain.webp", mustWebp(t, split, 90))
	if img := decodeStoredWebP(t, plain); img.Bounds() != (image.Rect(0, 0, OutputSize, OutputSize)) {
		t.Fatalf("plain webp produced bounds %v", img.Bounds())
	}

	rotated := mustProcess(t, p, staging, "rotated.webp", webpWithOrientation(t, split, 6))
	img := decodeStoredWebP(t, rotated)
	assertHue(t, img, 256, 120, 'R', "webp EXIF orientation 6 top")
	assertHue(t, img, 256, 390, 'B', "webp EXIF orientation 6 bottom")
}

// mustWebp encodes an image with the package's libwebp encoder.
func mustWebp(t *testing.T, img *image.NRGBA, quality float32) []byte {
	t.Helper()
	data, err := encodeWebP(img, quality)
	if err != nil {
		t.Fatalf("encodeWebP: %v", err)
	}
	return data
}

// TestProcessTransparentPNGIsWhite: AVT-056's alpha rule end to end — the
// white canvas wins and the stored WebP is fully opaque.
func TestProcessTransparentPNGIsWhite(t *testing.T) {
	src := solidImage(64, 64, color.NRGBA{R: 200, G: 50, B: 50, A: 0})
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	p, err := New(filepath.Join(t.TempDir(), "avatars"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	img := decodeStoredWebP(t, mustProcess(t, p, t.TempDir(), "clear.png", buf.Bytes()))
	for _, pt := range [][2]int{{0, 0}, {256, 256}, {511, 511}} {
		b := img.Bounds()
		c := color.NRGBAModel.Convert(img.At(b.Min.X+pt[0], b.Min.Y+pt[1])).(color.NRGBA)
		if c != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
			t.Errorf("pixel %v = %v, want opaque white", pt, c)
		}
	}
}

// TestProcessSentinels: the public path returns exactly the sentinel each
// failure maps to (AVT-094's flash keys), and writes nothing.
func TestProcessSentinels(t *testing.T) {
	p, err := New(filepath.Join(t.TempDir(), "avatars"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	staging := t.TempDir()

	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"text", []byte("just some text"), ErrType},
		{"avif", ftyp("avif", "avif", "mif1", "miaf"), ErrType},
		{"empty file", nil, ErrType},
		{"51 megapixel header", jpegHeader(9000, 6000), ErrBig},
		{"strip over max side", jpegHeader(16001, 100), ErrBig},
		{"webp over 64 MiB", riffWebP("VP8 "), ErrBig}, // truncated below to MaxSourceBytes+1
		{"corrupt png pixels", corruptPNG(t), ErrUnreadable},
		{"heic that libheif cannot read", ftyp("heic", "mif1"), ErrUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.data
			if tc.name == "webp over 64 MiB" {
				huge := filepath.Join(staging, "huge.webp")
				if err := os.WriteFile(huge, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Truncate(huge, MaxSourceBytes+1); err != nil {
					t.Fatal(err)
				}
				data, err = os.ReadFile(huge)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, got := runConversion(t, p, staging, "case", data)
			if !errors.Is(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// corruptPNG returns a PNG whose IHDR is intact (so the probe passes) but
// whose IDAT payload is destroyed (so the pixel decode fails).
func corruptPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solidImage(16, 16, bandGreen)); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	i := bytes.Index(b, []byte("IDAT"))
	if i < 0 {
		t.Fatal("encoded PNG has no IDAT chunk")
	}
	for k := i + 8; k < i+24 && k < len(b); k++ {
		b[k] ^= 0xFF
	}
	return b
}

// TestProcessHEICFixture: AVT-054 end to end — a real HEIC file is
// probed, converted through the temporary JPEG at quality 95, and stored
// as a 512×512 WebP. The fixture is optional; without one the test skips
// (nothing in the repo can synthesise a HEIC without libheif's encoder).
func TestProcessHEICFixture(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "*.heic"))
	if err != nil || len(fixtures) == 0 {
		t.Skip("no HEIC fixture in internal/avatar/testdata — add one to enable AVT-054 coverage")
	}
	p, err := New(filepath.Join(t.TempDir(), "avatars"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	for _, fix := range fixtures {
		data, rerr := os.ReadFile(fix)
		if rerr != nil {
			t.Fatal(rerr)
		}
		img := decodeStoredWebP(t, mustProcess(t, p, t.TempDir(), filepath.Base(fix), data))
		if img.Bounds().Dx() != OutputSize {
			t.Errorf("%s produced bounds %v", fix, img.Bounds())
		}
	}
}

// TestProcessLargeSourceWithinCeilings: a noise JPEG of tens of megabytes
// (the biggest thing the ceilings allow) is accepted — the caps are about
// pixels, not about refusing ambitious phone photos (AVT-051).
func TestProcessLargeSourceWithinCeilings(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic test noise
	var data []byte
	px := 0
	for _, dim := range [][2]int{{4500, 3000}, {6000, 4000}} {
		w, h := dim[0], dim[1]
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i] = byte(rng.Intn(256))
			img.Pix[i+1] = byte(rng.Intn(256))
			img.Pix[i+2] = byte(rng.Intn(256))
			img.Pix[i+3] = 255
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
			t.Fatal(err)
		}
		data, px = buf.Bytes(), w*h
		if len(data) >= 12_000_000 {
			break
		}
	}
	if len(data) < 12_000_000 {
		t.Logf("largest noise JPEG built: %d bytes (below the 12 MB goal)", len(data))
	} else {
		t.Logf("source: %d bytes, %d px (under the %d px ceiling)", len(data), px, MaxPixels)
	}

	p, err := New(filepath.Join(t.TempDir(), "avatars"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	if len(data) == 0 {
		t.Skip("no noise source built")
	}
	decodeStoredWebP(t, mustProcess(t, p, t.TempDir(), "noise.jpg", data))
}
