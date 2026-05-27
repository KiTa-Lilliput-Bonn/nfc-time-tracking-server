package sqlite

import (
	"context"
	"testing"

	"nfc-time-tracking-server/internal/model"
)

func TestTeamMeetingStore_OtherKindWithLabel(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	u := &model.User{Username: "tmu", PasswordHash: "x", DisplayName: "U", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	tms := NewTeamMeetingStore(db)

	m := &model.TeamMeeting{
		ISOWeekYear:  2026,
		ISOWeek:      12,
		MeetingDate:  "2026-03-18",
		Kind:         model.TeamMeetingKindOther,
		Label:        "Fortbildung",
		TimeStart:    "09:00",
		TimeEnd:      "10:30",
		Source:       "manual",
		SectionIndex: 0,
		UserIDs:      []int{u.ID},
	}
	if err := tms.CreateWithUsers(ctx, m); err != nil {
		t.Fatal(err)
	}
	got, err := tms.GetByID(ctx, m.ID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Kind != model.TeamMeetingKindOther || got.Label != "Fortbildung" {
		t.Fatalf("got kind=%q label=%q", got.Kind, got.Label)
	}
	if got.MeetingDate != "2026-03-18" {
		t.Fatalf("meeting_date: %q", got.MeetingDate)
	}
}

func TestValidateScheduleWeekday(t *testing.T) {
	if err := ValidateScheduleWeekday(2026, 12, "2026-03-16"); err != nil {
		t.Fatalf("monday: %v", err)
	}
	if err := ValidateScheduleWeekday(2026, 12, "2026-03-20"); err != nil {
		t.Fatalf("friday: %v", err)
	}
	if err := ValidateScheduleWeekday(2026, 12, "2026-03-21"); err == nil {
		t.Fatal("expected error for saturday")
	}
	if err := ValidateScheduleWeekday(2026, 12, "2026-03-09"); err == nil {
		t.Fatal("expected error for date outside week")
	}
}
