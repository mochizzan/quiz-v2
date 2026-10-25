//go:build cgo

package avatar

// The cgo codecs of AVT-053/054/057: libwebp through
// github.com/kolesa-team/go-webp and libheif through
// github.com/strukturag/libheif's Go bindings (package heif — note the
// import path carries the go/heif subdirectory; the design's
// "NewContext/ReadFromFile/GetPrimaryImageHandle" names all exist there,
// but its HeifToJpeg helper does NOT, so the HEIC→JPEG step is written
// below from image.Decode + image/jpeg.Encode).
//
// Every file importing these libraries must keep this build tag: without
// a C toolchain (a bare Windows shell, CGO_ENABLED=0) the package still
// has to compile, and codec_nocgo.go carries the stubs for that (AVT-105).

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"

	"github.com/kolesa-team/go-webp/decoder"
	"github.com/kolesa-team/go-webp/encoder"
	heif "github.com/strukturag/libheif/go/heif"
)

// checkCodec reports whether the image codecs are usable: with cgo they
// are, so construction proceeds (AVT-105).
func checkCodec() error { return nil }

// probeWebP reads the WebP bitstream features for the pixel-ceiling check
// (AVT-051, AVT-053). NewDecoder reads the whole input, which is exactly
// why convertFile refuses a > 64 MiB source before calling here. A WebP
// whose features cannot be parsed is ErrUnreadable.
func probeWebP(path string) (dims, error) {
	f, err := os.Open(path)
	if err != nil {
		return dims{}, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	defer func() { _ = f.Close() }()
	dec, err := decoder.NewDecoder(f, nil)
	if err != nil {
		return dims{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	features := dec.GetFeatures()
	return dims{w: features.Width, h: features.Height}, nil
}

// decodeWebP decodes the source through libwebp (AVT-053) into an
// *image.NRGBA — the decoder's concrete result — so the common orientation
// and scale paths run on it without conversion. The 64 MiB guard has
// already run in convertFile. A libwebp failure is ErrUnreadable.
func decodeWebP(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	defer func() { _ = f.Close() }()
	dec, err := decoder.NewDecoder(f, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	img, err := dec.Decode()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	return img, nil
}

// probeHEIC reads the container's primary-image dimensions through
// strukturag/libheif (AVT-051, AVT-054 step 1): metadata only, no pixel
// decode. The design's sketch names — NewContext, ReadFromFile,
// GetPrimaryImageHandle, GetWidth/GetHeight — all exist verbatim in
// package heif. A file libheif cannot open or parse is ErrUnreadable, not
// ErrType (AVT-054).
func probeHEIC(path string) (dims, error) {
	ctx, err := heif.NewContext()
	if err != nil {
		return dims{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if err := ctx.ReadFromFile(path); err != nil {
		return dims{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	handle, err := ctx.GetPrimaryImageHandle()
	if err != nil {
		return dims{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	return dims{w: handle.GetWidth(), h: handle.GetHeight()}, nil
}

// decodeHEIC is AVT-054's HEIC → JPEG → decode path: convert the source
// to a temp JPEG at quality 95, remove the temp on EVERY path (defer), and
// decode that with the standard JPEG decoder, which feeds the common
// crop/scale path. The intermediate JPEG carries no EXIF, so the EXIF step
// never rotates the result twice, and the libheif decoder has already
// applied the container's irot/imir transforms. A HEIC libheif cannot
// decode — including the intermediate conversion failing — is
// ErrUnreadable; temp-file staging failures are ErrFailed.
func decodeHEIC(path string) (image.Image, error) {
	tmp, err := os.CreateTemp("", "avatar-heic-*.jpg")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return nil, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	defer func() { _ = os.Remove(tmpName) }() // AVT-054 step 4, every path

	if err := heifToJPEG(path, tmpName, heifJPEGQuality); err != nil {
		return nil, err
	}
	return decodeStdlib(tmpName)
}

// heifToJPEG performs AVT-054 step 2 — the design's go-libheif.HeifToJpeg
// call, reimplemented against the real strukturag/libheif API: decode the
// primary image through image.Decode (the heif package's init registers
// the ftyp-magic formats, and its decoder applies the container's irot/imir
// transforms) and re-encode it as JPEG at quality. A decode failure is
// ErrUnreadable — "a HEIC that libheif cannot read" (AVT-054) — while
// failures writing the intermediate are ErrFailed.
func heifToJPEG(srcPath, dstPath string, quality int) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	defer func() { _ = in.Close() }()
	img, err := image.Decode(in)
	if err != nil {
		return fmt.Errorf("%w: heif decode: %v", ErrUnreadable, err)
	}
	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if err := jpeg.Encode(out, img, &jpeg.Options{Quality: quality}); err != nil {
		_ = out.Close()
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	return nil
}

// encodeWebP is AVT-057: libwebp's PresetPhoto at the requested quality
// with method 4 — the quality/speed sweet spot for a 512 px avatar. The
// canvas is already *image.NRGBA, so the encoder skips its own conversion.
// The size ceiling (AVT-058) is enforced by encodeAvatar in convert.go,
// which owns the quality-60 retry.
func encodeWebP(img *image.NRGBA, quality float32) ([]byte, error) {
	opts, err := encoder.NewLossyEncoderOptions(encoder.PresetPhoto, quality)
	if err != nil {
		return nil, err
	}
	opts.Method = webpMethod
	enc, err := encoder.NewEncoder(img, opts)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := enc.Encode(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
