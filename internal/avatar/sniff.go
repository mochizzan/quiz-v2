package avatar

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

// sniffBytes is the sniff window of AVT-050: the first 512 bytes of the
// staged source. The client filename, extension, and Content-Type are never
// consulted — only these bytes decide the format.
const sniffBytes = 512

// format is the outcome of the byte sniff (AVT-050). formatUnknown is a
// deliberate, distinct result: SVG, GIF, AVIF, plain text, and zero bytes
// all land there and are refused with ErrType.
type format uint8

// The accepted formats. formatHEIC covers the HEIF containers phones emit;
// AVIF is NOT one of them in v1 — the product copy says "JPG, PNG, WebP,
// atau HEIC" (AVT-050).
const (
	formatUnknown format = iota
	formatJPEG
	formatPNG
	formatWebP
	formatHEIC
)

// heicBrands are the ISO-BMFF brands strukturag/libheif registers and v1
// accepts (AVT-050), including the generic mif1 major brand some writers
// use for HEIC.
var heicBrands = map[string]bool{
	"heic": true,
	"heix": true,
	"hevc": true,
	"heim": true,
	"heis": true,
	"hevm": true,
	"hevs": true,
	"mif1": true,
}

// rejectedBrands are container brands v1 refuses even when they share a
// file with an accepted brand: HEVC image sequences (hevx, msf1) are not
// supported, and AVIF (avif, avis) has its own product decision — rejected
// in v1 (AVT-050). Checking every brand in the ftyp box, not just the
// major one, is what keeps an AVIF that also lists mif1 out.
var rejectedBrands = map[string]bool{
	"hevx": true,
	"msf1": true,
	"avif": true,
	"avis": true,
}

// sniffFormat classifies the first 512 bytes of a source (AVT-050):
// JPEG/PNG through http.DetectContentType, WebP through the RIFF/WEBP
// signature, HEIC/HEIF through the ISO-BMFF ftyp box and its brands.
// Anything else — including SVG, GIF, AVIF, and empty input — is
// formatUnknown, which the pipeline refuses with ErrType.
func sniffFormat(head []byte) format {
	// HEIC/HEIF first: the ftyp box sits at bytes 4..8, which no other
	// accepted format can produce.
	if len(head) >= 16 && string(head[4:8]) == "ftyp" {
		if heicCompatible(head) {
			return formatHEIC
		}
		// An ftyp box with a rejected or foreign brand (AVIF, an
		// unknown brand) is a wrong type, not a decode failure.
		return formatUnknown
	}
	if len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP" {
		return formatWebP
	}
	switch http.DetectContentType(head) {
	case "image/jpeg":
		return formatJPEG
	case "image/png":
		return formatPNG
	default:
		return formatUnknown
	}
}

// heicCompatible decides whether the ftyp box of head describes a HEIC we
// accept (AVT-050). The box is [size:4][ftyp:4][major:4][minor:4][brands:4…];
// the major brand and every compatible brand are read, any rejected brand
// vetoes the file, and one accepted brand passes it. Brands that do not fit
// in the 512-byte sniff window are simply not seen — ftyp boxes are
// always small and first in the file.
func heicCompatible(head []byte) bool {
	boxSize := int(binary.BigEndian.Uint32(head[0:4]))
	if boxSize < 16 {
		return false
	}
	end := boxSize
	if end > len(head) {
		end = len(head)
	}
	ok := false
	check := func(brand string) bool {
		if rejectedBrands[brand] {
			return false
		}
		if heicBrands[brand] {
			ok = true
		}
		return true
	}
	if !check(string(head[8:12])) { // major brand
		return false
	}
	for off := 16; off+4 <= end; off += 4 { // compatible brands
		if !check(string(head[off : off+4])) {
			return false
		}
	}
	return ok
}

// sniffFile reads the first sniffBytes of the staged source and classifies
// them. A file that cannot be read (other than a clean EOF) is an
// operational failure, not a user error, and wraps ErrFailed; a clean empty
// file yields formatUnknown with no error, so it is refused as a wrong type.
func sniffFile(path string) (format, error) {
	f, err := os.Open(path)
	if err != nil {
		return formatUnknown, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, sniffBytes)
	n, err := f.Read(head)
	if err != nil && !errors.Is(err, io.EOF) {
		return formatUnknown, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if n == 0 {
		return formatUnknown, nil
	}
	return sniffFormat(head[:n]), nil
}
