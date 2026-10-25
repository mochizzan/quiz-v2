package avatar

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg" // registers JPEG for image.Decode/DecodeConfig (AVT-051/052)
	_ "image/png"  // registers PNG likewise
	"os"
)

// dims are source dimensions probed BEFORE any pixel decode (AVT-051):
// headers for JPEG/PNG, container metadata for WebP/HEIC.
type dims struct {
	w, h int
}

// Encode and conversion policy (AVT-054, AVT-057, AVT-058): WebP quality
// 80 at method 4, the quality-60 retry when the output would exceed the
// 5 MB cap, and quality 95 for the intermediate HEIC JPEG so the double
// loss is invisible at 512 px.
const (
	webpQualityPrimary  = float32(80)
	webpQualityFallback = float32(60)
	webpMethod          = 4
	heifJPEGQuality     = 95
)

// convertFile runs the whole conversion for one staged source file
// (AVT-050..AVT-058): sniff → 64 MiB guard → probe dimensions → pixel
// ceiling → decode → EXIF orientation → centre-crop and scale → WebP
// encode → atomic write. It returns one of the sentinel errors and writes
// dstPath only on success — dstPath must be a file name inside dir (the
// caller generates it and owns every other file operation). The source at
// srcPath is never removed here; that stays the caller's defer (AVT-042).
//
// ctx is checked at stage boundaries: it can refuse work that has not
// started, but never pre-empts a decode already inside a cgo codec — the
// caps, not a wall-clock timeout, bound the work (AVT-059).
func convertFile(ctx context.Context, dir, srcPath, dstPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := sniffFile(srcPath)
	if err != nil {
		return err
	}
	if f == formatUnknown {
		return ErrType // AVT-050: SVG, GIF, AVIF, text, zero bytes, …
	}
	// WebP and HEIC are the only formats whose libraries read the whole
	// compressed file into memory; refuse a > 64 MiB source before either
	// probe touches it (AVT-053, AVT-054) — separate from the 50 MP
	// ceiling below and applied even when the pixel count would be small.
	if f == formatWebP || f == formatHEIC {
		info, serr := os.Stat(srcPath)
		if serr != nil {
			return fmt.Errorf("%w: %w", ErrFailed, serr)
		}
		if info.Size() > MaxSourceBytes {
			return ErrBig
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	size, err := probeImage(f, srcPath)
	if err != nil {
		return err // ErrBig or ErrUnreadable, already classified
	}
	if size.w <= 0 || size.h <= 0 {
		return fmt.Errorf("%w: source reports %dx%d pixels", ErrUnreadable, size.w, size.h)
	}
	// The pixel ceiling (AVT-051), int64 so a hostile 65535×65535 header
	// cannot overflow the product on any platform. MaxSide is checked on
	// its own: a 16001×100 strip is refused by side, not by area.
	if size.w > MaxSide || size.h > MaxSide || int64(size.w)*int64(size.h) > MaxPixels {
		return ErrBig
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	img, err := decodeSource(f, srcPath)
	if err != nil {
		return err // ErrUnreadable (or ErrFailed for staging I/O)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// EXIF orientation runs BEFORE cropping (AVT-055): the crop must see
	// the upright photo, not the sensor's. PNG ignores eXIf in v1 and HEIC
	// is already upright from libheif's irot/imir handling.
	img = applyOrientation(img, fileExifOrientation(f, srcPath))

	canvas, err := transformToAvatar(img)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := encodeAvatar(canvas, encodeWebP)
	if err != nil {
		return err // ErrFailed, nothing written
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeAvatar(dir, dstPath, data)
}

// probeImage returns the source's pixel dimensions without decoding pixels
// (AVT-051): image.DecodeConfig for JPEG/PNG (header only), the WebP
// bitstream features, and libheif's container metadata for HEIC. Probe
// failures classify as ErrUnreadable — the format is right, the file is
// not — except for an unopenable source, which wraps ErrFailed.
func probeImage(f format, path string) (dims, error) {
	switch f {
	case formatJPEG, formatPNG:
		return probeStdlib(path)
	case formatWebP:
		return probeWebP(path)
	case formatHEIC:
		return probeHEIC(path)
	default:
		return dims{}, ErrType
	}
}

// probeStdlib is the header-only JPEG/PNG probe (AVT-051): no pixel is
// decoded, so a 50 MP image is refused here in microseconds.
func probeStdlib(path string) (dims, error) {
	f, err := os.Open(path)
	if err != nil {
		return dims{}, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return dims{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	return dims{w: cfg.Width, h: cfg.Height}, nil
}

// decodeSource decodes the source to pixels (AVT-052..AVT-054). Every
// decoder failure is ErrUnreadable: the sniff said the type is right, the
// bytes do not hold together. The source file itself is only read.
func decodeSource(f format, path string) (image.Image, error) {
	switch f {
	case formatJPEG, formatPNG:
		return decodeStdlib(path)
	case formatWebP:
		return decodeWebP(path)
	case formatHEIC:
		return decodeHEIC(path)
	default:
		return nil, ErrType
	}
}

// decodeStdlib is the AVT-052 path: standard-library JPEG and PNG decode.
// A failure — truncated file, bad CRC, corrupt entropy data — is
// ErrUnreadable, never ErrType: the magic bytes already said the type.
func decodeStdlib(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	return img, nil
}

// encodeAvatar implements the WebP encode policy (AVT-057, AVT-058):
// PresetPhoto at quality 80 and method 4; if the output would exceed
// MaxAvatarBytes it is re-encoded at quality 60; if it STILL exceeds the
// cap the conversion fails with ErrFailed and nothing is written. The
// encoder is injected so the size policy is unit-testable without cgo.
func encodeAvatar(canvas *image.NRGBA, encode func(*image.NRGBA, float32) ([]byte, error)) ([]byte, error) {
	data, err := encode(canvas, webpQualityPrimary)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if len(data) <= MaxAvatarBytes {
		return data, nil
	}
	data, err = encode(canvas, webpQualityFallback)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if len(data) > MaxAvatarBytes {
		return nil, fmt.Errorf("%w: encoded avatar is %d bytes, over %d even at quality 60",
			ErrFailed, len(data), MaxAvatarBytes)
	}
	return data, nil
}

// writeAvatar stages data inside dir as a dot-prefixed .tmp file, fsyncs
// it, renames it onto dstPath, and chmods it 0640 (§4.2): a crash at any
// point leaves at worst a dot-prefixed temp file that fails the avatar
// grammar, never a half-written avatar under a live name. dir is created
// lazily on first write with mode 0750. Every failure removes the temp
// file (and the renamed file if the final chmod fails) and wraps
// ErrFailed — dstPath is written only in full, or not at all.
func writeAvatar(dir, dstPath string, data []byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	tmp, err := os.CreateTemp(dir, ".avatar-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if err := os.Rename(tmpName, dstPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if err := os.Chmod(dstPath, 0o640); err != nil {
		_ = os.Remove(dstPath)
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	return nil
}
