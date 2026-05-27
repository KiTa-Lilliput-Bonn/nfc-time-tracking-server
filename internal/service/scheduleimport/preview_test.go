package scheduleimport_test

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/scheduleimport"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestPreviewPastImport_MatchesFutureApplySkipCounts(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	u := &model.User{
		Username: "p", PasswordHash: "x", DisplayName: "Past User",
		Role: model.RoleUser, Active: true,
	}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	mon := time.Date(2026, 3, 16, 12, 0, 0, 0, time.Local)
	dates := [5]string{
		mon.Format("2006-01-02"),
		mon.AddDate(0, 0, 1).Format("2006-01-02"),
		mon.AddDate(0, 0, 2).Format("2006-01-02"),
		mon.AddDate(0, 0, 3).Format("2006-01-02"),
		mon.AddDate(0, 0, 4).Format("2006-01-02"),
	}
	isoY, isoW := mon.ISOWeek()

	parsed := &scheduleimport.ParsedSheet{
		Weeks: []scheduleimport.ParsedWeek{{
			ISOYear: isoY,
			ISOWk:   isoW,
			Notes:   "<p>Alte Woche</p>",
			Dates:   dates,
			Rows: []scheduleimport.ParsedEmployeeRow{{
				RawName: "Past User",
				Cells:   [5]string{"10:00-18:00", "U", "F", "09:00-17:00", ""},
			}},
		}},
	}

	deps := scheduleimport.Deps{
		Users: us, Schedules: sqlite.NewScheduleStore(db), Absences: sqlite.NewAbsenceStore(db),
		Holidays: sqlite.NewHolidayStore(db), Closures: sqlite.NewClosureDayStore(db),
		Claims: sqlite.NewCompensationDayClaimStore(db), TeamMeetings: nil,
	}

	const todayPastImport = "2026-06-15"
	prev := scheduleimport.PreviewPastImportFromParsed(ctx, deps, parsed, todayPastImport)

	rep, err := scheduleimport.Apply(ctx, deps, parsed, 1, todayPastImport, scheduleimport.ImportScopeFuture)
	if err != nil {
		t.Fatal(err)
	}
	if prev.PastCellsSkipped != rep.PastCellsSkipped {
		t.Fatalf("cells: preview %d apply %d", prev.PastCellsSkipped, rep.PastCellsSkipped)
	}
	if prev.PastWeekNotesSkipped != rep.PastWeekNotesSkipped {
		t.Fatalf("week notes: preview %d apply %d", prev.PastWeekNotesSkipped, rep.PastWeekNotesSkipped)
	}
	if prev.PastTeamMeetingsSkipped != rep.PastTeamMeetingsSkipped {
		t.Fatalf("team meetings: preview %d apply %d", prev.PastTeamMeetingsSkipped, rep.PastTeamMeetingsSkipped)
	}
}
