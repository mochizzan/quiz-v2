package avatar

import (
	"encoding/binary"
	"image"
	"io"
	"os"

	"golang.org/x/image/draw"
)

// maxExifChunk caps the WebP EXIF RIFF chunk this reader will load: an
// orientation value lives in a few hundred bytes, and a hostile chunk
// header must not turn into a hostile allocation (AVT-055: malformed EXIF
// is ignored, never fatal).
const maxExifChunk = 16 << 20

// fileExifOrientation returns the EXIF display orientation stored in path's
// container — the JPEG APP1 0x0112 tag or the WebP EXIF RIFF chunk — as
// 1..8, or 0 when absent, malformed, or out of range (AVT-055). PNG eXIf is
// ignored in v1, and HEIC needs no reader here: libheif applies the
// container's irot/imir transforms itself and the intermediate JPEG carries
// no EXIF at all, so nothing is ever rotated twice (AVT-054).
//
// The file is opened here and walked marker by marker, seeking over
// everything it does not need, so a 100 MB JPEG costs at most one 64 KB
// segment read. Pure Go: no cgo, unit-testable under CGO_ENABLED=0.
func fileExifOrientation(f format, path string) int {
	switch f {
	case formatJPEG:
		return jpegExifOrientation(path)
	case formatWebP:
		return webpExifOrientation(path)
	default:
		return 0
	}
}

// jpegExifOrientation scans JPEG segments from SOI and returns the
// orientation of the first well-formed EXIF APP1 segment. The scan stops at
// SOS or EOI (entropy-coded data follows SOS, so no APP1 can appear later),
// skips XMP and non-EXIF APP1 segments, and returns 0 on any malformation —
// a broken EXIF block never fails the photo, it just leaves it unrotated.
func jpegExifOrientation(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	var sig [2]byte
	if _, err := io.ReadFull(f, sig[:]); err != nil || sig[0] != 0xFF || sig[1] != 0xD8 {
		return 0 // not even SOI: nothing to scan
	}
	for {
		marker, ok := readJPEGMarker(f)
		if !ok {
			return 0
		}
		if marker == 0xD9 || marker == 0xDA { // EOI or SOS: image data starts
			return 0
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) { // standalone markers carry no payload
			continue
		}
		var length [2]byte
		if _, err := io.ReadFull(f, length[:]); err != nil {
			return 0
		}
		segLen := int(binary.BigEndian.Uint16(length[:]))
		if segLen < 2 { // the length field itself is part of segLen
			return 0
		}
		payloadLen := segLen - 2
		if marker != 0xE1 { // not APP1: skip the payload
			if _, err := f.Seek(int64(payloadLen), io.SeekCurrent); err != nil {
				return 0
			}
			continue
		}
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(f, payload); err != nil {
			return 0
		}
		if len(payload) >= 6 && string(payload[:6]) == "Exif\x00\x00" {
			if o := tiffOrientation(payload[6:]); o != 0 {
				return o
			}
		}
		// XMP APP1 or a malformed EXIF block: keep scanning; the next
		// segment may still carry the orientation.
	}
}

// readJPEGMarker reads the next marker id after the 0xFF prefix, tolerating
// the 0xFF fill bytes the format allows. Anything byte-aligned wrong (a
// stuffed 0x00, stray data) reports not-ok, which the caller treats as
// malformed EXIF — ignored, never fatal (AVT-055).
func readJPEGMarker(f *os.File) (byte, bool) {
	var b [1]byte
	if _, err := io.ReadFull(f, b[:]); err != nil || b[0] != 0xFF {
		return 0, false
	}
	for {
		if _, err := io.ReadFull(f, b[:]); err != nil {
			return 0, false
		}
		if b[0] == 0xFF {
			continue // fill byte
		}
		if b[0] == 0x00 { // stuffed byte: only legal inside entropy data
			return 0, false
		}
		return b[0], true
	}
}

// webpExifOrientation walks the RIFF chunks of a WebP file and parses the
// EXIF chunk if one is present. Chunk sizes are little-endian and padded to
// even lengths; a truncated, oversized, or non-TIFF chunk yields 0
// (AVT-055: malformed EXIF is ignored).
func webpExifOrientation(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	var header [12]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return 0
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WEBP" {
		return 0
	}
	for {
		var chunk [8]byte
		if _, err := io.ReadFull(f, chunk[:]); err != nil {
			return 0 // ran out of chunks: no EXIF found
		}
		id := string(chunk[0:4])
		size := int64(binary.LittleEndian.Uint32(chunk[4:8]))
		if id == "EXIF" {
			if size < 8 || size > maxExifChunk {
				return 0 // too small to be TIFF, too big to trust
			}
			payload := make([]byte, size)
			if _, err := io.ReadFull(f, payload); err != nil {
				return 0
			}
			return tiffOrientation(payload)
		}
		if _, err := f.Seek(size+(size&1), io.SeekCurrent); err != nil {
			return 0
		}
	}
}

// tiffOrientation reads the Orientation tag (0x0112) from a bare TIFF/EXIF
// header in either byte order and returns 1..8, or 0 for anything absent,
// malformed, unsupported, or out of range (AVT-055). Every offset and
// length is bounds-checked before it is read, the entry count is capped by
// the data it claims to sit in, and a WebP chunk's optional "Exif\x00\x00"
// prefix is tolerated. The reader understands SHORT and LONG values of
// count 1; anything else is ignored rather than guessed at.
func tiffOrientation(data []byte) int {
	if len(data) >= 6 && string(data[:4]) == "Exif" && data[4] == 0 && data[5] == 0 {
		data = data[6:]
	}
	if len(data) < 8 {
		return 0
	}
	var littleEndian bool
	switch string(data[:2]) {
	case "II":
		littleEndian = true
	case "MM":
	default:
		return 0 // not a TIFF byte-order mark
	}
	u16 := func(off int) (int, bool) {
		if off < 0 || off+2 > len(data) {
			return 0, false
		}
		if littleEndian {
			return int(binary.LittleEndian.Uint16(data[off:])), true
		}
		return int(binary.BigEndian.Uint16(data[off:])), true
	}
	u32 := func(off int) (int, bool) {
		if off < 0 || off+4 > len(data) {
			return 0, false
		}
		if littleEndian {
			return int(binary.LittleEndian.Uint32(data[off:])), true
		}
		return int(binary.BigEndian.Uint32(data[off:])), true
	}
	if magic, ok := u16(2); !ok || magic != 42 {
		return 0
	}
	ifdOff, ok := u32(4)
	if !ok {
		return 0
	}
	count, ok := u16(ifdOff) // number of IFD0 entries
	if !ok {
		return 0
	}
	for i := 0; i < count; i++ {
		off := ifdOff + 2 + i*12 // each entry is 12 bytes
		if off < 0 || off+12 > len(data) {
			return 0 // the entry table runs past the data: malformed
		}
		if tag, ok := u16(off); !ok || tag != 0x0112 {
			continue
		}
		typ, ok := u16(off + 2)
		if !ok || (typ != 3 && typ != 4) { // SHORT or LONG only
			return 0
		}
		if n, ok := u32(off + 4); !ok || n != 1 { // count 1: value sits inline
			return 0
		}
		var val int
		if typ == 4 { // LONG: the value is all four bytes
			v, ok := u32(off + 8)
			if !ok {
				return 0
			}
			val = v
		} else { // SHORT: the value is the first two bytes, in file order
			v, ok := u16(off + 8)
			if !ok {
				return 0
			}
			val = v
		}
		if val < 1 || val > 8 {
			return 0 // orientations 1-8 only; anything else is ignored
		}
		return val
	}
	return 0
}

// applyOrientation rewinds the EXIF display transform (AVT-055), producing
// the upright image the crop must see: it runs BEFORE cropping. An absent,
// unknown, or identity orientation returns img untouched — the common case
// costs nothing. Orientations 5-8 transpose to a new canvas; 2-4 flip the
// pixels in place on one NRGBA copy, so no orientation ever needs two
// full-size buffers.
func applyOrientation(img image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return img
	}
	src := toNRGBA(img)
	w, h := src.Rect.Dx(), src.Rect.Dy()
	pix := src.Pix
	stride := src.Stride
	swap := func(x0, y0, x1, y1 int) {
		i := y0*stride + x0*4
		j := y1*stride + x1*4
		for k := 0; k < 4; k++ {
			pix[i+k], pix[j+k] = pix[j+k], pix[i+k]
		}
	}
	switch orientation {
	case 2: // flip horizontal
		for y := 0; y < h; y++ {
			for x := 0; x < w/2; x++ {
				swap(x, y, w-1-x, y)
			}
		}
	case 3: // rotate 180°: swap each pixel with its partner across the centre
		n := w * h // stride == 4*w (see toNRGBA), so pixels are dense
		for i := 0; i < n/2; i++ {
			si, di := i*4, (n-1-i)*4
			for k := 0; k < 4; k++ {
				pix[si+k], pix[di+k] = pix[di+k], pix[si+k]
			}
		}
	case 4: // flip vertical
		for y := 0; y < h/2; y++ {
			for x := 0; x < w; x++ {
				swap(x, y, x, h-1-y)
			}
		}
	case 5, 6, 7, 8: // transpose family: dimensions swap (AVT-055 table)
		dst := image.NewNRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < w; y++ { // dst height is the source width
			for x := 0; x < h; x++ { // dst width is the source height
				var sx, sy int
				switch orientation {
				case 5: // transpose
					sx, sy = y, x
				case 6: // rotate 90° CW
					sx, sy = y, h-1-x
				case 7: // transverse
					sx, sy = w-1-y, h-1-x
				case 8: // rotate 90° CCW
					sx, sy = w-1-y, x
				}
				si := sy*stride + sx*4
				di := y*dst.Stride + x*4
				copy(dst.Pix[di:di+4], pix[si:si+4])
			}
		}
		return dst
	}
	return src
}

// toNRGBA returns img as a tightly-packed *image.NRGBA. Decoded sources are
// almost always NRGBA already (WebP always is), so the common path is the
// image itself; anything else — YCbCr JPEG, RGBA, Gray — is converted once
// with a plain Src blit.
func toNRGBA(img image.Image) *image.NRGBA {
	if v, ok := img.(*image.NRGBA); ok &&
		v.Rect.Min == (image.Point{}) &&
		v.Stride == 4*v.Rect.Dx() {
		return v
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}
