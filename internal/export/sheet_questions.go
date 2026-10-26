package export

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
	"golang.org/x/image/webp"

	"quiz-v2/internal/attempts"
	"quiz-v2/internal/authoring"
	"quiz-v2/internal/viewmodel"
)

var questionHeaders = []string{"No", "Tipe", "Soal", "Gambar", "Pilihan", "Kunci"}

func (b *builder) buildQuestions() {
	sheet := sheetQuestions
	for i, h := range questionHeaders {
		b.setStr(sheet, columnName(i+1)+"1", h)
	}
	b.styleRange(sheet, "A1", "F1", b.st.header)
	b.rowHeight(sheet, 1, 26)

	b.styleRange(sheet, "A2", fmt.Sprintf("F%d", b.qLast), b.st.body)

	// Set the final column widths before embedding pictures: excelize's
	// AutoFit measures the cell at AddPicture time. If widths are applied
	// afterwards, pictures are scaled against the default column and end up
	// as narrow strips.
	b.width(sheet, "A", "A", 6)
	b.width(sheet, "B", "B", 18)
	b.width(sheet, "C", "C", 50)
	b.width(sheet, "D", "D", 30)
	b.width(sheet, "E", "E", 45)
	b.width(sheet, "F", "F", 14)

	for outIdx, srcIdx := range b.qOrder {
		row := 2 + outIdx
		q := b.data.Questions[srcIdx]
		number := q.Position
		if number <= 0 {
			number = outIdx + 1
		}
		b.setNumber(sheet, fmt.Sprintf("A%d", row), number)
		b.setStr(sheet, fmt.Sprintf("B%d", row), q.Type.Label())
		b.setStr(sheet, fmt.Sprintf("C%d", row), q.Prompt)

		// The row height must also be final before AutoFit measures the cell.
		b.rowHeight(sheet, row, 96)

		switch {
		case q.ImagePath == "":
			// No image: leave the cell blank.
		case b.embedQuestionImage(row, q):
			// The picture floats over D; no cell text is needed.
		default:
			b.setStr(sheet, fmt.Sprintf("D%d", row), "[IMAGE]")
		}

		if q.Type.IsObjective() {
			opts := orderedOptions(q.Options)
			lines := make([]string, 0, len(opts))
			correct := make([]string, 0, len(opts))
			for i, opt := range opts {
				letter := viewmodel.OptionLetter(i)
				lines = append(lines, letter+". "+opt.Body)
				if opt.Correct {
					correct = append(correct, letter)
				}
			}
			b.setStr(sheet, fmt.Sprintf("E%d", row), strings.Join(lines, "\n"))
			key := strings.Join(correct, ", ")
			if key == "" {
				key = "—"
			}
			b.setStr(sheet, fmt.Sprintf("F%d", row), key)
		} else {
			b.setStr(sheet, fmt.Sprintf("F%d", row), "—")
		}

		b.styleCell(sheet, fmt.Sprintf("A%d", row), b.st.integer)
	}

	b.gridlines(sheet)
	b.zebra(sheet, fmt.Sprintf("A2:F%d", b.qLast))
	b.freeze(sheet, "A2", 0, 1, "bottomLeft")
}

func orderedOptions(opts []attempts.ExportOption) []attempts.ExportOption {
	order := optionOrder(opts)
	out := make([]attempts.ExportOption, 0, len(opts))
	for _, i := range order {
		out = append(out, opts[i])
	}
	return out
}

// embedQuestionImage resolves, validates, and embeds one image. Every failure
// returns false; the caller writes the "[IMAGE]" fallback.
func (b *builder) embedQuestionImage(row int, q attempts.ExportQuestion) bool {
	if b.err != nil || b.uploadsDir == "" || b.imageBudgetSpent {
		return false
	}
	name := q.ImagePath
	if !authoring.ValidImageName(name) || filepath.Base(name) != name {
		return false
	}
	if b.limits.maxPerImage <= 0 || b.limits.maxTotal <= 0 {
		return false
	}

	path := filepath.Join(b.uploadsDir, name)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > b.limits.maxPerImage {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 || int64(len(raw)) > b.limits.maxPerImage {
		return false
	}

	var (
		payload   []byte
		extension string
	)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg":
		payload, extension = raw, ".jpg"
	case ".png":
		payload, extension = raw, ".png"
	case ".webp":
		img, err := webp.Decode(bytes.NewReader(raw))
		if err != nil {
			return false
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			return false
		}
		payload, extension = encoded.Bytes(), ".png"
	default:
		return false
	}
	if int64(len(payload)) > b.limits.maxPerImage {
		return false
	}
	if b.totalImageBytes+int64(len(payload)) > b.limits.maxTotal {
		b.imageBudgetSpent = true
		return false
	}

	err = b.f.AddPictureFromBytes(sheetQuestions, fmt.Sprintf("D%d", row), &excelize.Picture{
		Extension: extension,
		File:      payload,
		Format: &excelize.GraphicOptions{
			AutoFit: true,
			AltText: sanitizeCell(q.ImageAlt),
		},
	})
	if err != nil {
		return false
	}
	b.totalImageBytes += int64(len(payload))
	return true
}
