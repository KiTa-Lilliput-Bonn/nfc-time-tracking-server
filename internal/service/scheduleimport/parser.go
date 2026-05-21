package scheduleimport

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

var (
	weekHeaderRE = regexp.MustCompile(
		`^\s*(\d{1,2})\.(\d{1,2})\.\s*-\s*(\d{1,2})\.(\d{1,2})\.\s*(\d{4})\s*$`,
	)
	shortDateRE = regexp.MustCompile(`^\s*(\d{1,2})\.(\d{1,2})\.(\d{4})?\s*$`)
)

// ParsedSheet enthält alle aus der Vorlage gelesenen Daten.
type ParsedSheet struct {
	Weeks []ParsedWeek
}

// ParsedWeek ein erkanntes Wochenfenster (Mo–Fr).
type ParsedWeek struct {
	ISOYear int
	ISOWk   int

	Notes string

	// Dates hat genau 5 Einträge Mo–Fr (YYYY-MM-DD).
	Dates [5]string

	SkipDay [5]bool // Feiertagsspalten / überspringen

	Rows []ParsedEmployeeRow

	// TeamMondaySections: je „Datum“-Block eine Sektion (Zeile darüber = Teamsitzungstext).
	TeamMondaySections []ParsedTeamMondaySection

	TeamMeetingParseWarnings []string
}

// ParsedEmployeeRow eine Datenzeile unterhalb einer Datum-Zeile.
type ParsedEmployeeRow struct {
	RawName string
	Cells   [5]string
}

// ParseXLSX liest die Arbeitsmappe (ein relevantes Blatt).
func ParseXLSX(file []byte) (*ParsedSheet, error) {
	f, err := excelize.OpenReader(bytes.NewReader(file))
	if err != nil {
		return nil, fmt.Errorf("excel öffnen: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, fmt.Errorf("kein Arbeitsblatt")
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}

	out := &ParsedSheet{}
	r := 0
	for r < len(rows) {
		b := trimCell(cell(rows, r, 1))
		if weekHeaderRE.MatchString(b) {
			submatch := weekHeaderRE.FindStringSubmatch(b)
			if len(submatch) < 6 {
				r++
				continue
			}
			year, _ := strconv.Atoi(submatch[5])
			notes, err := collectNotesRichHTML(f, sheet, r, len(rows))
			if err != nil {
				return nil, err
			}
			wk, err := parseWeekBlock(rows, &r, year, notes)
			if err != nil {
				return nil, err
			}
			if wk != nil {
				out.Weeks = append(out.Weeks, *wk)
			}
			continue
		}
		r++
	}

	return out, nil
}

func cell(rows [][]string, row, col int) string {
	if row < 0 || row >= len(rows) {
		return ""
	}
	if col < 0 || col >= len(rows[row]) {
		return ""
	}
	return rows[row][col]
}

func parseWeekBlock(rows [][]string, rPtr *int, headerYear int, notes string) (*ParsedWeek, error) {
	r := *rPtr
	r++ // erste Zeile nach Überschrift ist schon verarbeitet — wir starten unter der Header-Zeile

	var dates [5]string
	var skip [5]bool
	haveDates := false

	var empRows []ParsedEmployeeRow
	var teamSections []ParsedTeamMondaySection
	var teamWarns []string
	curTeamSec := -1

	for r < len(rows) {
		b := trimCell(cell(rows, r, 1))

		if weekHeaderRE.MatchString(b) {
			*rPtr = r
			break
		}

		if strings.EqualFold(b, "Datum") {
			prev := ""
			if r > 0 {
				prev = trimCell(cell(rows, r-1, 1))
			}
			line := ParseTeamMeetingLine(prev)
			for _, w := range line.Warnings {
				teamWarns = append(teamWarns, fmt.Sprintf("KW-Zeile über „Datum“ (%q): %s", prev, w))
			}
			d, sk, err := parseDatumRow(rows, r, headerYear)
			if err != nil {
				return nil, err
			}
			dates = d
			skip = sk
			haveDates = true
			teamSections = append(teamSections, ParsedTeamMondaySection{Line: line})
			curTeamSec = len(teamSections) - 1
			r++
			continue
		}

		if b == "" || isSectionHeaderRow(b) {
			r++
			continue
		}

		if !haveDates {
			r++
			continue
		}

		if isLikelyEmployeeRow(b) {
			var cells [5]string
			for i := 0; i < 5; i++ {
				cells[i] = trimCell(cell(rows, r, 2+i))
			}
			empRows = append(empRows, ParsedEmployeeRow{RawName: b, Cells: cells})
			if curTeamSec >= 0 && curTeamSec < len(teamSections) {
				ts := &teamSections[curTeamSec]
				ts.EmployeeRawNames = append(ts.EmployeeRawNames, b)
			}
		}
		r++
	}

	*rPtr = r

	if !haveDates {
		return nil, nil
	}
	var isoY, isoW int
	found := false
	for _, d := range dates {
		if d == "" {
			continue
		}
		t0, err := time.ParseInLocation("2006-01-02", d, time.Local)
		if err != nil {
			return nil, fmt.Errorf("ungültiges Datum %q", d)
		}
		isoY, isoW = t0.ISOWeek()
		found = true
		break
	}
	if !found {
		return nil, nil
	}

	skip = detectHolidayColumnsFromEmployees(empRows, skip)

	pw := &ParsedWeek{
		ISOYear:                  isoY,
		ISOWk:                    isoW,
		Notes:                    notes,
		Dates:                    dates,
		SkipDay:                  skip,
		Rows:                     empRows,
		TeamMondaySections:       teamSections,
		TeamMeetingParseWarnings: teamWarns,
	}
	return pw, nil
}

func isSectionHeaderRow(b string) bool {
	low := strings.ToLower(b)
	if strings.HasPrefix(low, "gt") || strings.HasPrefix(low, "kt") {
		return true
	}
	if strings.Contains(low, "kein team") {
		return true
	}
	return false
}

func isLikelyEmployeeRow(b string) bool {
	if strings.EqualFold(b, "Datum") {
		return false
	}
	if weekHeaderRE.MatchString(b) {
		return false
	}
	return true
}

// detectHolidayColumnsFromEmployees markiert Spalten, in denen der senkrechte Feiertagstext steht.
func detectHolidayColumnsFromEmployees(empRows []ParsedEmployeeRow, skip [5]bool) [5]bool {
	for col := 0; col < 5; col++ {
		if skip[col] {
			continue
		}
		for _, row := range empRows {
			pc, _ := ParseCellContent(row.Cells[col])
			if pc.Kind == CellSkipHoliday {
				skip[col] = true
				break
			}
		}
	}
	return skip
}

func parseDatumRow(rows [][]string, rowIdx int, defaultYear int) ([5]string, [5]bool, error) {
	var dates [5]string
	var skip [5]bool

	for col := 0; col < 5; col++ {
		raw := trimCell(cell(rows, rowIdx, 2+col))
		d, sk, err := parseGermanDayCell(raw, defaultYear)
		if err != nil {
			return dates, skip, err
		}
		dates[col] = d
		skip[col] = sk || d == ""
	}
	return dates, skip, nil
}

func parseGermanDayCell(raw string, defaultYear int) (dateISO string, skipBecauseHolidayHint bool, err error) {
	if raw == "" {
		return "", false, nil
	}
	low := strings.ToLower(raw)
	if strings.Contains(low, "karfreitag") || (strings.Contains(low, "feiertag") && !timeRangeRE.MatchString(raw)) {
		return "", true, nil
	}

	m := shortDateRE.FindStringSubmatch(raw)
	if m == nil {
		return "", false, fmt.Errorf("Datum-Zelle nicht erkannt: %q", raw)
	}
	d1, _ := strconv.Atoi(m[1])
	d2, _ := strconv.Atoi(m[2])
	y := defaultYear
	if m[3] != "" {
		y, _ = strconv.Atoi(m[3])
	}
	t := time.Date(y, time.Month(d2), d1, 12, 0, 0, 0, time.Local)
	return t.Format("2006-01-02"), false, nil
}

// Note: German date format is DD.MM — m[1] is day, m[2] is month in regex I used:
// ^\s*(\d{1,2})\.(\d{1,2})\. — first group day, second month. Good.
