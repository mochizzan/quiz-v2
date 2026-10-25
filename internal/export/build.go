// Package export builds the owner-only quiz XLSX workbook described by
// docs/superpowers/specs/2026-10-25-quiz-xlsx-export-design.md.
package export

import (
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"

	"quiz-v2/internal/attempts"
	"quiz-v2/internal/authoring"
)

const (
	sheetSummary      = "Ringkasan"
	sheetParticipants = "Peserta"
	sheetQuestions    = "Soal"
	sheetGrading      = "Penilaian"

	// maxTotalImageBytes is the whole-workbook image budget from §8.
	maxTotalImageBytes = 50 << 20
)

// imageLimits is parameterised so tests can exercise the per-image and total
// fallbacks without writing a 50 MB workbook.
type imageLimits struct {
	maxPerImage int64
	maxTotal    int64
}

var defaultImageLimits = imageLimits{
	maxPerImage: int64(authoring.MaxImageBytes),
	maxTotal:    int64(maxTotalImageBytes),
}

type styles struct {
	header  int
	body    int
	label   int
	date    int
	score   int
	average int
	integer int
	note    int
	best    int
}

type builder struct {
	f          *excelize.File
	data       attempts.ExportData
	uploadsDir string
	limits     imageLimits
	st         styles

	qLast    int
	pLast    int
	aLast    int
	hLast    int
	rankLast int
	h        int
	finalCol string
	qOrder   []int

	totalImageBytes  int64
	imageBudgetSpent bool
	err              error
}

// Build renders one workbook from a read-model snapshot. uploadsDir is the
// configured image root; an image that cannot be embedded becomes "[IMAGE]".
func Build(data attempts.ExportData, uploadsDir string) ([]byte, error) {
	return build(data, uploadsDir, defaultImageLimits)
}

func build(data attempts.ExportData, uploadsDir string, limits imageLimits) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	b := &builder{
		f:          f,
		data:       data,
		uploadsDir: uploadsDir,
		limits:     limits,
	}
	b.initLayout()
	b.initStyles()
	if b.err != nil {
		return nil, fmt.Errorf("export: build workbook: %w", b.err)
	}

	for _, name := range []string{sheetSummary, sheetParticipants, sheetQuestions, sheetGrading} {
		if _, err := f.NewSheet(name); err != nil {
			return nil, fmt.Errorf("export: create sheet %s: %w", name, err)
		}
	}
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, fmt.Errorf("export: remove default sheet: %w", err)
	}
	if idx, err := f.GetSheetIndex(sheetSummary); err == nil {
		f.SetActiveSheet(idx)
	}

	b.buildParticipants()
	b.buildQuestions()
	b.buildGrading()
	b.buildSummary()
	b.setCalcProps()

	if b.err != nil {
		return nil, fmt.Errorf("export: build workbook: %w", b.err)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("export: write workbook: %w", err)
	}
	return buf.Bytes(), nil
}

func (b *builder) initLayout() {
	q := len(b.data.Questions)
	p := len(b.data.Participants)
	a := len(b.data.Attempts)
	h := len(participantsWithAttempts(b.data.Participants, b.data.Attempts))

	b.h = h
	b.qLast = maxInt(2, q+1)
	b.pLast = maxInt(2, p+1)
	b.aLast = maxInt(2, a+1)
	b.hLast = maxInt(2, h+1)
	b.rankLast = 18 + maxInt(1, h)
	b.finalCol = columnName(7 + 2*q)
	b.qOrder = questionOrder(b.data.Questions)
}

func (b *builder) initStyles() {
	border := []excelize.Border{
		{Type: "left", Color: "#D9DCE3", Style: 1},
		{Type: "right", Color: "#D9DCE3", Style: 1},
		{Type: "top", Color: "#D9DCE3", Style: 1},
		{Type: "bottom", Color: "#D9DCE3", Style: 1},
	}
	wrapTop := &excelize.Alignment{Vertical: "top", WrapText: true}
	center := &excelize.Alignment{Vertical: "top", Horizontal: "center", WrapText: true}

	b.st.header = b.newStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#293681"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    border,
	})
	b.st.body = b.newStyle(&excelize.Style{
		Alignment: wrapTop,
		Border:    border,
	})
	b.st.label = b.newStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: wrapTop,
		Border:    border,
	})
	b.st.date = b.newStyle(&excelize.Style{
		Alignment:    center,
		Border:       border,
		CustomNumFmt: strPtr("yyyy-mm-dd hh:mm"),
	})
	b.st.score = b.newStyle(&excelize.Style{
		Alignment:    center,
		Border:       border,
		CustomNumFmt: strPtr("0.##"),
	})
	b.st.average = b.newStyle(&excelize.Style{
		Alignment:    center,
		Border:       border,
		CustomNumFmt: strPtr("0.0"),
	})
	b.st.integer = b.newStyle(&excelize.Style{
		Alignment:    center,
		Border:       border,
		CustomNumFmt: strPtr("0"),
	})
	b.st.note = b.newStyle(&excelize.Style{
		Font:      &excelize.Font{Italic: true, Color: "#5A6172"},
		Alignment: wrapTop,
	})
	b.st.best = b.newConditionalStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#FFF2CC"}},
	})
}

func (b *builder) newStyle(style *excelize.Style) int {
	if b.err != nil {
		return 0
	}
	id, err := b.f.NewStyle(style)
	if err != nil {
		b.err = err
		return 0
	}
	return id
}

func (b *builder) newConditionalStyle(style *excelize.Style) int {
	if b.err != nil {
		return 0
	}
	id, err := b.f.NewConditionalStyle(style)
	if err != nil {
		b.err = err
		return 0
	}
	return id
}

func (b *builder) setCalcProps() {
	if b.err != nil {
		return
	}
	mode := "auto"
	yes := true
	b.fail(b.f.SetCalcProps(&excelize.CalcPropsOptions{
		CalcMode:       &mode,
		FullCalcOnLoad: &yes,
		ForceFullCalc:  &yes,
		CalcOnSave:     &yes,
	}))
}

func (b *builder) fail(err error) {
	if b.err == nil && err != nil {
		b.err = err
	}
}

func (b *builder) setStr(sheet, cell, value string) {
	if b.err != nil {
		return
	}
	value = sanitizeCell(value)
	if value == "" {
		return
	}
	b.fail(b.f.SetCellStr(sheet, cell, value))
}

func (b *builder) setFormula(sheet, cell, formula string) {
	if b.err != nil || formula == "" {
		return
	}
	b.fail(b.f.SetCellFormula(sheet, cell, formula))
}

func (b *builder) setNumber(sheet, cell string, value any) {
	if b.err != nil {
		return
	}
	b.fail(b.f.SetCellValue(sheet, cell, value))
}

func (b *builder) setDate(sheet, cell string, t time.Time) {
	if t.IsZero() {
		b.setStr(sheet, cell, "—")
		return
	}
	if b.err != nil {
		return
	}
	b.fail(b.f.SetCellValue(sheet, cell, excelTime(t)))
}

func (b *builder) setOptionalDate(sheet, cell string, t *time.Time) {
	if t == nil || t.IsZero() {
		b.setStr(sheet, cell, "—")
		return
	}
	b.setDate(sheet, cell, *t)
}

func (b *builder) styleRange(sheet, topLeft, bottomRight string, id int) {
	if b.err != nil || id == 0 {
		return
	}
	b.fail(b.f.SetCellStyle(sheet, topLeft, bottomRight, id))
}

func (b *builder) styleCell(sheet, cell string, id int) {
	b.styleRange(sheet, cell, cell, id)
}

func (b *builder) width(sheet, startCol, endCol string, width float64) {
	if b.err != nil {
		return
	}
	b.fail(b.f.SetColWidth(sheet, startCol, endCol, width))
}

func (b *builder) rowHeight(sheet string, row int, height float64) {
	if b.err != nil {
		return
	}
	b.fail(b.f.SetRowHeight(sheet, row, height))
}

func (b *builder) hideCols(sheet, columns string) {
	if b.err != nil {
		return
	}
	b.fail(b.f.SetColVisible(sheet, columns, false))
}

func (b *builder) freeze(sheet, topLeft string, xSplit, ySplit int, activePane string) {
	if b.err != nil {
		return
	}
	b.fail(b.f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      xSplit,
		YSplit:      ySplit,
		TopLeftCell: topLeft,
		ActivePane:  activePane,
	}))
}

func (b *builder) autoFilter(sheet, ref string) {
	if b.err != nil {
		return
	}
	b.fail(b.f.AutoFilter(sheet, ref, nil))
}

func strPtr(s string) *string { return &s }
