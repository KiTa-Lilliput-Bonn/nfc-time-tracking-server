package scheduleimport

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

// Apply wendet geparste Wochen auf die Datenbank an.
// todayLocal ist YYYY-MM-DD (Kalendertag in time.Local).
// scope ImportScopeFuture: nur date >= today; ImportScopePastOnly: nur date < today.
func Apply(ctx context.Context, deps Deps, parsed *ParsedSheet, createdBy int, todayLocal string, scope ImportScope) (*Report, error) {
	if parsed == nil {
		return &Report{}, nil
	}

	index, fnwByUID, teamMeetingOptOut, ambWarnings, err := buildEmployeeNameIndex(ctx, deps.Users, deps.FixedNonWorkWeekdays)
	if err != nil {
		return nil, err
	}

	rep := &Report{
		Warnings: ambWarnings,
	}

	seenFixedFreeShiftWarn := map[string]struct{}{}
	weekOccurrences := countISOWeekOccurrences(parsed.Weeks)
	skippedCellsByWeek := map[isoWeekKey]int{}

	for _, w := range parsed.Weeks {
		wkKey := isoWeekKey{year: w.ISOYear, week: w.ISOWk}
		skippedThisBlock := 0

		wr := WeekReport{
			ISOYear: w.ISOYear,
			ISOWk:   w.ISOWk,
		}

		skip := mergeHolidaySkip(ctx, deps.Holidays, w)

		// Pro Feiertagsspalte höchstens 1 — nicht Mitarbeiter × Spalte (sonst 3 Tage × 2 MA = 6).
		var skippedHolidayColWithContent [5]bool

		if applyNotes, countPastNotes := weekNotesInImportScope(w, todayLocal, scope); applyNotes || countPastNotes {
			if countPastNotes {
				rep.PastWeekNotesSkipped++
				rep.Warnings = append(rep.Warnings, fmt.Sprintf(
					"KW %d/%d: Wochennotiz nicht gespeichert (gesamte Woche liegt vor %s).",
					w.ISOWk, w.ISOYear, todayLocal))
			} else if applyNotes {
				if err := deps.Schedules.PutWeekNotes(ctx, w.ISOYear, w.ISOWk, w.Notes); err != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("KW %d/%d Wochennotiz: %v", w.ISOWk, w.ISOYear, err))
				} else {
					wr.NotesWritten = true
					rep.WeekNotesUpdated++
				}
			}
		}

		seenUnknown := map[string]struct{}{}

		for _, row := range w.Rows {
			uid, ok := index[normalizeName(row.RawName)]
			if !ok {
				if _, du := seenUnknown[row.RawName]; !du {
					seenUnknown[row.RawName] = struct{}{}
					rep.UnknownNames = append(rep.UnknownNames, strings.TrimSpace(row.RawName))
				}
				continue
			}

			for col := 0; col < 5; col++ {
				if skip[col] && strings.TrimSpace(row.Cells[col]) != "" {
					skippedHolidayColWithContent[col] = true
				}
			}

			for col := 0; col < 5; col++ {
				if skip[col] {
					continue
				}

				date := w.Dates[col]
				if date == "" {
					continue
				}
				if !dateInImportScope(date, todayLocal, scope) {
					if scope == ImportScopeFuture && date < todayLocal && strings.TrimSpace(row.Cells[col]) != "" {
						rep.PastCellsSkipped++
						wr.PastCellsSkipped++
					}
					continue
				}

				raw := row.Cells[col]
				pc, parseMeta := ParseCellContent(raw)

				ref := cellRef{date: date, userLabel: row.RawName, uid: uid}

				if parseMeta.DotInsteadOfHyphenBetweenTimes {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf(
						"Zwischen den Zeiten stand \".\" statt \"-\" (%s, %s): %q — als Schichtzeit übernommen.",
						row.RawName, date, strings.TrimSpace(raw)))
				}

				if strings.TrimSpace(raw) != "" && pc.Kind == CellEmpty {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("Unbekannter Zellinhalt (%s, %s): %q", row.RawName, date, raw))
				}

				switch pc.Kind {
				case CellEmpty:
					// leere Zelle: keine Änderung
				case CellSkipHoliday:
					// sollte durch Spalten-Skip abgedeckt sein; sicherheitshalber ignorieren
				case CellFreeDay:
					if err := deleteSchedule(ctx, deps.Schedules, ref, rep, &wr); err != nil {
						rep.Errors = append(rep.Errors, err.Error())
					}
					if err := purgeAbsence(ctx, deps, uid, date); err != nil {
						rep.Errors = append(rep.Errors, err.Error())
					}
				case CellVacation:
					if err := deleteSchedule(ctx, deps.Schedules, ref, rep, &wr); err != nil {
						rep.Errors = append(rep.Errors, err.Error())
					}
					if err := replaceWithVacation(ctx, deps, uid, date, createdBy, rep, &wr); err != nil {
						rep.Errors = append(rep.Errors, err.Error())
					}
				case CellCompensationDay:
					if err := deleteSchedule(ctx, deps.Schedules, ref, rep, &wr); err != nil {
						rep.Errors = append(rep.Errors, err.Error())
					}
					if err := replaceWithCompensationDay(ctx, deps, uid, date, createdBy, rep, &wr); err != nil {
						rep.Errors = append(rep.Errors, err.Error())
					}
				case CellOtherAbsence:
					if err := deleteSchedule(ctx, deps.Schedules, ref, rep, &wr); err != nil {
						rep.Errors = append(rep.Errors, err.Error())
					}
					if err := replaceWithOther(ctx, deps, uid, date, createdBy, rep, &wr); err != nil {
						rep.Errors = append(rep.Errors, err.Error())
					}
				case CellWorkTimes:
					if err := purgeAbsence(ctx, deps, uid, date); err != nil {
						rep.Errors = append(rep.Errors, err.Error())
					}
					sch := &model.Schedule{
						UserID: uid, ScheduleDate: date,
						ShiftStart: pc.ShiftStart, ShiftEnd: pc.ShiftEnd,
					}
					if err := deps.Schedules.Set(ctx, sch); err != nil {
						rep.Errors = append(rep.Errors, fmt.Sprintf("%s %s Schicht: %v", row.RawName, date, err))
					} else {
						rep.SchedulesWritten++
						wr.TimesWritten++
						maybeWarnShiftOnFixedFreeDay(
							rep, seenFixedFreeShiftWarn, uid, row.RawName, date,
							fmt.Sprintf("%s-%s", pc.ShiftStart, pc.ShiftEnd), fnwByUID,
						)
					}
				}
			}
		}

		for i := 0; i < 5; i++ {
			if skippedHolidayColWithContent[i] {
				skippedThisBlock++
			}
		}
		skippedCellsByWeek[wkKey] += skippedThisBlock

		applyTeamMeetingsForWeek(ctx, deps, w, index, teamMeetingOptOut, skip, todayLocal, scope, rep, &wr)

		rep.Weeks = append(rep.Weeks, wr)
	}

	rep.AbsencesSkipped = aggregateSkippedHolidayCells(skippedCellsByWeek, weekOccurrences, rep)

	if scope == ImportScopeFuture && rep.PastCellsSkipped > 0 {
		rep.Warnings = append([]string{fmt.Sprintf(
			"Vergangenheit: %d Zellen mit Inhalt wurden nicht geändert (nur ab %s).",
			rep.PastCellsSkipped, todayLocal)}, rep.Warnings...)
	}

	return rep, nil
}

// isoWeekKey gruppiert Import-Zähler je Kalenderwoche (ISO-Jahr kann vom Kalenderjahr abweichen).
type isoWeekKey struct {
	year int
	week int
}

func countISOWeekOccurrences(weeks []ParsedWeek) map[isoWeekKey]int {
	out := map[isoWeekKey]int{}
	for _, w := range weeks {
		k := isoWeekKey{year: w.ISOYear, week: w.ISOWk}
		out[k]++
	}
	return out
}

// aggregateSkippedHolidayCells teilt die „übersprungen“-Zählung (Feiertagstage je Spalte) durch die Anzahl
// gleicher KW-Blöcke im Sheet (dieselbe KW kann durch einen Bereichstrenner mehrfach vorkommen).
func aggregateSkippedHolidayCells(
	perWeek map[isoWeekKey]int,
	occurrences map[isoWeekKey]int,
	rep *Report,
) int {
	total := 0
	seenDupWarn := map[isoWeekKey]struct{}{}
	for k, sum := range perWeek {
		n := occurrences[k]
		if n <= 1 {
			total += sum
			continue
		}
		total += sum / n
		if _, dup := seenDupWarn[k]; dup {
			continue
		}
		seenDupWarn[k] = struct{}{}
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"Kalenderwoche %d/%d ist im Sheet %d-mal vorhanden (z. B. durch einen Bereichstrenner) — für die Meldung „Feiertag übersprungen“ wurde die Zählung durch %d geteilt.",
			k.week, k.year, n, n))
	}
	return total
}

type cellRef struct {
	date      string
	userLabel string
	uid       int
}

// weekFullyPast ist true, wenn alle gesetzten Tagesdaten der Woche vor todayLocal liegen.
func weekFullyPast(w ParsedWeek, todayLocal string) bool {
	has := false
	for _, d := range w.Dates {
		if d == "" {
			continue
		}
		has = true
		if d >= todayLocal {
			return false
		}
	}
	return has
}

func mergeHolidaySkip(ctx context.Context, holidays store.HolidayStore, w ParsedWeek) [5]bool {
	var out [5]bool
	for i := 0; i < 5; i++ {
		out[i] = w.SkipDay[i]
		if out[i] || w.Dates[i] == "" {
			continue
		}
		h, err := holidays.GetForDate(ctx, w.Dates[i])
		if err == nil && h != nil {
			out[i] = true
		}
	}
	return out
}

func normalizeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.ToLower(s)
}

func buildEmployeeNameIndex(ctx context.Context, users store.UserStore, fnw store.FixedNonWorkWeekdaysStore) (map[string]int, map[int][]model.FixedNonWorkWeekdays, map[int]struct{}, []string, error) {
	list, err := users.List(ctx, true)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	buckets := map[string][]int{}
	fnwByUID := map[int][]model.FixedNonWorkWeekdays{}
	teamMeetingOptOut := map[int]struct{}{}
	for _, u := range list {
		if !u.Active || u.Role == model.RoleSuperadmin {
			continue
		}
		if !u.DefaultTeamMeetingParticipant {
			teamMeetingOptOut[u.ID] = struct{}{}
		}
		if fnw != nil {
			if rows, err := fnw.ListByUser(ctx, u.ID); err == nil && len(rows) > 0 {
				fnwByUID[u.ID] = rows
			}
		}
		n := normalizeName(u.DisplayName)
		if n == "" {
			continue
		}
		buckets[n] = append(buckets[n], u.ID)
	}

	index := map[string]int{}
	var warns []string
	for n, ids := range buckets {
		if len(ids) > 1 {
			warns = append(warns, fmt.Sprintf("Mehrdeutiger Anzeigename (Excel-Match ausgeschlossen): %q (%d Treffer)", n, len(ids)))
			continue
		}
		index[n] = ids[0]
	}
	return index, fnwByUID, teamMeetingOptOut, warns, nil
}

// maybeWarnShiftOnFixedFreeDay meldet Schichtimport an einem in den Stammdaten fest freien Wochentag.
func maybeWarnShiftOnFixedFreeDay(
	rep *Report,
	seen map[string]struct{},
	uid int,
	userLabel, date, shiftRange string,
	fnwByUID map[int][]model.FixedNonWorkWeekdays,
) {
	fixed := model.FixedNonWorkWeekdaysForDate(fnwByUID[uid], date)
	if len(fixed) == 0 {
		return
	}
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return
	}
	if !model.IsFixedNonWorkWeekday(t.Weekday(), fixed) {
		return
	}
	key := fmt.Sprintf("%d|%s", uid, date)
	if _, dup := seen[key]; dup {
		return
	}
	seen[key] = struct{}{}
	wd := germanWeekdayName[t.Weekday()]
	rep.Warnings = append(rep.Warnings, fmt.Sprintf(
		"Schicht importiert an festem freien Tag (%s, %s %s): %s — Wochentag ist in den Stammdaten regulär frei.",
		userLabel, wd, t.Format("02.01.2006"), shiftRange,
	))
}

func deleteSchedule(ctx context.Context, schStore store.ScheduleStore, ref cellRef, rep *Report, wr *WeekReport) error {
	sch, err := schStore.GetForUserDate(ctx, ref.uid, ref.date)
	if err != nil || sch == nil {
		return nil
	}
	if err := schStore.Delete(ctx, sch.ID); err != nil {
		return fmt.Errorf("%s %s Dienstplan löschen: %w", ref.userLabel, ref.date, err)
	}
	rep.SchedulesDeleted++
	wr.TimesCleared++
	return nil
}

// purgeAbsence entfernt eine Abwesenheit ohne „ersetzt“-Statistik (Freier Tag / Schicht mit Aufräumen).
func purgeAbsence(ctx context.Context, deps Deps, uid int, date string) error {
	a, err := deps.Absences.GetForUserDate(ctx, uid, date)
	if err != nil {
		return err
	}
	if a == nil {
		return nil
	}
	if a.AbsenceType == model.AbsenceCompensationDay && deps.Claims != nil {
		_ = deps.Claims.ReopenByAbsenceID(ctx, a.ID)
	}
	if err := deps.Absences.Delete(ctx, a.ID); err != nil {
		return fmt.Errorf("Abwesenheit löschen %d/%s: %w", uid, date, err)
	}
	return nil
}

// absenceAlreadyMatchesExcelImport ist true, wenn die gespeicherte Abwesenheit
// dieselbe Bedeutung hat wie das, was der Excel-Import anlegen würde (Ganztag, gleicher Typ).
// Dann entfällt Löschen/Neu-Anlegen — keine irreführenden „neu/ersetzt“-Zähler.
func absenceAlreadyMatchesExcelImport(a *model.Absence, want model.AbsenceType) bool {
	if a == nil || a.AbsenceType != want {
		return false
	}
	return !a.HalfDay
}

func replaceWithVacation(ctx context.Context, deps Deps, uid int, date string, createdBy int, rep *Report, wr *WeekReport) error {
	existing, err := deps.Absences.GetForUserDate(ctx, uid, date)
	if err != nil {
		return err
	}
	if absenceAlreadyMatchesExcelImport(existing, model.AbsenceVacation) {
		return nil
	}
	if err := deleteExistingAbsenceOnly(ctx, deps, uid, date, rep, wr); err != nil {
		return err
	}
	fixed := fixedNonWorkWeekdaysForUser(ctx, deps.FixedNonWorkWeekdays, uid, date)
	if err := validateVacationAbsenceDate(ctx, deps.Holidays, deps.Closures, fixed, date); err != nil {
		return fmt.Errorf("Urlaub %d %s: %v", uid, date, err)
	}
	a := &model.Absence{
		UserID: uid, AbsenceDate: date, AbsenceType: model.AbsenceVacation,
		HalfDay: false, CreatedBy: createdBy,
	}
	if err := deps.Absences.Create(ctx, a); err != nil {
		return fmt.Errorf("Urlaub anlegen %d %s: %w", uid, date, err)
	}
	rep.AbsencesCreated++
	wr.AbsencesCreated++
	return nil
}

func replaceWithCompensationDay(ctx context.Context, deps Deps, uid int, date string, createdBy int, rep *Report, wr *WeekReport) error {
	existing, err := deps.Absences.GetForUserDate(ctx, uid, date)
	if err != nil {
		return err
	}
	if absenceAlreadyMatchesExcelImport(existing, model.AbsenceCompensationDay) {
		return nil
	}
	if err := deleteExistingAbsenceOnly(ctx, deps, uid, date, rep, wr); err != nil {
		return err
	}
	fixed := fixedNonWorkWeekdaysForUser(ctx, deps.FixedNonWorkWeekdays, uid, date)
	if err := validateCompensationDayAbsenceDate(ctx, deps.Holidays, fixed, date); err != nil {
		return fmt.Errorf("Ausgleichstag %d %s: %v", uid, date, err)
	}
	if deps.Claims == nil {
		return fmt.Errorf("Ausgleichstag %d %s: keine Claim-Konfiguration", uid, date)
	}
	claim, err := deps.Claims.GetOldestOpen(ctx, uid)
	if err != nil {
		return fmt.Errorf("Ausgleichstag %d %s: Claims prüfen: %w", uid, date, err)
	}
	if claim == nil {
		return fmt.Errorf("Ausgleichstag %d %s: kein offener Ausgleichstag-Anspruch", uid, date)
	}
	a := &model.Absence{
		UserID: uid, AbsenceDate: date, AbsenceType: model.AbsenceCompensationDay,
		HalfDay: false, CreatedBy: createdBy,
	}
	if err := deps.Absences.Create(ctx, a); err != nil {
		return fmt.Errorf("Ausgleichstag anlegen %d %s: %w", uid, date, err)
	}
	if err := deps.Claims.MarkUsed(ctx, claim.ID, a.ID); err != nil {
		_ = deps.Absences.Delete(ctx, a.ID)
		return fmt.Errorf("Ausgleichstag Claim %d %s: %w", uid, date, err)
	}
	rep.AbsencesCreated++
	wr.AbsencesCreated++
	return nil
}

func replaceWithOther(ctx context.Context, deps Deps, uid int, date string, createdBy int, rep *Report, wr *WeekReport) error {
	existing, err := deps.Absences.GetForUserDate(ctx, uid, date)
	if err != nil {
		return err
	}
	if absenceAlreadyMatchesExcelImport(existing, model.AbsenceOther) {
		return nil
	}
	if err := deleteExistingAbsenceOnly(ctx, deps, uid, date, rep, wr); err != nil {
		return err
	}
	a := &model.Absence{
		UserID: uid, AbsenceDate: date, AbsenceType: model.AbsenceOther,
		HalfDay: false, CreatedBy: createdBy,
	}
	if err := deps.Absences.Create(ctx, a); err != nil {
		return fmt.Errorf("Sonstige Abwesenheit %d %s: %w", uid, date, err)
	}
	rep.AbsencesCreated++
	wr.AbsencesCreated++
	return nil
}

// deleteExistingAbsenceOnly entfernt eine vorhandene Abwesenheit vor dem Anlegen einer neuen Excel-Abwesenheit.
func deleteExistingAbsenceOnly(ctx context.Context, deps Deps, uid int, date string, rep *Report, wr *WeekReport) error {
	a, err := deps.Absences.GetForUserDate(ctx, uid, date)
	if err != nil {
		return err
	}
	if a == nil {
		return nil
	}
	if a.AbsenceType == model.AbsenceCompensationDay && deps.Claims != nil {
		_ = deps.Claims.ReopenByAbsenceID(ctx, a.ID)
	}
	if err := deps.Absences.Delete(ctx, a.ID); err != nil {
		return fmt.Errorf("Abwesenheit ersetzen: löschen %d/%s: %w", uid, date, err)
	}
	rep.AbsencesReplaced++
	wr.AbsencesReplaced++
	return nil
}
