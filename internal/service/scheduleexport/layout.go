package scheduleexport

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

const (
	colNotesWidth         = 40 // wie Dienstplanvorlage.xlsx Spalte I
	colASpacerWidth       = 2.45
	documentTitleRowHeight = 39.5
	defaultRowHeightPt    = 13
)

// Mindestbreiten Mo–Fr (Schichten) — etwas schmaler als zuvor.
var minShiftColWidth = 9.0

// columnWidths: Spalte B nur nach längstem Anzeigenamen; C–G nach Schicht-/Tageszellen.
type columnWidths struct {
	maxName int
	max     [10]int // Spalten 3–7
}

func (c *columnWidths) observeShift(col int, value string) {
	if col < 3 || col > 7 {
		return
	}
	n := utf8.RuneCountInString(value)
	if n > c.max[col] {
		c.max[col] = n
	}
}

func (c *columnWidths) observeEmployeeDisplayName(name string) {
	n := utf8.RuneCountInString(strings.TrimSpace(name))
	if n > c.maxName {
		c.maxName = n
	}
}

// excelBColWidth: knapp über dem längsten Mitarbeiternamen in Spalte B.
func excelBColWidth(runes int) float64 {
	if runes <= 0 {
		return 7
	}
	w := float64(runes)*0.95 + 2.8
	if w < 6.5 {
		return 6.5
	}
	if w > 48 {
		return 48
	}
	return w
}

func excelShiftColWidth(charCount int) float64 {
	if charCount <= 0 {
		return minShiftColWidth
	}
	w := float64(charCount)*1.05 + 1.2
	if w < minShiftColWidth {
		return minShiftColWidth
	}
	if w > 50 {
		return 50
	}
	return w
}

func applySheetLayout(f *excelize.File, sheet string, widths *columnWidths) error {
	if err := f.SetColWidth(sheet, "A", "A", colASpacerWidth); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "I", "I", colNotesWidth); err != nil {
		return err
	}
	colB, err := excelize.ColumnNumberToName(2)
	if err != nil {
		return err
	}
	nameW := excelBColWidth(widths.maxName)
	if err := f.SetColWidth(sheet, colB, colB, nameW); err != nil {
		return err
	}
	for col := 3; col <= 7; col++ {
		w := excelShiftColWidth(widths.max[col])
		colName, err := excelize.ColumnNumberToName(col)
		if err != nil {
			return err
		}
		if err := f.SetColWidth(sheet, colName, colName, w); err != nil {
			return err
		}
	}
	return nil
}

func formatDocumentTitle(firstMonday, lastFriday time.Time) string {
	return fmt.Sprintf("Dienstplan vom %02d.%02d. - %02d.%02d.%d",
		firstMonday.Day(), int(firstMonday.Month()),
		lastFriday.Day(), int(lastFriday.Month()), lastFriday.Year())
}

func writeDocumentTitle(f *excelize.File, sheet string, row int, title string, styleID int) error {
	if err := setMergedHeaderRow(f, sheet, row, title, styleID); err != nil {
		return err
	}
	return f.SetRowHeight(sheet, row, documentTitleRowHeight)
}

func setNormalRowHeight(f *excelize.File, sheet string, row int) error {
	return f.SetRowHeight(sheet, row, defaultRowHeightPt)
}

// applyHolidayColumnMerge schreibt den Feiertagsnamen senkrecht über alle Mitarbeiterzeilen einer Sektion.
func applyHolidayColumnMerge(f *excelize.File, sheet string, col, startRow, endRow int, holidayName string, styleID int) error {
	if holidayName == "" || endRow < startRow {
		return nil
	}
	topLeft, err := excelize.CoordinatesToCellName(col, startRow)
	if err != nil {
		return err
	}
	bottomRight, err := excelize.CoordinatesToCellName(col, endRow)
	if err != nil {
		return err
	}
	if endRow > startRow {
		if err := f.MergeCell(sheet, topLeft, bottomRight); err != nil {
			return err
		}
	}
	if err := f.SetCellValue(sheet, topLeft, holidayName); err != nil {
		return err
	}
	return f.SetCellStyle(sheet, topLeft, bottomRight, styleID)
}
