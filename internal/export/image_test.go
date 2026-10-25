package export

// image_test.go covers the Soal image column (design §8): a valid PNG is
// embedded at the question row, WebP is transcoded to PNG, and every failure
// path (missing file, invalid name, per-image cap, total budget) writes the
// literal [IMAGE] fallback instead of failing the export.

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"quiz-v2/internal/attempts"
)

// pngOnePixel is a valid 1x1 image. The workbook never decodes PNG/JPEG, so
// the exact pixels do not matter; the signature and dimensions do.
func pngOnePixel(t *testing.T, c color.Color) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{c})); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func pngSolid(t *testing.T, width, height int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func jpgOnePixel(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.White}), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// webpOnePixel is a valid 1x1 lossy WebP. The decoder accepts it; the builder
// must not pass these bytes through unchanged because Excel cannot display WebP.
var webpOnePixel = mustBase64("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")

func mustBase64(value string) []byte {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return raw
}

// imageName builds a valid capability name: exactly 32 lowercase hex
// characters followed by the stored extension.
func imageName(n int, ext string) string {
	return fmt.Sprintf("%032x%s", n, ext)
}

func writeUpload(t *testing.T, dir, name string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatalf("write upload %s: %v", name, err)
	}
}

// imageData is one single-choice question with an image and one attempt, so the
// workbook has a D2 row but no dependency on participants for the assertion.
func imageData(name, mime string) attempts.ExportData {
	data := sampleExportData()
	data.Questions = data.Questions[:1]
	data.Questions[0].ImagePath = name
	data.Questions[0].ImageMIME = mime
	data.Questions[0].ImageAlt = "Diagram soal"
	data.Attempts = nil
	data.Participants = nil
	return data
}

func TestQuestionImageEmbeddedAndFallbacks(t *testing.T) {
	t.Run("png embeds", func(t *testing.T) {
		dir := t.TempDir()
		name := imageName(0, ".png")
		raw := pngOnePixel(t, color.White)
		writeUpload(t, dir, name, raw)

		f, _ := buildAndOpen(t, imageData(name, "image/png"), dir)
		pics, err := f.GetPictures("Soal", "D2")
		if err != nil {
			t.Fatalf("GetPictures: %v", err)
		}
		if len(pics) != 1 {
			t.Fatalf("pictures at D2 = %d, want 1", len(pics))
		}
		if pics[0].Extension != ".png" || !bytes.Equal(pics[0].File, raw) {
			t.Errorf("embedded picture = ext %q, %d bytes; want the source png", pics[0].Extension, len(pics[0].File))
		}
		if pics[0].Format == nil || pics[0].Format.AltText != "Diagram soal" {
			t.Errorf("alt text = %+v, want Diagram soal", pics[0].Format)
		}
		wantCell(t, f, "Soal", "D2", "")
	})

	t.Run("jpeg embeds", func(t *testing.T) {
		dir := t.TempDir()
		name := imageName(1, ".jpg")
		raw := jpgOnePixel(t)
		writeUpload(t, dir, name, raw)

		f, _ := buildAndOpen(t, imageData(name, "image/jpeg"), dir)
		pics, err := f.GetPictures("Soal", "D2")
		if err != nil {
			t.Fatalf("GetPictures: %v", err)
		}
		// Excelize normalizes .jpg to .jpeg in the stored media type.
		if len(pics) != 1 || pics[0].Extension != ".jpeg" || !bytes.Equal(pics[0].File, raw) {
			t.Fatalf("embedded picture = %+v, want the source jpeg", pics)
		}
	})

	t.Run("webp is transcoded to png", func(t *testing.T) {
		dir := t.TempDir()
		name := imageName(2, ".webp")
		writeUpload(t, dir, name, webpOnePixel)

		f, _ := buildAndOpen(t, imageData(name, "image/webp"), dir)
		pics, err := f.GetPictures("Soal", "D2")
		if err != nil {
			t.Fatalf("GetPictures: %v", err)
		}
		if len(pics) != 1 {
			t.Fatalf("pictures at D2 = %d, want 1", len(pics))
		}
		if pics[0].Extension != ".png" {
			t.Errorf("extension = %q, want .png after WebP transcode", pics[0].Extension)
		}
		if _, err := png.Decode(bytes.NewReader(pics[0].File)); err != nil {
			t.Errorf("transcoded picture is not a png: %v", err)
		}
		if bytes.HasPrefix(pics[0].File, []byte("RIFF")) {
			t.Error("WebP bytes were embedded unchanged")
		}
	})

	for _, tc := range []struct {
		name string
		path string
		raw  []byte
	}{
		{"missing file", imageName(3, ".png"), nil},
		{"invalid image name", "not-a-hash.png", pngOnePixel(t, color.White)},
		{"broken webp", imageName(4, ".webp"), []byte("not a webp")},
	} {
		t.Run(tc.name+" falls back", func(t *testing.T) {
			dir := t.TempDir()
			if tc.raw != nil {
				writeUpload(t, dir, tc.path, tc.raw)
			}
			f, _ := buildAndOpen(t, imageData(tc.path, "image/png"), dir)
			wantCell(t, f, "Soal", "D2", "[IMAGE]")
			pics, err := f.GetPictures("Soal", "D2")
			if err != nil {
				t.Fatalf("GetPictures: %v", err)
			}
			if len(pics) != 0 {
				t.Errorf("pictures at D2 = %d, want none", len(pics))
			}
		})
	}
}

// TestQuestionImageAutoFitUsesFinalCellSize guards the ordering in
// buildQuestions: AutoFit measures the cell when AddPicture runs, so the row
// height and column widths must already be final. Otherwise a real image is
// scaled against the default 20 px row and later stretched into a strip.
func TestQuestionImageAutoFitUsesFinalCellSize(t *testing.T) {
	dir := t.TempDir()
	name := imageName(8, ".png")
	writeUpload(t, dir, name, pngSolid(t, 240, 180, color.RGBA{R: 41, G: 54, B: 129, A: 255}))

	_, workbook := buildAndOpen(t, imageData(name, "image/png"), dir)

	zr, err := zip.NewReader(bytes.NewReader(workbook), int64(len(workbook)))
	if err != nil {
		t.Fatalf("open workbook zip: %v", err)
	}
	var drawingXML []byte
	for _, part := range zr.File {
		if part.Name != "xl/drawings/drawing1.xml" {
			continue
		}
		rc, err := part.Open()
		if err != nil {
			t.Fatalf("open drawing: %v", err)
		}
		drawingXML, err = io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read drawing: %v", err)
		}
		break
	}
	if drawingXML == nil {
		t.Fatal("workbook has no drawing part")
	}

	var drawing struct {
		TwoCellAnchor struct {
			Pic struct {
				SpPr struct {
					Xfrm struct {
						Ext struct {
							Cx int64 `xml:"cx,attr"`
							Cy int64 `xml:"cy,attr"`
						} `xml:"ext"`
					} `xml:"xfrm"`
				} `xml:"spPr"`
			} `xml:"pic"`
		} `xml:"twoCellAnchor"`
	}
	if err := xml.Unmarshal(drawingXML, &drawing); err != nil {
		t.Fatalf("parse drawing: %v", err)
	}
	ext := drawing.TwoCellAnchor.Pic.SpPr.Xfrm.Ext
	width, height := float64(ext.Cx)/9525, float64(ext.Cy)/9525
	if width < 100 || height < 100 {
		t.Errorf("picture is %.0fx%.0f px, want it scaled against the final 96 pt row", width, height)
	}
	if ratio := width / height; math.Abs(ratio-4.0/3.0) > 0.05 {
		t.Errorf("picture aspect ratio = %.3f, want ~1.333", ratio)
	}
}

func TestQuestionImageLimits(t *testing.T) {
	raw := pngOnePixel(t, color.White)

	t.Run("per-image cap", func(t *testing.T) {
		dir := t.TempDir()
		name := imageName(5, ".png")
		writeUpload(t, dir, name, raw)

		workbook, err := build(imageData(name, "image/png"), dir, imageLimits{
			maxPerImage: int64(len(raw) - 1),
			maxTotal:    1 << 20,
		})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		f := openWorkbook(t, workbook)
		wantCell(t, f, "Soal", "D2", "[IMAGE]")
	})

	t.Run("total budget stops at the first image", func(t *testing.T) {
		dir := t.TempDir()
		first := imageName(6, ".png")
		second := imageName(7, ".png")
		writeUpload(t, dir, first, raw)
		writeUpload(t, dir, second, raw)

		data := imageData(first, "image/png")
		data.Questions = append(data.Questions, attempts.ExportQuestion{
			ID:        202,
			Position:  2,
			Type:      "single",
			Prompt:    "Soal kedua",
			ImagePath: second,
			ImageMIME: "image/png",
		})
		// A strict total lets the first image through and refuses the second.
		workbook, err := build(data, dir, imageLimits{
			maxPerImage: int64(len(raw)),
			maxTotal:    int64(len(raw))*2 - 1,
		})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		f := openWorkbook(t, workbook)

		pics, err := f.GetPictures("Soal", "D2")
		if err != nil {
			t.Fatalf("GetPictures D2: %v", err)
		}
		if len(pics) != 1 {
			t.Errorf("pictures at D2 = %d, want the first image embedded", len(pics))
		}
		wantCell(t, f, "Soal", "D3", "[IMAGE]")
	})
}
