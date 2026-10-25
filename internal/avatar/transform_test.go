package avatar

import (
	"errors"
	"image"
	"image/color"
	"testing"
)

// The three test bands. A centre crop of a banded source must keep the
// middle band and drop the outer two, which is what pins AVT-056's
// "centre-cropped on the short side".
var (
	bandRed   = color.NRGBA{R: 200, A: 255}
	bandGreen = color.NRGBA{G: 200, A: 255}
	bandBlue  = color.NRGBA{B: 200, A: 255}
)

// fillBands paints red|green|blue across img with the two given cut
// points on the chosen axis: pixels below cuts[0] are red, below cuts[1]
// green, the rest blue. The cuts are absolute bounds-relative offsets, so
// a band boundary can sit exactly on a crop boundary.
func fillBands(img *image.NRGBA, cuts [2]int, vertical bool) {
	b := img.Bounds()
	cols := [3]color.Color{bandRed, bandGreen, bandBlue}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			off := x - b.Min.X
			if !vertical {
				off = y - b.Min.Y
			}
			i := 2
			switch {
			case off < cuts[0]:
				i = 0
			case off < cuts[1]:
				i = 1
			}
			img.Set(x, y, cols[i])
		}
	}
}

// assertOpaqueColor checks one output pixel: exact alpha (AVT-056 says no
// alpha survives into the stored WebP — the white canvas must win), and
// RGB within a small tolerance of what CatmullRom scaling of a uniform
// region should produce.
func assertOpaqueColor(t *testing.T, img image.Image, x, y int, want color.NRGBA, what string) {
	t.Helper()
	b := img.Bounds()
	c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
	if c.A != 255 {
		t.Errorf("%s at (%d,%d) alpha = %d, want 255 (opaque)", what, x, y, c.A)
	}
	tol := func(a, b uint8) int {
		d := int(a) - int(b)
		if d < 0 {
			return -d
		}
		return d
	}
	if tol(c.R, want.R) > 4 || tol(c.G, want.G) > 4 || tol(c.B, want.B) > 4 {
		t.Errorf("%s at (%d,%d) = %v, want ~%v", what, x, y, c, want)
	}
}

// assertAvatarBounds pins the stored geometry: exactly OutputSize² at the
// origin, whatever the source looked like.
func assertAvatarBounds(t *testing.T, img image.Image) {
	t.Helper()
	if b := img.Bounds(); b != (image.Rect(0, 0, OutputSize, OutputSize)) {
		t.Fatalf("bounds = %v, want 0,0,%d,%d", b, OutputSize, OutputSize)
	}
}

// TestTransformCropsCenterLandscape: a 400×200 source with red|green|blue
// bands cut at 100 and 300 centre-crops to exactly the green band (the
// crop starts at x=100 and ends at x=300), so every sampled output pixel
// is green — a crop anchored at either edge would show red or blue.
func TestTransformCropsCenterLandscape(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	fillBands(src, [2]int{100, 300}, true)

	out, err := transformToAvatar(src)
	if err != nil {
		t.Fatalf("transformToAvatar: %v", err)
	}
	assertAvatarBounds(t, out)

	// Sample an inset grid: the outermost pixels may still touch the band
	// boundary through the filter kernel, everything inside must be green.
	for y := 40; y <= 470; y += 43 {
		for x := 40; x <= 470; x += 43 {
			assertOpaqueColor(t, out, x, y, bandGreen, "cropped landscape")
		}
	}
}

// TestTransformCropsCenterPortrait is the vertical twin: the short side is
// height, so the crop window is y=100..300 and only the green band may
// reach the output.
func TestTransformCropsCenterPortrait(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 200, 400))
	fillBands(src, [2]int{100, 300}, false)

	out, err := transformToAvatar(src)
	if err != nil {
		t.Fatalf("transformToAvatar: %v", err)
	}
	assertAvatarBounds(t, out)
	for y := 40; y <= 470; y += 43 {
		for x := 40; x <= 470; x += 43 {
			assertOpaqueColor(t, out, x, y, bandGreen, "cropped portrait")
		}
	}
}

// TestTransformSquareSourceIsNotCropped: a square source keeps all of
// itself — no band is lost to the crop.
func TestTransformSquareSourceIsNotCropped(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	fillBands(src, [2]int{66, 133}, true)

	out, err := transformToAvatar(src)
	if err != nil {
		t.Fatalf("transformToAvatar: %v", err)
	}
	assertAvatarBounds(t, out)
	// The left third of the square (x < 66 of 200) maps to x < ~169 of 512.
	assertOpaqueColor(t, out, 80, 256, bandRed, "square left third")
	assertOpaqueColor(t, out, 256, 256, bandGreen, "square middle third")
	assertOpaqueColor(t, out, 430, 256, bandBlue, "square right third")
}

// TestTransformCompositesOntoWhite pins the alpha rule of AVT-056: a
// transparent source is composited onto the pre-filled white canvas, so
// the stored avatar is opaque white rather than transparent or tinted by
// the source's colour channels.
func TestTransformCompositesOntoWhite(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 200, G: 50, B: 50, A: 0}) // fully transparent red
		}
	}

	out, err := transformToAvatar(src)
	if err != nil {
		t.Fatalf("transformToAvatar: %v", err)
	}
	assertAvatarBounds(t, out)
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for _, p := range [][2]int{{0, 0}, {256, 256}, {511, 511}, {40, 470}} {
		assertOpaqueColor(t, out, p[0], p[1], white, "transparent source")
	}
}

// TestTransformScalesSmallSource: a tiny source still lands on the full
// OutputSize canvas (upscaled by CatmullRom like any other).
func TestTransformScalesSmallSource(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.SetNRGBA(x, y, bandGreen)
		}
	}

	out, err := transformToAvatar(src)
	if err != nil {
		t.Fatalf("transformToAvatar: %v", err)
	}
	assertAvatarBounds(t, out)
	for _, p := range [][2]int{{0, 0}, {256, 256}, {511, 511}} {
		assertOpaqueColor(t, out, p[0], p[1], bandGreen, "4px source")
	}
}

// TestTransformNonZeroOrigin: sources whose bounds do not start at the
// origin (a SubImage, say) crop correctly — the window is computed from
// b.Min, not from zero.
func TestTransformNonZeroOrigin(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 5, 405, 205))
	fillBands(src, [2]int{100, 300}, true) // offsets are bounds-relative

	out, err := transformToAvatar(src)
	if err != nil {
		t.Fatalf("transformToAvatar: %v", err)
	}
	assertAvatarBounds(t, out)
	for y := 40; y <= 470; y += 43 {
		for x := 40; x <= 470; x += 43 {
			assertOpaqueColor(t, out, x, y, bandGreen, "offset source")
		}
	}
}

// TestTransformEmptySource: a decoded image with no pixels is
// ErrUnreadable — the format was right, the payload is not (AVT-056).
func TestTransformEmptySource(t *testing.T) {
	_, err := transformToAvatar(image.NewNRGBA(image.Rect(0, 0, 0, 0)))
	if !errors.Is(err, ErrUnreadable) {
		t.Errorf("empty source: got %v, want ErrUnreadable", err)
	}
	_, err = transformToAvatar(image.NewNRGBA(image.Rect(0, 0, 0, 10)))
	if !errors.Is(err, ErrUnreadable) {
		t.Errorf("zero-width source: got %v, want ErrUnreadable", err)
	}
}
