package scheduleimport

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	timeRangeRE = regexp.MustCompile(`(?i)(\d{1,2})[.:](\d{2})\s*-\s*(\d{1,2})[.:](\d{2})`)
	// Tippfehler: Punkt statt Bindestrich zwischen Beginn- und Endzeit (z. B. "9:00.17:00" statt "9:00-17:00").
	timeRangeDotBetweenRE = regexp.MustCompile(`(?i)(\d{1,2})[.:](\d{2})\s*\.\s*(\d{1,2})[.:](\d{2})`)
)

// ParseMeta liefert Hinweise zur Zellanalyse (z. B. erkannte Tippfehler-Korrektur).
type ParseMeta struct {
	// DotInsteadOfHyphenBetweenTimes: zwischen den beiden Uhrzeiten stand "." statt "-".
	DotInsteadOfHyphenBetweenTimes bool
}

// CellKind klassifiziert den Zellinhalt.
type CellKind int

const (
	CellEmpty CellKind = iota
	CellWorkTimes
	CellVacation
	CellCompensationDay
	CellOtherAbsence // Schule / Seminar
	CellFreeDay      // xxx — keine Schicht, keine Abwesenheit
	CellSkipHoliday  // expliziter Feiertagstext in der Zelle
)

// ParsedCell ist das Ergebnis der Zellanalyse.
type ParsedCell struct {
	Kind              CellKind
	ShiftStart        string
	ShiftEnd          string
	OtherAbsenceLabel string
}

func trimCell(s string) string {
	return strings.TrimSpace(s)
}

// ParseCellContent wertet eine Excel-Zelle aus (Raw-String).
func ParseCellContent(raw string) (ParsedCell, ParseMeta) {
	s := trimCell(raw)
	if s == "" {
		return ParsedCell{Kind: CellEmpty}, ParseMeta{}
	}
	low := strings.ToLower(s)

	if strings.Contains(low, "karfreitag") {
		return ParsedCell{Kind: CellSkipHoliday}, ParseMeta{}
	}
	if strings.Contains(low, "feiertag") && !timeRangeRE.MatchString(s) && !timeRangeDotBetweenRE.MatchString(s) {
		return ParsedCell{Kind: CellSkipHoliday}, ParseMeta{}
	}

	if matched, _ := regexp.MatchString(`(?i)^x{3}$`, s); matched {
		return ParsedCell{Kind: CellFreeDay}, ParseMeta{}
	}
	if matched, _ := regexp.MatchString(`(?i)^u$`, s); matched {
		return ParsedCell{Kind: CellVacation}, ParseMeta{}
	}
	if matched, _ := regexp.MatchString(`(?i)^at$`, s); matched {
		return ParsedCell{Kind: CellCompensationDay}, ParseMeta{}
	}
	if strings.Contains(low, "schule") {
		return ParsedCell{Kind: CellOtherAbsence, OtherAbsenceLabel: "Schule"}, ParseMeta{}
	}
	if strings.Contains(low, "seminar") {
		return ParsedCell{Kind: CellOtherAbsence, OtherAbsenceLabel: "Seminar"}, ParseMeta{}
	}

	m := timeRangeRE.FindStringSubmatch(s)
	if m != nil {
		return parsedCellFromTimeSubmatch(m), ParseMeta{}
	}
	m = timeRangeDotBetweenRE.FindStringSubmatch(s)
	if m != nil {
		return parsedCellFromTimeSubmatch(m), ParseMeta{DotInsteadOfHyphenBetweenTimes: true}
	}

	return ParsedCell{Kind: CellEmpty}, ParseMeta{}
}

// ParseClockRange parses a substring like "15.30-17.00" into normalized HH:MM — HH:MM (same rules as shift cells).
func ParseClockRange(s string) (start, end string, ok bool) {
	s = trimCell(s)
	if s == "" {
		return "", "", false
	}
	m := timeRangeRE.FindStringSubmatch(s)
	if m != nil {
		pc := parsedCellFromTimeSubmatch(m)
		return pc.ShiftStart, pc.ShiftEnd, true
	}
	m = timeRangeDotBetweenRE.FindStringSubmatch(s)
	if m != nil {
		pc := parsedCellFromTimeSubmatch(m)
		return pc.ShiftStart, pc.ShiftEnd, true
	}
	return "", "", false
}

func parsedCellFromTimeSubmatch(m []string) ParsedCell {
	h1, _ := strconv.Atoi(m[1])
	min1, _ := strconv.Atoi(m[2])
	h2, _ := strconv.Atoi(m[3])
	min2, _ := strconv.Atoi(m[4])
	return ParsedCell{
		Kind:       CellWorkTimes,
		ShiftStart: normalizeTime(h1, min1),
		ShiftEnd:   normalizeTime(h2, min2),
	}
}

func normalizeTime(h, m int) string {
	if h < 0 {
		h = 0
	}
	if h > 23 {
		h = 23
	}
	if m < 0 {
		m = 0
	}
	if m > 59 {
		m = 59
	}
	return formatHMParts(h, m)
}

func formatHMParts(h, m int) string {
	return string([]byte{
		byte('0' + (h/10)%10),
		byte('0' + h%10),
		':',
		byte('0' + (m/10)%10),
		byte('0' + m%10),
	})
}
