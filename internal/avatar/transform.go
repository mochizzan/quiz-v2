package avatar

import (
	"fmt"
	"image"

	"golang.org/x/image/draw"
)

// transformToAvatar builds the stored avatar geometry (AVT-056): the
// square is centre-cropped on the source's short side, then scaled to
// OutputSize×OutputSize with the CatmullRom filter onto a fresh NRGBA
// canvas that has been pre-filled opaque white. The scale draws with
// draw.Over, so any source alpha — a transparent PNG, a WebP with an alpha
// channel — composites onto that white and every output pixel is opaque:
// no alpha survives into the encoded WebP.
//
// The crop rectangle is clamped to the source bounds by construction
// (it never exceeds them), so no sampler ever reads outside the image.
func transformToAvatar(src image.Image) (*image.NRGBA, error) {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	if side <= 0 {
		return nil, fmt.Errorf("%w: decoded image has no pixels", ErrUnreadable)
	}
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	crop := image.Rect(x0, y0, x0+side, y0+side)

	dst := image.NewNRGBA(image.Rect(0, 0, OutputSize, OutputSize))
	// Pre-fill white (AVT-056): the destination is opaque before the
	// first sample lands, which is what makes draw.Over composite onto
	// white instead of onto nothing.
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Over, nil)
	return dst, nil
}
