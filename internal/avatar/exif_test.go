package avatar

import (
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// buildTIFF returns a TIFF header whose IFD0 holds a single orientation
// entry, in either byte order, with a SHORT (3) or LONG (4) value of
// count 1 — the shapes AVT-055's reader must understand.
func buildTIFF(little bool, orientation uint16, typ uint16) []byte {
	b := make([]byte, 26)
	if little {
		b[0], b[1] = 'I', 'I'
		binary.LittleEndian.PutUint16(b[2:], 42)
		binary.LittleEndian.PutUint32(b[4:], 8)
		binary.LittleEndian.PutUint16(b[8:], 1) // one entry
		binary.LittleEndian.PutUint16(b[10:], 0x0112)
		binary.LittleEndian.PutUint16(b[12:], typ)
		binary.LittleEndian.PutUint32(b[14:], 1)
		if typ == 4 {
			binary.LittleEndian.PutUint32(b[18:], uint32(orientation))
		} else {
			binary.LittleEndian.PutUint16(b[18:], orientation)
		}
		// bytes 22..26 stay zero: no next IFD
		return b
	}
	b[0], b[1] = 'M', 'M'
	binary.BigEndian.PutUint16(b[2:], 42)
	binary.BigEndian.PutUint32(b[4:], 8)
	binary.BigEndian.PutUint16(b[8:], 1)
	binary.BigEndian.PutUint16(b[10:], 0x0112)
	binary.BigEndian.PutUint16(b[12:], typ)
	binary.BigEndian.PutUint32(b[14:], 1)
	if typ == 4 {
		binary.BigEndian.PutUint32(b[18:], uint32(orientation))
	} else {
		binary.BigEndian.PutUint16(b[18:], orientation)
	}
	return b
}

// TestTiffOrientation pins the bounds-checked TIFF reader (AVT-055): both
// byte orders, SHORT and LONG, the optional WebP "Exif" prefix, and every
// malformed shape returning 0 — ignored, never a failure.
func TestTiffOrientation(t *testing.T) {
	valid := []struct {
		name string
		data []byte
		want int
	}{
		{"le short 1", buildTIFF(true, 1, 3), 1},
		{"le short 3", buildTIFF(true, 3, 3), 3},
		{"le short 6", buildTIFF(true, 6, 3), 6},
		{"le short 8", buildTIFF(true, 8, 3), 8},
		{"be short 6", buildTIFF(false, 6, 3), 6},
		{"le long 8", buildTIFF(true, 8, 4), 8},
		{"be long 6", buildTIFF(false, 6, 4), 6},
		{"webp exif prefix", append([]byte("Exif\x00\x00"), buildTIFF(true, 5, 3)...), 5},
		{"out of range 0", buildTIFF(true, 0, 3), 0},
		{"out of range 9", buildTIFF(true, 9, 3), 0},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			if got := tiffOrientation(tc.data); got != tc.want {
				t.Errorf("tiffOrientation() = %d, want %d", got, tc.want)
			}
		})
	}

	malformed := []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"too short", []byte("II*\x00")},
		{"bad byte order", []byte("XX\x2a\x00\x08\x00\x00\x00")},
		{"bad magic", []byte{'I', 'I', 43, 0, 8, 0, 0, 0}},
		{"ifd offset past end", []byte{'I', 'I', 42, 0, 0xF0, 0x00, 0x00, 0x00}},
		{"truncated entry table", buildTIFF(true, 6, 3)[:20]},
		{"giant entry count", append(buildTIFF(true, 6, 3)[:8], 0xFF, 0xFF)},
		{"wrong value type", withValueType(buildTIFF(true, 6, 3), 2)},
		{"count not one", withEntryCount(buildTIFF(true, 6, 3), 2)},
		{"no orientation tag", withTag(buildTIFF(true, 6, 3), 0x0100)},
	}
	for _, tc := range malformed {
		t.Run(tc.name, func(t *testing.T) {
			if got := tiffOrientation(tc.data); got != 0 {
				t.Errorf("tiffOrientation() = %d, want 0 (ignored)", got)
			}
		})
	}
}

// withValueType/withEntryCount/withTag mutate the single IFD0 entry of a
// well-formed little-endian blob to build malformed variants.
func withValueType(b []byte, typ uint16) []byte {
	binary.LittleEndian.PutUint16(b[12:], typ)
	return b
}

func withEntryCount(b []byte, count uint32) []byte {
	binary.LittleEndian.PutUint32(b[14:], count)
	return b
}

func withTag(b []byte, tag uint16) []byte {
	binary.LittleEndian.PutUint16(b[10:], tag)
	return b
}

// jpegSegment builds one marker segment: [FF marker][len][payload].
func jpegSegment(marker byte, payload []byte) []byte {
	b := make([]byte, 4+len(payload))
	b[0], b[1] = 0xFF, marker
	binary.BigEndian.PutUint16(b[2:], uint16(len(payload)+2))
	copy(b[4:], payload)
	return b
}

func exifApp1(t *testing.T, orientation uint16) []byte {
	t.Helper()
	return jpegSegment(0xE1, append([]byte("Exif\x00\x00"), buildTIFF(true, orientation, 3)...))
}

// TestJpegExifOrientation walks real segment layouts on disk (AVT-055):
// EXIF after other APP segments, XMP APP1 before the EXIF APP1, no EXIF at
// all, and malformed segment streams — the orientation is found when it
// exists, and every other shape is ignored as 0.
func TestJpegExifOrientation(t *testing.T) {
	soi := []byte{0xFF, 0xD8}
	eoi := []byte{0xFF, 0xD9}
	app0 := jpegSegment(0xE0, []byte("JFIF\x00"))
	xmp := jpegSegment(0xE1, []byte("http://ns.adobe.com/xap/1.0/\x00<xmp/>"))

	cases := []struct {
		name string
		body []byte
		want int
	}{
		{"exif first", append(soi, append(exifApp1(t, 6), eoi...)...), 6},
		{"app0 then exif", append(soi, append(append(app0, exifApp1(t, 8)...), eoi...)...), 8},
		{"xmp before exif", append(soi, append(append(xmp, exifApp1(t, 3)...), eoi...)...), 3},
		{"no exif at all", append(soi, append(app0, eoi...)...), 0},
		{"exif stops at sos", append(soi, append([]byte{0xFF, 0xDA, 0x00, 0x02}, exifApp1(t, 6)...)...), 0},
		{"malformed length", append(soi, append([]byte{0xFF, 0xE1, 0x00, 0x01}, eoi...)...), 0},
		{"stray non-marker bytes", append(soi, append([]byte{0x12, 0x34}, exifApp1(t, 6)...)...), 0},
		{"truncated after soi", soi, 0},
		{"not a jpeg at all", []byte("hello world"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "a.jpg")
			if err := os.WriteFile(p, tc.body, 0o600); err != nil {
				t.Fatal(err)
			}
			if got := jpegExifOrientation(p); got != tc.want {
				t.Errorf("jpegExifOrientation() = %d, want %d", got, tc.want)
			}
		})
	}
}

// riffChunk builds one padded RIFF chunk: [id][size][payload][pad byte].
func riffChunk(id string, payload []byte) []byte {
	b := make([]byte, 8+len(payload)+len(payload)%2)
	copy(b[0:4], id)
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(payload)))
	copy(b[8:], payload)
	return b
}

// webpHeader builds [RIFF][size][WEBP].
func webpHeader(payloadLen int) []byte {
	b := make([]byte, 12)
	copy(b[0:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], uint32(4+payloadLen))
	copy(b[8:12], "WEBP")
	return b
}

// TestWebpExifOrientation pins the WebP EXIF RIFF-chunk reader (AVT-055):
// chunk walking with even-size padding, both TIFF byte orders, the optional
// Exif prefix inside the chunk, and malformed or oversized chunks ignored.
func TestWebpExifOrientation(t *testing.T) {
	build := func(chunks ...[]byte) string {
		var body []byte
		for _, c := range chunks {
			body = append(body, c...)
		}
		data := append(webpHeader(len(body)), body...)
		p := filepath.Join(t.TempDir(), "a.webp")
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if got := webpExifOrientation(build(riffChunk("VP8 ", []byte{0x00}), riffChunk("EXIF", buildTIFF(false, 6, 3)))); got != 6 {
		t.Errorf("with VP8 first: got %d, want 6", got)
	}
	// An odd-sized chunk before EXIF exercises the even-size padding skip.
	if got := webpExifOrientation(build(riffChunk("JUNK", []byte{1, 2, 3}), riffChunk("EXIF", buildTIFF(true, 8, 3)))); got != 8 {
		t.Errorf("with odd JUNK before EXIF: got %d, want 8", got)
	}
	prefixed := append([]byte("Exif\x00\x00"), buildTIFF(true, 2, 3)...)
	if got := webpExifOrientation(build(riffChunk("EXIF", prefixed))); got != 2 {
		t.Errorf("with Exif prefix in chunk: got %d, want 2", got)
	}
	if got := webpExifOrientation(build(riffChunk("VP8L", []byte{0x00}), riffChunk("EXIF", buildTIFF(true, 6, 3)))); got != 6 {
		t.Errorf("no leading chunk: got %d, want 6", got)
	}
	if got := webpExifOrientation(build(riffChunk("VP8 ", []byte{0x00}))); got != 0 {
		t.Errorf("without EXIF chunk: got %d, want 0", got)
	}
	if got := webpExifOrientation(build(riffChunk("EXIF", []byte("garbage")))); got != 0 {
		t.Errorf("garbage EXIF chunk: got %d, want 0", got)
	}
	// A chunk claiming more than maxExifChunk must not be allocated.
	huge := make([]byte, 12)
	copy(huge[0:4], "RIFF")
	copy(huge[8:12], "WEBP")
	huge = append(huge, 'E', 'X', 'I', 'F')
	huge = append(huge, 0xFF, 0xFF, 0xFF, 0xFF)
	p := filepath.Join(t.TempDir(), "huge.webp")
	if err := os.WriteFile(p, huge, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := webpExifOrientation(p); got != 0 {
		t.Errorf("oversized EXIF chunk: got %d, want 0", got)
	}
	if got := webpExifOrientation(filepath.Join(t.TempDir(), "missing.webp")); got != 0 {
		t.Errorf("missing file: got %d, want 0", got)
	}
}

// TestFileExifOrientationDispatch pins which formats consult EXIF (AVT-055):
// JPEG and WebP read it, PNG ignores eXIf in v1, and HEIC needs no reader
// here (libheif applies irot/imir itself; the intermediate JPEG has no
// EXIF), so nothing is ever rotated twice.
func TestFileExifOrientationDispatch(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(p, []byte("II*\x00anything"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := fileExifOrientation(formatPNG, p); got != 0 {
		t.Errorf("png: got %d, want 0", got)
	}
	if got := fileExifOrientation(formatHEIC, p); got != 0 {
		t.Errorf("heic: got %d, want 0", got)
	}
	if got := fileExifOrientation(formatUnknown, p); got != 0 {
		t.Errorf("unknown: got %d, want 0", got)
	}
	if got := fileExifOrientation(formatJPEG, filepath.Join(t.TempDir(), "gone.jpg")); got != 0 {
		t.Errorf("missing jpeg: got %d, want 0", got)
	}
}

// src3x2 builds the fixed 3×2 test photo: values 10..60 in red, laid out
//
//	A B C
//	D E F
//
// with a distinctive alpha on F so copies are proven to carry all four
// channels. applyOrientation mutates NRGBA inputs in place, so every case
// builds its own source.
func src3x2() *image.NRGBA {
	vals := [6]uint8{10, 20, 30, 40, 50, 60}
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for i, v := range vals {
		o := img.PixOffset(i%3, i/3)
		img.Pix[o] = v
		img.Pix[o+3] = 255
	}
	f := img.PixOffset(2, 1)
	img.Pix[f+3] = 100 // F carries alpha 100
	return img
}

// assertPhoto checks dimensions and the red value of every pixel against a
// row-major table. It reads the non-premultiplied channels: one test pixel
// carries alpha 100, and RGBA() would report its red premultiplied
// (60×100/255 = 23) instead of the stored 60.
func assertPhoto(t *testing.T, img image.Image, wantW, wantH int, want []uint8) {
	t.Helper()
	b := img.Bounds()
	if b.Dx() != wantW || b.Dy() != wantH {
		t.Fatalf("bounds = %v, want %dx%d", b, wantW, wantH)
	}
	for y := 0; y < wantH; y++ {
		for x := 0; x < wantW; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			if c.R != want[y*wantW+x] {
				t.Errorf("pixel(%d,%d) red = %d, want %d", x, y, c.R, want[y*wantW+x])
			}
		}
	}
}

// TestApplyOrientation pins all eight EXIF display transforms (AVT-055)
// against hand-derived expectations: 1-4 keep the 3×2 dimensions, 5-8
// transpose to 2×3, and absent/unknown/identity orientations return the
// image untouched. The 3×2 shape makes every mapping position unique.
func TestApplyOrientation(t *testing.T) {
	if got := applyOrientation(src3x2(), 0); got == nil {
		t.Fatal("orientation 0 returned nil, want the image unchanged")
	}
	if img := src3x2(); applyOrientation(img, 1) != img {
		t.Error("orientation 1 did not return the original image")
	}
	if img := src3x2(); applyOrientation(img, 9) != img {
		t.Error("orientation 9 (out of range) did not return the original image")
	}
	if img := src3x2(); applyOrientation(img, -1) != img {
		t.Error("orientation -1 (malformed) did not return the original image")
	}

	cases := []struct {
		orientation int
		wantW       int
		wantH       int
		want        []uint8
	}{
		{2, 3, 2, []uint8{30, 20, 10, 60, 50, 40}}, // flip horizontal
		{3, 3, 2, []uint8{60, 50, 40, 30, 20, 10}}, // rotate 180°
		{4, 3, 2, []uint8{40, 50, 60, 10, 20, 30}}, // flip vertical
		{5, 2, 3, []uint8{10, 40, 20, 50, 30, 60}}, // transpose
		{6, 2, 3, []uint8{40, 10, 50, 20, 60, 30}}, // rotate 90° CW
		{7, 2, 3, []uint8{60, 30, 50, 20, 40, 10}}, // transverse
		{8, 2, 3, []uint8{30, 60, 20, 50, 10, 40}}, // rotate 90° CCW
	}
	for _, tc := range cases {
		assertPhoto(t, applyOrientation(src3x2(), tc.orientation), tc.wantW, tc.wantH, tc.want)
	}
}

// TestApplyOrientationKeepsAlpha: the transpose copy moves all four bytes,
// so a translucent source pixel stays translucent after the transform
// (alpha compositing happens later, in transformToAvatar).
func TestApplyOrientationKeepsAlpha(t *testing.T) {
	img := applyOrientation(src3x2(), 5) // F (alpha 100) moves to row 2, col 1
	b := img.Bounds()
	_, _, _, a := img.At(b.Min.X+1, b.Min.Y+2).RGBA()
	if got := uint8(a >> 8); got != 100 {
		t.Errorf("transposed F alpha = %d, want 100", got)
	}
}

// TestApplyOrientationFromRGBA: a non-NRGBA source (a decoded RGBA PNG,
// say) is converted once and transformed identically.
func TestApplyOrientationFromRGBA(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i, v := range []uint8{10, 20, 30, 40, 50, 60} {
		o := src.PixOffset(i%3, i/3)
		src.Pix[o] = v
		src.Pix[o+3] = 255
	}
	got := applyOrientation(src, 6)
	if _, ok := got.(*image.NRGBA); !ok {
		t.Fatalf("result type = %T, want *image.NRGBA", got)
	}
	assertPhoto(t, got, 2, 3, []uint8{40, 10, 50, 20, 60, 30})
}
