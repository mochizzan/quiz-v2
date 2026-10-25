package avatar

// Conversion tests for AVT-050..AVT-058. Everything here is pure Go: the
// sniff, the pixel ceiling, the source-size guard, the encode size policy,
// and the atomic write are exercised without touching a cgo codec, so the
// suite runs under CGO_ENABLED=0 (the cgo end-to-end paths live in
// processor_test.go).

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// jpegHeader crafts a JPEG whose SOF0 declares the given dimensions and
// whose SOS carries an empty payload. image.DecodeConfig stops at SOS
// (configOnly), so the header-only probe sees w×h — exactly what the pixel
// ceiling test needs before any pixel work — while image.Decode fails in
// processSOS, which is the ErrUnreadable case.
func jpegHeader(w, h int) []byte {
	return []byte{
		0xFF, 0xD8, // SOI
		0xFF, 0xC0, 0x00, 0x11, // SOF0; length 17 = 2 + 6 + 3 components × 3
		0x08,                  // 8-bit precision
		byte(h >> 8), byte(h), // height
		byte(w >> 8), byte(w), // width
		0x03,             // component count
		0x01, 0x11, 0x00, // Y: HV 1×1, quant table 0
		0x02, 0x11, 0x00, // Cb
		0x03, 0x11, 0x00, // Cr
		0xFF, 0xDA, 0x00, 0x02, // SOS, zero-length payload
	}
}

// writeSrc drops data into dir under name and returns the full path.
func writeSrc(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// convertOnce runs one conversion into out.webp inside dir.
func convertOnce(t *testing.T, dir, src string) error {
	t.Helper()
	return convertFile(context.Background(), dir, src, filepath.Join(dir, "out.webp"))
}

// assertNoAvatarTemps: writeAvatar's staging files must never outlive the
// write (§4.2).
func assertNoAvatarTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".avatar-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover staging file %q", e.Name())
		}
	}
}

// TestConvertFileRejectsWrongType pins AVT-050: only JPEG, PNG, WebP, and
// HEIC-with-an-accepted-brand reach the pipeline; SVG, GIF, AVIF, plain
// text, and an empty file are refused with ErrType, and nothing is written.
func TestConvertFileRejectsWrongType(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		data []byte
	}{
		{"text.txt", []byte("hello, world")},
		{"empty.bin", nil},
		{"anim.gif", []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00")},
		{"vector.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"/>`)},
		{"avif.avif", ftyp("avif", "avif", "mif1", "miaf")},
		{"hevc-sequence.hevc", ftyp("hevx", "mif1")},
	}
	dst := filepath.Join(dir, "out.webp")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := writeSrc(t, dir, tc.name, tc.data)
			err := convertFile(context.Background(), dir, src, dst)
			if !errors.Is(err, ErrType) {
				t.Fatalf("got %v, want ErrType", err)
			}
			if errors.Is(err, ErrBig) || errors.Is(err, ErrUnreadable) || errors.Is(err, ErrFailed) {
				t.Errorf("wrong type must not be classified as %v/%v/%v", ErrBig, ErrUnreadable, ErrFailed)
			}
			if _, err := os.Stat(dst); !os.IsNotExist(err) {
				t.Errorf("a refused source still produced %s", dst)
			}
		})
	}
}

// TestConvertFilePixelCeiling pins AVT-051 against crafted JPEG headers:
// dimensions are read before any pixel decode, over-50 MP or over-16000 px
// on either axis is ErrBig, exactly 50 MP is not, and a header that passes
// the ceiling but cannot be decoded is ErrUnreadable — never ErrType.
func TestConvertFilePixelCeiling(t *testing.T) {
	refused := []struct {
		name string
		w, h int
	}{
		{"area over the ceiling", 9000, 6000},       // 54,000,000 px
		{"one pixel over the ceiling", 10000, 5001}, // 50,001,000 px
		{"wide strip over max side", 16001, 100},
		{"tall strip over max side", 100, 16001},
		{"max 16-bit dimensions", 65535, 65535}, // must not overflow the product
	}
	acceptedByProbe := []struct {
		name string
		w, h int
	}{
		{"exactly 50,000,000 px", 10000, 5000},
		{"small header with no scan data", 8, 8},
	}

	dir := t.TempDir()
	for _, tc := range refused {
		t.Run("refused: "+tc.name, func(t *testing.T) {
			src := writeSrc(t, dir, "huge.jpg", jpegHeader(tc.w, tc.h))
			err := convertOnce(t, dir, src)
			if !errors.Is(err, ErrBig) {
				t.Fatalf("%dx%d: got %v, want ErrBig", tc.w, tc.h, err)
			}
		})
	}
	for _, tc := range acceptedByProbe {
		t.Run("probed: "+tc.name, func(t *testing.T) {
			src := writeSrc(t, dir, "probe.jpg", jpegHeader(tc.w, tc.h))
			err := convertOnce(t, dir, src)
			if !errors.Is(err, ErrUnreadable) {
				t.Fatalf("%dx%d: got %v, want ErrUnreadable", tc.w, tc.h, err)
			}
			if errors.Is(err, ErrBig) {
				t.Errorf("%dx%d must sit under the pixel ceiling", tc.w, tc.h)
			}
		})
	}
}

// TestConvertFileSourceBytesGuard pins AVT-053/054: WebP and HEIC are
// refused at over 64 MiB before either cgo library reads them, while a file
// of exactly 64 MiB passes the guard (and then fails on its own merits).
func TestConvertFileSourceBytesGuard(t *testing.T) {
	dir := t.TempDir()
	shapes := []struct {
		name string
		head []byte
	}{
		{"photo.webp", riffWebP("VP8 ")},
		{"photo.heic", ftyp("heic", "mif1")},
	}
	for _, tc := range shapes {
		t.Run(tc.name+" over 64 MiB", func(t *testing.T) {
			src := filepath.Join(dir, "over-"+tc.name)
			if err := os.WriteFile(src, tc.head, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(src, MaxSourceBytes+1); err != nil { // sparse, cheap
				t.Fatal(err)
			}
			err := convertOnce(t, dir, src)
			if !errors.Is(err, ErrBig) {
				t.Fatalf("got %v, want ErrBig before any library reads the file", err)
			}
		})
		t.Run(tc.name+" exactly 64 MiB", func(t *testing.T) {
			src := filepath.Join(dir, "at-"+tc.name)
			if err := os.WriteFile(src, tc.head, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(src, MaxSourceBytes); err != nil {
				t.Fatal(err)
			}
			err := convertOnce(t, dir, src)
			if errors.Is(err, ErrBig) {
				t.Fatalf("exactly 64 MiB must pass the size guard, got %v", err)
			}
			// It proceeds to the probe, which cannot parse this stub:
			// ErrUnreadable with the cgo codecs, the !cgo stub's ErrFailed.
			if !errors.Is(err, ErrUnreadable) && !errors.Is(err, ErrFailed) {
				t.Fatalf("got %v, want ErrUnreadable or ErrFailed", err)
			}
		})
	}
}

// TestConvertFileOperationalFailures: a source that cannot be opened is an
// operational failure (ErrFailed), and a cancelled context stops the
// pipeline before anything is written.
func TestConvertFileOperationalFailures(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.webp")

	missing := filepath.Join(dir, "nope.jpg")
	err := convertFile(context.Background(), dir, missing, dst)
	if !errors.Is(err, ErrFailed) {
		t.Errorf("missing source: got %v, want ErrFailed", err)
	}

	src := writeSrc(t, dir, "ok.jpg", jpegHeader(8, 8))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := convertFile(ctx, dir, src, dst); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled context: got %v, want context.Canceled", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("cancelled conversion still wrote %s", dst)
	}
}

// TestConvertFileWithoutCGOFailsAtEncode documents AVT-105: with the cgo
// codecs absent the sniff, probe, decode, orientation, and crop all work
// (they are pure Go), and only the WebP encode fails — with ErrFailed and
// the required CGO_ENABLED=1 message, and nothing written.
func TestConvertFileWithoutCGOFailsAtEncode(t *testing.T) {
	if checkCodec() == nil {
		t.Skip("cgo codecs are available; this path exists only under CGO_ENABLED=0")
	}
	dir := t.TempDir()
	srcImg := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			srcImg.SetNRGBA(x, y, color.NRGBA{G: 200, A: 255})
		}
	}

	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, srcImg, nil); err != nil {
		t.Fatal(err)
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, srcImg); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"jpeg", jpg.Bytes()},
		{"png", pngBuf.Bytes()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := writeSrc(t, dir, "solid."+tc.name, tc.data)
			err := convertOnce(t, dir, src)
			if !errors.Is(err, ErrFailed) {
				t.Fatalf("got %v, want ErrFailed", err)
			}
			if !strings.Contains(err.Error(), "CGO_ENABLED=1") {
				t.Errorf("error %q must name the missing CGO_ENABLED=1 support", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "out.webp")); !os.IsNotExist(err) {
				t.Error("a failed conversion wrote the destination")
			}
			assertNoAvatarTemps(t, dir)
		})
	}
}

// TestProbeAndDecodeClassifyPngErrors: the probe reads only the header, so
// a PNG with a good IHDR but destroyed image data passes the ceiling and
// then fails in the pixel decode as ErrUnreadable; a header that cannot
// even be read is ErrUnreadable too (ErrType would mean wrong bytes).
func TestProbeAndDecodeClassifyPngErrors(t *testing.T) {
	dir := t.TempDir()

	// A real PNG with its IDAT payload corrupted: DecodeConfig stops at the
	// IDAT chunk header and succeeds, image.Decode fails inside the stream.
	var buf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	corrupt := buf.Bytes()
	if i := bytes.Index(corrupt, []byte("IDAT")); i < 0 {
		t.Fatal("encoded PNG has no IDAT chunk")
	} else {
		for k := i + 8; k < i+8+16 && k < len(corrupt); k++ {
			corrupt[k] ^= 0xFF
		}
	}
	src := writeSrc(t, dir, "corrupt.png", corrupt)
	err := convertOnce(t, dir, src)
	if !errors.Is(err, ErrUnreadable) {
		t.Errorf("corrupt IDAT: got %v, want ErrUnreadable", err)
	}
	if errors.Is(err, ErrType) {
		t.Error("corrupt IDAT must not be reported as a wrong type")
	}

	// A PNG truncated inside IHDR cannot even be probed.
	truncated := writeSrc(t, dir, "truncated.png", buf.Bytes()[:12])
	if err := convertOnce(t, dir, truncated); !errors.Is(err, ErrUnreadable) {
		t.Errorf("truncated header: got %v, want ErrUnreadable", err)
	}
}

// TestEncodeAvatarSizePolicy pins AVT-057/058 with an injected encoder:
// quality 80 first, one retry at quality 60 when the output would exceed
// 5,000,000 bytes, ErrFailed (nothing written) if it is still too big or
// either encode fails.
func TestEncodeAvatarSizePolicy(t *testing.T) {
	canvas := image.NewNRGBA(image.Rect(0, 0, OutputSize, OutputSize))
	small := []byte("webp-bytes")
	oversize := make([]byte, MaxAvatarBytes+1)
	boom := errors.New("encoder failed")

	type attempt struct {
		data []byte
		err  error
	}
	cases := []struct {
		name      string
		attempts  []attempt
		wantData  []byte
		wantErr   error
		wantText  string
		wantQuals []float32
	}{
		{
			name:      "small at quality 80",
			attempts:  []attempt{{data: small}},
			wantData:  small,
			wantQuals: []float32{80},
		},
		{
			name:      "retry at quality 60 then fits",
			attempts:  []attempt{{data: oversize}, {data: small}},
			wantData:  small,
			wantQuals: []float32{80, 60},
		},
		{
			name:      "still oversize at quality 60 fails",
			attempts:  []attempt{{data: oversize}, {data: oversize}},
			wantErr:   ErrFailed,
			wantText:  "quality 60",
			wantQuals: []float32{80, 60},
		},
		{
			name:      "encoder error at quality 80",
			attempts:  []attempt{{err: boom}},
			wantErr:   ErrFailed,
			wantText:  boom.Error(),
			wantQuals: []float32{80},
		},
		{
			name:      "encoder error at quality 60",
			attempts:  []attempt{{data: oversize}, {err: boom}},
			wantErr:   ErrFailed,
			wantText:  boom.Error(),
			wantQuals: []float32{80, 60},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []float32
			i := 0
			encode := func(_ *image.NRGBA, q float32) ([]byte, error) {
				got = append(got, q)
				a := tc.attempts[i]
				i++
				return a.data, a.err
			}
			data, err := encodeAvatar(canvas, encode)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("got %v, want success", err)
				}
				if !bytes.Equal(data, tc.wantData) {
					t.Errorf("data = %d bytes, want %d bytes", len(data), len(tc.wantData))
				}
			} else {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got %v, want %v", err, tc.wantErr)
				}
				if data != nil {
					t.Errorf("a failed encode returned %d bytes", len(data))
				}
				if tc.wantText != "" && !strings.Contains(err.Error(), tc.wantText) {
					t.Errorf("error %q must contain %q", err, tc.wantText)
				}
			}
			if len(got) != len(tc.wantQuals) {
				t.Fatalf("qualities = %v, want %v", got, tc.wantQuals)
			}
			for k, q := range tc.wantQuals {
				if got[k] != q {
					t.Errorf("quality[%d] = %v, want %v", k, got[k], q)
				}
			}
			if i != len(tc.attempts) {
				t.Errorf("encoder called %d times, want %d", i, len(tc.attempts))
			}
		})
	}
}

// TestWriteAvatar pins the §4.2 write: the directory is created lazily, a
// successful write leaves the exact bytes under dstPath and no staging
// file, and a failed rename (dstPath already a directory) leaves neither a
// temp file nor a partial destination.
func TestWriteAvatar(t *testing.T) {
	payload := []byte("RIFF....WEBPavatar")

	t.Run("success creates the directory and the file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nested", "avatars")
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("test setup: directory should not exist yet")
		}
		dst := filepath.Join(dir, "abc123.webp")
		if err := writeAvatar(dir, dst, payload); err != nil {
			t.Fatalf("writeAvatar: %v", err)
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, payload) {
			t.Errorf("stored %d bytes, want %d", len(got), len(payload))
		}
		if runtime.GOOS != "windows" { // Windows only carries the read-only bit
			if fi, err := os.Stat(dst); err != nil {
				t.Fatal(err)
			} else if fi.Mode().Perm() != 0o640 {
				t.Errorf("mode = %o, want 640 (§4.2)", fi.Mode().Perm())
			}
		}
		assertNoAvatarTemps(t, dir)
	})

	t.Run("rename failure writes nothing and leaves no temp", func(t *testing.T) {
		dir := t.TempDir()
		dst := filepath.Join(dir, "taken")
		if err := os.Mkdir(dst, 0o750); err != nil {
			t.Fatal(err)
		}
		err := writeAvatar(dir, dst, payload)
		if !errors.Is(err, ErrFailed) {
			t.Fatalf("got %v, want ErrFailed", err)
		}
		assertNoAvatarTemps(t, dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "taken" || !entries[0].IsDir() {
			t.Errorf("directory contents = %v, want only the pre-existing dir", entries)
		}
	})
}
