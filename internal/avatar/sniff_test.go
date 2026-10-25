package avatar

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ftyp builds a minimal ISO-BMFF ftyp box: [size][ftyp][major][minor 0][compat…].
func ftyp(major string, compat ...string) []byte {
	b := make([]byte, 16+4*len(compat))
	binary.BigEndian.PutUint32(b[0:4], uint32(len(b)))
	copy(b[4:8], "ftyp")
	copy(b[8:12], major)
	for i, c := range compat {
		copy(b[16+4*i:], c)
	}
	return b
}

// riffWebP builds a RIFF/WEBP header: [RIFF][size][WEBP][chunk id] — enough
// for the sniff, no valid payload needed.
func riffWebP(chunk string) []byte {
	b := make([]byte, 16)
	copy(b[0:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(b)-8))
	copy(b[8:12], "WEBP")
	copy(b[12:16], chunk)
	return b
}

// TestSniffFormat pins AVT-050: exactly JPEG, PNG, WebP, and HEIC with an
// accepted brand pass the 512-byte sniff, and every other shape — AVIF
// (even one also listing mif1), HEVC sequences, SVG, GIF, text, RIFF
// payloads that are not WebP, truncated headers, and empty input — is the
// distinct formatUnknown outcome the pipeline refuses as a wrong type.
func TestSniffFormat(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want format
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, formatJPEG},
		{"jpeg soi+sof", []byte{0xFF, 0xD8, 0xFF, 0xC0}, formatJPEG},
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}, formatPNG},
		{"webp vp8l", riffWebP("VP8L"), formatWebP},
		{"webp vp8x", riffWebP("VP8X"), formatWebP},
		// The RIFF/WEBP signature alone decides; no VP8 chunk required.
		{"webp bare signature", []byte("RIFF\x24\x00\x00\x00WEBP"), formatWebP},

		{"heic major brand", ftyp("heic", "mif1", "miaf"), formatHEIC},
		{"heix", ftyp("heix"), formatHEIC},
		{"hevc", ftyp("hevc"), formatHEIC},
		{"mif1 major with heic compat", ftyp("mif1", "heic", "mif1", "miaf"), formatHEIC},
		// The design lists mif1 itself as an accepted brand.
		{"mif1 major alone", ftyp("mif1"), formatHEIC},

		{"avif", ftyp("avif", "avif", "mif1", "miaf"), formatUnknown},
		{"avis", ftyp("avis", "avis", "mif1"), formatUnknown},
		// A mif1-major AVIF must not slip through via the accepted brand.
		{"avif behind mif1 major", ftyp("mif1", "avif", "mif1", "miaf"), formatUnknown},
		{"hevx", ftyp("hevx", "mif1"), formatUnknown},
		{"msf1", ftyp("msf1"), formatUnknown},
		{"heic listing avif is vetoed", ftyp("heic", "avif"), formatUnknown},
		{"unknown brand only", ftyp("isom", "mp41"), formatUnknown},
		{"ftyp box too small", append([]byte{0x00, 0x00, 0x00, 0x0C}, []byte("ftypheic")...), formatUnknown},

		{"gif", []byte("GIF89a\x01\x00\x01\x00"), formatUnknown},
		{"svg", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), formatUnknown},
		{"svg bare", []byte("<svg viewBox=\"0 0 8 8\"></svg>"), formatUnknown},
		{"text", []byte("hello, world"), formatUnknown},
		{"empty", nil, formatUnknown},
		{"riff not webp", []byte("RIFF\x24\x00\x00\x00WAVEfmt "), formatUnknown},
		{"truncated riff", []byte("RIFF"), formatUnknown},
		{"truncated ftyp", []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y'}, formatUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sniffFormat(tc.head); got != tc.want {
				t.Errorf("sniffFormat() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSniffFile pins the file wrapper: a clean empty file is a wrong type
// with no operational error, a missing file wraps ErrFailed, and a real
// file sniffs from its first 512 bytes only — a JPEG magic followed by
// anything else still sniffs as JPEG.
func TestSniffFile(t *testing.T) {
	dir := t.TempDir()

	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	if f, err := sniffFile(write("empty.bin", nil)); err != nil || f != formatUnknown {
		t.Errorf("sniffFile(empty) = (%v, %v), want (formatUnknown, nil)", f, err)
	}

	jpegPath := write("a.jpg", append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 1024)...))
	if f, err := sniffFile(jpegPath); err != nil || f != formatJPEG {
		t.Errorf("sniffFile(jpeg) = (%v, %v), want (formatJPEG, nil)", f, err)
	}

	missing := filepath.Join(dir, "nope.jpg")
	if f, err := sniffFile(missing); f != formatUnknown || !errors.Is(err, ErrFailed) {
		t.Errorf("sniffFile(missing) = (%v, %v), want ErrFailed", f, err)
	}
}
