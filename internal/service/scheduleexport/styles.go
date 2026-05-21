package scheduleexport

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

const fontFamilyArial = "Arial"

// Wochen-Füllfarben (5er-Rotation) wie Dienstplanvorlage.xlsx.
var weekFillColors = [5]string{
	"00B0F0", // hellblau
	"FFF2CC", // hellgelb
	"FDA5A5", // hellrosa
	"BDD7EE", // pastellblau
	"E2F0D9", // hellgrün
}

const shiftMutedFill = "D9D9D9" // Urlaub, Krank, Feiertag, …

// hairBorder entspricht dem Haarlinien-Rahmen der Vorlage (Style-Index 7).
var hairBorder = []excelize.Border{
	{Type: "left", Color: "000000", Style: 7},
	{Type: "right", Color: "000000", Style: 7},
	{Type: "top", Color: "000000", Style: 7},
	{Type: "bottom", Color: "000000", Style: 7},
}

var documentTitleBottomBorder = []excelize.Border{
	{Type: "bottom", Color: "000000", Style: 7},
}

type weekStyles struct {
	documentTitle   int
	employeeName    int
	shiftNormal     int
	shiftMuted      int
	holidayHorizontal int // Feiertagsname mit Zeilenumbruch (wenn genug Zeilenhöhe)
	holidayVertical   int // Feiertagsname senkrecht (90°), wenn horizontal nicht passt
	notes           int
	weekTitle       [5]int // KW-Zeile, 12 pt
	weekSection     [5]int // Gruppe / Teamsitzung, 10 pt
	weekDate        [5]int // Datum-Zeile, 11 pt
}

func newWeekStyles(f *excelize.File) (weekStyles, error) {
	var ws weekStyles

	titleID, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14, Family: fontFamilyArial},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    documentTitleBottomBorder,
	})
	if err != nil {
		return weekStyles{}, fmt.Errorf("document title style: %w", err)
	}
	ws.documentTitle = titleID

	empID, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 10, Family: fontFamilyArial},
		Alignment: &excelize.Alignment{
			Horizontal: "left", Vertical: "center", WrapText: true,
		},
		Border: hairBorder,
	})
	if err != nil {
		return weekStyles{}, fmt.Errorf("employee name style: %w", err)
	}
	ws.employeeName = empID

	shiftID, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 9, Family: fontFamilyArial},
		Alignment: &excelize.Alignment{
			Horizontal: "center", Vertical: "center", WrapText: true,
		},
		Border: hairBorder,
	})
	if err != nil {
		return weekStyles{}, fmt.Errorf("shift cell style: %w", err)
	}
	ws.shiftNormal = shiftID

	shiftMutedID, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{shiftMutedFill}, Pattern: 1},
		Font: &excelize.Font{Bold: true, Size: 9, Family: fontFamilyArial},
		Alignment: &excelize.Alignment{
			Horizontal: "center", Vertical: "center", WrapText: true,
		},
		Border: hairBorder,
	})
	if err != nil {
		return weekStyles{}, fmt.Errorf("shift muted style: %w", err)
	}
	ws.shiftMuted = shiftMutedID

	holidayHorizID, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{shiftMutedFill}, Pattern: 1},
		Font: &excelize.Font{Bold: true, Size: 9, Family: fontFamilyArial},
		Alignment: &excelize.Alignment{
			Horizontal: "center", Vertical: "center", WrapText: true,
		},
		Border: hairBorder,
	})
	if err != nil {
		return weekStyles{}, fmt.Errorf("holiday horizontal style: %w", err)
	}
	ws.holidayHorizontal = holidayHorizID

	holidayVertID, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{shiftMutedFill}, Pattern: 1},
		Font: &excelize.Font{Bold: true, Size: 9, Family: fontFamilyArial},
		Alignment: &excelize.Alignment{
			Horizontal:   "center",
			Vertical:     "center",
			WrapText:     false,
			TextRotation: 90,
		},
		Border: hairBorder,
	})
	if err != nil {
		return weekStyles{}, fmt.Errorf("holiday vertical style: %w", err)
	}
	ws.holidayVertical = holidayVertID

	notesID, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 8, Family: fontFamilyArial},
		Alignment: &excelize.Alignment{
			WrapText:   true,
			Vertical:   "top",
			Horizontal: "left",
		},
	})
	if err != nil {
		return weekStyles{}, fmt.Errorf("notes style: %w", err)
	}
	ws.notes = notesID

	for i, color := range weekFillColors {
		fill := excelize.Fill{Type: "pattern", Color: []string{color}, Pattern: 1}
		align := &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}

		titleID, err := f.NewStyle(&excelize.Style{
			Fill: fill,
			Font: &excelize.Font{Bold: true, Size: 12, Family: fontFamilyArial},
			Alignment: align,
		})
		if err != nil {
			return weekStyles{}, fmt.Errorf("week title style %d: %w", i, err)
		}
		ws.weekTitle[i] = titleID

		sectionID, err := f.NewStyle(&excelize.Style{
			Fill: fill,
			Font: &excelize.Font{Bold: true, Size: 10, Family: fontFamilyArial},
			Alignment: align,
		})
		if err != nil {
			return weekStyles{}, fmt.Errorf("week section style %d: %w", i, err)
		}
		ws.weekSection[i] = sectionID

		dateID, err := f.NewStyle(&excelize.Style{
			Fill: fill,
			Font: &excelize.Font{Bold: true, Size: 11, Family: fontFamilyArial},
			Alignment: align,
		})
		if err != nil {
			return weekStyles{}, fmt.Errorf("week date style %d: %w", i, err)
		}
		ws.weekDate[i] = dateID
	}

	return ws, nil
}

func (ws weekStyles) weekTitleStyleID(weekOrdinal int) int {
	return ws.weekTitle[weekOrdinal%5]
}

func (ws weekStyles) weekSectionStyleID(weekOrdinal int) int {
	return ws.weekSection[weekOrdinal%5]
}

func (ws weekStyles) weekDateStyleID(weekOrdinal int) int {
	return ws.weekDate[weekOrdinal%5]
}

// holidayFewEmployeeRows: darunter gilt ein Abschnitt als „wenige Mitarbeitende“.
const holidayFewEmployeeRows = 4

func (ws weekStyles) holidayStyleID(holidayName string, employeeRows int) int {
	if holidayUseHorizontalException(holidayName, employeeRows) {
		return ws.holidayHorizontal
	}
	return ws.holidayVertical
}

// holidayUseHorizontalException ist true nur in Ausnahmefällen: wenige Zeilen im Abschnitt
// und der Feiertagsname passt senkrecht (90°) nicht in die verfügbare Höhe.
func holidayUseHorizontalException(name string, employeeRows int) bool {
	if employeeRows >= holidayFewEmployeeRows {
		return false
	}
	runes := len([]rune(strings.TrimSpace(name)))
	if runes < 10 {
		return false
	}
	available := float64(employeeRows) * defaultRowHeightPt
	const charHeightPt = 6.5 // geschätzter Höhenbedarf pro Zeichen bei TextRotation 90
	return float64(runes)*charHeightPt > available
}

func (ws weekStyles) shiftStyleID(cellValue string) int {
	if usesMutedShiftFill(cellValue) {
		return ws.shiftMuted
	}
	return ws.shiftNormal
}

// usesMutedShiftFill entspricht grau hinterlegten Zellen in der Vorlage.
func usesMutedShiftFill(value string) bool {
	switch value {
	case "U", "AT", "xxx", "Schule":
		return true
	default:
		return false
	}
}

func applyRowStyle(f *excelize.File, sheet string, row, colFrom, colTo, styleID int) error {
	topLeft, err := excelize.CoordinatesToCellName(colFrom, row)
	if err != nil {
		return err
	}
	bottomRight, err := excelize.CoordinatesToCellName(colTo, row)
	if err != nil {
		return err
	}
	return f.SetCellStyle(sheet, topLeft, bottomRight, styleID)
}

func applyEmployeeNameStyle(f *excelize.File, sheet string, row int, styleID int) error {
	return applyRowStyle(f, sheet, row, 2, 2, styleID)
}

func applyShiftCellStyle(f *excelize.File, sheet string, row, col int, value string, ws weekStyles) error {
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return err
	}
	return f.SetCellStyle(sheet, cell, cell, ws.shiftStyleID(value))
}

// setMergedHeaderRow schreibt eine über B:G zusammengeführte Kopfzeile (wie Vorlage).
func setMergedHeaderRow(f *excelize.File, sheet string, row int, text string, styleID int) error {
	topLeft, err := excelize.CoordinatesToCellName(2, row)
	if err != nil {
		return err
	}
	bottomRight, err := excelize.CoordinatesToCellName(7, row)
	if err != nil {
		return err
	}
	if err := f.MergeCell(sheet, topLeft, bottomRight); err != nil {
		return err
	}
	if err := f.SetCellValue(sheet, topLeft, text); err != nil {
		return err
	}
	if err := f.SetCellStyle(sheet, topLeft, bottomRight, styleID); err != nil {
		return err
	}
	return setNormalRowHeight(f, sheet, row)
}
