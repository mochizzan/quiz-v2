//go:build !cgo

package avatar

// CGO_ENABLED=0 stubs (AVT-105). The pure-Go parts of this package —
// sniffing, the EXIF reader, the transform, the encode size policy, and
// the pool — compile and test without a C toolchain; the codecs themselves
// (libwebp, libheif) cannot exist without one. checkCodec fails the
// constructor with the required message, so the stubs below are never
// reached in a running server; they exist so the package builds, and
// convertFile reports ErrFailed if invoked directly in a test.

import (
	"errors"
	"fmt"
	"image"
)

// errNoCGO is the constructor's failure under CGO_ENABLED=0 (AVT-105): the
// image codecs require cgo, and this machine has no C toolchain.
var errNoCGO = errors.New("avatar: cgo image support requires CGO_ENABLED=1")

// checkCodec refuses construction without cgo.
func checkCodec() error { return errNoCGO }

// noCodec is the stub failure for any codec entry point.
func noCodec() error { return fmt.Errorf("%w: %w", ErrFailed, errNoCGO) }

func probeWebP(string) (dims, error) { return dims{}, noCodec() }

func probeHEIC(string) (dims, error) { return dims{}, noCodec() }

func decodeWebP(string) (image.Image, error) { return nil, noCodec() }

func decodeHEIC(string) (image.Image, error) { return nil, noCodec() }

func encodeWebP(*image.NRGBA, float32) ([]byte, error) { return nil, noCodec() }
