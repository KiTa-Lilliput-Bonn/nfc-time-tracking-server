package scheduleexport

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"nfc-time-tracking-server/internal/model"
)

type employeeSection struct {
	title     string
	employees []*model.User
}

// BuildXLSX erzeugt eine .xlsx-Datei im Dienstplan-Import-Layout für den KW-Zeitraum.
func BuildXLSX(ctx context.Context, deps Deps, fromYear, fromWeek, toYear, toWeek int) ([]byte, error) {
	weeks, err := iterateISOWeeks(fromYear, fromWeek, toYear, toWeek)
	if err != nil {
		return nil, err
	}

	users, err := deps.Users.List(ctx, true)
	if err != nil {
		return nil, err
	}
	employees := filterScheduleEmployees(users)

	groups, err := deps.Groups.List(ctx)
	if err != nil {
		return nil, err
	}
	sections := buildSections(employees, groups)

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, fmt.Errorf("kein Arbeitsblatt")
	}

	styles, err := newWeekStyles(f)
	if err != nil {
		return nil, err
	}
	var colW columnWidths

	firstMon, err := mondayOfISOWeek(weeks[0].Year, weeks[0].Week)
	if err != nil {
		return nil, err
	}
	_, _, lastFri, err := weekDates(weeks[len(weeks)-1].Year, weeks[len(weeks)-1].Week)
	if err != nil {
		return nil, err
	}

	row := 1
	docTitle := formatDocumentTitle(firstMon, lastFri)
	if err := writeDocumentTitle(f, sheet, row, docTitle, styles.documentTitle); err != nil {
		return nil, err
	}
	row++ // Leerzeile unter Dokumentüberschrift
	row++

	for weekOrdinal, wk := range weeks {
		row, err = writeWeek(ctx, f, sheet, deps, styles, &colW, sections, employees, wk.Year, wk.Week, weekOrdinal, row)
		if err != nil {
			return nil, err
		}
		if weekOrdinal < len(weeks)-1 {
			row++ // Leerzeile zwischen Wochenblöcken
		}
	}

	if err := applySheetLayout(f, sheet, &colW); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func filterScheduleEmployees(users []model.User) []*model.User {
	var out []*model.User
	for i := range users {
		u := &users[i]
		if !u.Active || u.Role == model.RoleSuperadmin {
			continue
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].DisplayName < out[j].DisplayName
	})
	return out
}

func buildSections(employees []*model.User, groups []model.Group) []employeeSection {
	sortedGroups := append([]model.Group(nil), groups...)
	sort.Slice(sortedGroups, func(i, j int) bool {
		if sortedGroups[i].SortOrder != sortedGroups[j].SortOrder {
			return sortedGroups[i].SortOrder < sortedGroups[j].SortOrder
		}
		return sortedGroups[i].ID < sortedGroups[j].ID
	})

	known := map[int]struct{}{}
	for _, g := range sortedGroups {
		known[g.ID] = struct{}{}
	}

	var sections []employeeSection
	for _, g := range sortedGroups {
		var emps []*model.User
		for _, e := range employees {
			if e.GroupID != nil && *e.GroupID == g.ID {
				emps = append(emps, e)
			}
		}
		sort.Slice(emps, func(i, j int) bool {
			return emps[i].DisplayName < emps[j].DisplayName
		})
		if len(emps) > 0 {
			sections = append(sections, employeeSection{title: g.Name, employees: emps})
		}
	}

	var orphan []*model.User
	for _, e := range employees {
		if e.GroupID == nil {
			orphan = append(orphan, e)
			continue
		}
		if _, ok := known[*e.GroupID]; !ok {
			orphan = append(orphan, e)
		}
	}
	sort.Slice(orphan, func(i, j int) bool {
		return orphan[i].DisplayName < orphan[j].DisplayName
	})
	if len(orphan) > 0 {
		sections = append(sections, employeeSection{title: "Ohne Gruppe", employees: orphan})
	}
	if len(sections) == 0 && len(employees) > 0 {
		sections = append(sections, employeeSection{title: "", employees: employees})
	}
	return sections
}

func writeWeek(
	ctx context.Context,
	f *excelize.File,
	sheet string,
	deps Deps,
	styles weekStyles,
	colW *columnWidths,
	sections []employeeSection,
	allEmployees []*model.User,
	isoYear, isoWeek, weekOrdinal, startRow int,
) (int, error) {
	monday, dates, friday, err := weekDates(isoYear, isoWeek)
	if err != nil {
		return startRow, err
	}

	cellData, meetings, notes, err := loadWeekData(ctx, deps, isoYear, isoWeek, dates, allEmployees)
	if err != nil {
		return startRow, err
	}

	weekTitleStyle := styles.weekTitleStyleID(weekOrdinal)
	weekSectionStyle := styles.weekSectionStyleID(weekOrdinal)
	weekDateStyle := styles.weekDateStyleID(weekOrdinal)
	row := startRow

	weekHeaderRow := row
	weekHeaderText := formatWeekHeader(monday, friday)
	if err := setMergedHeaderRow(f, sheet, row, weekHeaderText, weekTitleStyle); err != nil {
		return row, err
	}
	row++

	if len(sections) == 0 {
		sections = []employeeSection{{title: "", employees: allEmployees}}
	}

	for secIdx, sec := range sections {
		if line := formatTeamMeetingLine(meetings, secIdx); line != "" {
			if err := setMergedHeaderRow(f, sheet, row, line, weekSectionStyle); err != nil {
				return row, err
			}
			row++
		}

		if err := setCell(f, sheet, 2, row, "Datum"); err != nil {
			return row, err
		}
		for i := 0; i < 5; i++ {
			val := datumCellText(dates[i], cellData.holiday[i], cellData.skipDay[i])
			if err := setCell(f, sheet, 3+i, row, val); err != nil {
				return row, err
			}
			colW.observeShift(3+i, val)
		}
		if err := applyRowStyle(f, sheet, row, 2, 7, weekDateStyle); err != nil {
			return row, err
		}
		if err := setNormalRowHeight(f, sheet, row); err != nil {
			return row, err
		}
		row++

		employeeStartRow := row
		for _, emp := range sec.employees {
			if err := setCell(f, sheet, 2, row, emp.DisplayName); err != nil {
				return row, err
			}
			colW.observeEmployeeDisplayName(emp.DisplayName)
			for i := 0; i < 5; i++ {
				if cellData.skipDay[i] {
					continue
				}
				val := cellData.cellValue(emp, i, dates[i])
				if err := setCell(f, sheet, 3+i, row, val); err != nil {
					return row, err
				}
				colW.observeShift(3+i, val)
				if err := applyShiftCellStyle(f, sheet, row, 3+i, val, styles); err != nil {
					return row, err
				}
			}
			if err := applyEmployeeNameStyle(f, sheet, row, styles.employeeName); err != nil {
				return row, err
			}
			if err := setNormalRowHeight(f, sheet, row); err != nil {
				return row, err
			}
			row++
		}
		employeeEndRow := row - 1
		if employeeEndRow >= employeeStartRow {
			for i := 0; i < 5; i++ {
				if !cellData.skipDay[i] || cellData.holiday[i] == "" {
					continue
				}
				empRows := employeeEndRow - employeeStartRow + 1
				holidayStyle := styles.holidayStyleID(cellData.holiday[i], empRows)
				if err := applyHolidayColumnMerge(f, sheet, 3+i, employeeStartRow, employeeEndRow, cellData.holiday[i], holidayStyle); err != nil {
					return row, err
				}
			}
		}
	}

	notesEndRow := row - 1
	if notesEndRow < weekHeaderRow {
		notesEndRow = weekHeaderRow
	}
	if err := applyNotesMerge(f, sheet, weekHeaderRow, notesEndRow, notes, styles.notes); err != nil {
		return row, err
	}

	return row, nil
}

func loadWeekData(
	ctx context.Context,
	deps Deps,
	isoYear, isoWeek int,
	dates [5]time.Time,
	employees []*model.User,
) (*weekCellData, []model.TeamMeeting, string, error) {
	data := &weekCellData{
		schedules: map[int]map[string]*model.Schedule{},
		absences:  map[int]map[string]*model.Absence{},
		fnwByUser: map[int][]model.FixedNonWorkWeekdays{},
	}
	for _, e := range employees {
		data.schedules[e.ID] = map[string]*model.Schedule{}
		data.absences[e.ID] = map[string]*model.Absence{}
		if deps.FixedNonWorkWeekdays != nil {
			if rows, err := deps.FixedNonWorkWeekdays.ListByUser(ctx, e.ID); err == nil {
				data.fnwByUser[e.ID] = rows
			}
		}
	}

	schedules, err := deps.Schedules.ListByWeek(ctx, isoYear, isoWeek)
	if err != nil {
		return nil, nil, "", err
	}
	for i := range schedules {
		sch := &schedules[i]
		dateISO := strings.TrimSpace(sch.ScheduleDate)
		if len(dateISO) > 10 {
			dateISO = dateISO[:10]
		}
		if data.schedules[sch.UserID] == nil {
			data.schedules[sch.UserID] = map[string]*model.Schedule{}
		}
		data.schedules[sch.UserID][dateISO] = sch
	}

	from := dates[0].Format("2006-01-02")
	to := dates[4].Format("2006-01-02")
	absences, err := deps.Absences.ListByDateRangeTypes(ctx, from, to, []model.AbsenceType{
		model.AbsenceVacation,
		model.AbsenceCompensationDay,
		model.AbsenceSick,
		model.AbsenceOther,
	})
	if err != nil {
		return nil, nil, "", err
	}
	for i := range absences {
		a := &absences[i]
		dateISO := strings.TrimSpace(a.AbsenceDate)
		if len(dateISO) > 10 {
			dateISO = dateISO[:10]
		}
		if data.absences[a.UserID] == nil {
			data.absences[a.UserID] = map[string]*model.Absence{}
		}
		data.absences[a.UserID][dateISO] = a
	}

	for i := 0; i < 5; i++ {
		dISO := dates[i].Format("2006-01-02")
		if deps.Holidays != nil {
			hol, err := deps.Holidays.GetForDate(ctx, dISO)
			if err != nil {
				return nil, nil, "", err
			}
			if hol != nil {
				data.skipDay[i] = true
				data.holiday[i] = hol.Name
			}
		}
	}

	var meetings []model.TeamMeeting
	if deps.TeamMeetings != nil {
		meetings, err = deps.TeamMeetings.ListByWeek(ctx, isoYear, isoWeek)
		if err != nil {
			return nil, nil, "", err
		}
	}

	notes, err := deps.Schedules.GetWeekNotes(ctx, isoYear, isoWeek)
	if err != nil {
		return nil, nil, "", err
	}
	notes = mergeOtherMeetingsIntoNotesHTML(notes, meetings)

	return data, meetings, notes, nil
}

func formatTeamMeetingLine(meetings []model.TeamMeeting, sectionIndex int) string {
	var parts []string
	for _, m := range meetings {
		if m.Kind != model.TeamMeetingKindKT || m.SectionIndex != sectionIndex {
			continue
		}
		parts = append(parts, fmt.Sprintf("KT %s - %s",
			formatTeamMeetingClock(m.TimeStart), formatTeamMeetingClock(m.TimeEnd)))
	}
	seenGT := map[string]struct{}{}
	for _, m := range meetings {
		if m.Kind != model.TeamMeetingKindGT {
			continue
		}
		key := m.TimeStart + "|" + m.TimeEnd
		if _, ok := seenGT[key]; ok {
			continue
		}
		seenGT[key] = struct{}{}
		parts = append(parts, fmt.Sprintf("GT %s - %s",
			formatTeamMeetingClock(m.TimeStart), formatTeamMeetingClock(m.TimeEnd)))
	}
	return strings.Join(parts, " + ")
}

func setCell(f *excelize.File, sheet string, col, row int, value string) error {
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return err
	}
	return f.SetCellValue(sheet, cell, value)
}

// ExportDefaults liefert Start- und End-KW für den Export-Dialog.
func ExportDefaults(ctx context.Context, schedules storeScheduleReader) (startYear, startWeek, endYear, endWeek int, err error) {
	now := time.Now().In(time.Local)
	startYear, startWeek = now.ISOWeek()
	endYear, endWeek = startYear, startWeek
	if schedules != nil {
		if y, w, ok, e := schedules.LastISOWeekWithShift(ctx); e != nil {
			return 0, 0, 0, 0, e
		} else if ok {
			endYear, endWeek = y, w
		}
	}
	return startYear, startWeek, endYear, endWeek, nil
}

type storeScheduleReader interface {
	LastISOWeekWithShift(ctx context.Context) (year, week int, ok bool, err error)
}
