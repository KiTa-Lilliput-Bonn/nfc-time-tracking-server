package store

import (
	"context"
	"time"

	"nfc-time-tracking-server/internal/model"
)

type UserStore interface {
	Create(ctx context.Context, u *model.User) error
	GetByID(ctx context.Context, id int) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	List(ctx context.Context, activeOnly bool) ([]model.User, error)
	Update(ctx context.Context, u *model.User) error
	// SetPassword updates password hash and must_change_password flag.
	SetPassword(ctx context.Context, userID int, passwordHash string, mustChangePassword bool) error
	Count(ctx context.Context) (int, error)
}

type GroupStore interface {
	List(ctx context.Context) ([]model.Group, error)
	GetByID(ctx context.Context, id int) (*model.Group, error)
	Create(ctx context.Context, g *model.Group) error
	Update(ctx context.Context, g *model.Group) error
	Delete(ctx context.Context, id int) error
	// Reorder setzt die Reihenfolge aller Gruppen; ids muss jede Gruppe genau einmal enthalten.
	Reorder(ctx context.Context, ids []int) error
}

type NFCTagStore interface {
	Assign(ctx context.Context, tag *model.NFCTag) error
	GetActiveByTagUID(ctx context.Context, tagUID string) (*model.NFCTag, error)
	ListByUser(ctx context.Context, userID int) ([]model.NFCTag, error)
	ResolveUserID(ctx context.Context, tagUID string, at time.Time) (int, error)
	// ListActiveUserIDs returns distinct active user/leitung IDs with at least one NFC tag assignment.
	ListActiveUserIDsWithOpenNFCTag(ctx context.Context) ([]int, error)
	// LatestOpenTagUID returns the tag_uid valid for the user on today's calendar date.
	LatestOpenTagUID(ctx context.Context, userID int) (string, error)
	// TagUIDForUserAt returns the NFC tag_uid assigned to the user on the given instant (calendar date in UTC for overlap).
	TagUIDForUserAt(ctx context.Context, userID int, at time.Time) (string, error)
}

type PunchStore interface {
	InsertBatch(ctx context.Context, punches []model.RawPunch) (int, error)
	ListByUserAndDate(ctx context.Context, userID int, date string) ([]model.RawPunch, error)
	// ListByUTCDateForLanSync returns raw punches on the given UTC calendar date (YYYY-MM-DD) for all active users with valid tag overlap.
	ListByUTCDateForLanSync(ctx context.Context, utcDate string) ([]model.LanSyncPunch, error)
}

type WorkPeriodStore interface {
	ReplaceForUserDate(ctx context.Context, userID int, date string, periods []model.WorkPeriod) error
	ListByUserDateRange(ctx context.Context, userID int, from, to string) ([]model.WorkPeriod, error)
	GetByID(ctx context.Context, id int) (*model.WorkPeriod, error)
	CreateManual(ctx context.Context, wp *model.WorkPeriod) error
	DeleteManual(ctx context.Context, id int) error
}

type CorrectionStore interface {
	Create(ctx context.Context, c *model.TimeCorrection) error
	GetLatestForPeriod(ctx context.Context, workPeriodID int) (*model.TimeCorrection, error)
	ListByUser(ctx context.Context, userID int, from, to string) ([]model.TimeCorrection, error)
}

type WeeklyHoursStore interface {
	Set(ctx context.Context, wh *model.WeeklyHours) error
	Delete(ctx context.Context, userID int, id int) error
	GetByID(ctx context.Context, userID int, id int) (*model.WeeklyHours, error)
	GetForDate(ctx context.Context, userID int, date string) (*model.WeeklyHours, error)
	ListByUser(ctx context.Context, userID int) ([]model.WeeklyHours, error)
}

type VacationEntitlementStore interface {
	Set(ctx context.Context, ve *model.VacationEntitlement) error
	Delete(ctx context.Context, userID int, id int) error
	GetByID(ctx context.Context, userID int, id int) (*model.VacationEntitlement, error)
	GetForDate(ctx context.Context, userID int, date string) (*model.VacationEntitlement, error)
	ListByUser(ctx context.Context, userID int) ([]model.VacationEntitlement, error)
}

type FixedNonWorkWeekdaysStore interface {
	Set(ctx context.Context, row *model.FixedNonWorkWeekdays) error
	Delete(ctx context.Context, userID int, id int) error
	GetByID(ctx context.Context, userID int, id int) (*model.FixedNonWorkWeekdays, error)
	GetForDate(ctx context.Context, userID int, date string) (*model.FixedNonWorkWeekdays, error)
	ListByUser(ctx context.Context, userID int) ([]model.FixedNonWorkWeekdays, error)
}

type ScheduleStore interface {
	Set(ctx context.Context, s *model.Schedule) error
	GetByID(ctx context.Context, id int) (*model.Schedule, error)
	GetForUserDate(ctx context.Context, userID int, date string) (*model.Schedule, error)
	ListByUserDateRange(ctx context.Context, userID int, from, to string) ([]model.Schedule, error)
	ListByWeek(ctx context.Context, year, week int) ([]model.Schedule, error)
	Delete(ctx context.Context, id int) error
	// ISO week-year (Kalenderjahr) + ISO week number (1–53), same as ListByWeek.
	GetWeekNotes(ctx context.Context, isoYear, isoWeek int) (string, error)
	PutWeekNotes(ctx context.Context, isoYear, isoWeek int, notes string) error
	// LastISOWeekWithShift returns ISO year/week of the latest schedule_date, or ok=false if none.
	LastISOWeekWithShift(ctx context.Context) (year, week int, ok bool, err error)
}

// TeamMeetingStore verwaltet geplante Teamsitzungen (Montag) mit expliziter Nutzerzuordnung.
type TeamMeetingStore interface {
	DeleteByWeekAndSource(ctx context.Context, isoYear, isoWeek int, source string) error
	// NextManualSectionIndex liefert den nächsten freien section_index für source=manual und gegebene Art in dieser ISO-Woche.
	NextManualSectionIndex(ctx context.Context, isoYear, isoWeek int, kind model.TeamMeetingKind) (int, error)
	CreateWithUsers(ctx context.Context, m *model.TeamMeeting) error
	ListByWeek(ctx context.Context, isoYear, isoWeek int) ([]model.TeamMeeting, error)
	ListForUserInDateRange(ctx context.Context, userID int, from, to string) ([]model.TeamMeeting, error)
	GetByID(ctx context.Context, id int) (*model.TeamMeeting, error)
	ReplaceMeetingAndUsers(ctx context.Context, m *model.TeamMeeting) error
	Delete(ctx context.Context, id int) error
}

type AbsenceStore interface {
	Create(ctx context.Context, a *model.Absence) error
	Delete(ctx context.Context, id int) error
	GetByID(ctx context.Context, id int) (*model.Absence, error)
	GetForUserDate(ctx context.Context, userID int, date string) (*model.Absence, error)
	ListByUserDateRange(ctx context.Context, userID int, from, to string) ([]model.Absence, error)
	// ListByDateRangeTypes lists absences in [from,to] whose type is one of types (inclusive date range).
	ListByDateRangeTypes(ctx context.Context, from, to string, types []model.AbsenceType) ([]model.Absence, error)
}

type CompensationDayClaimStore interface {
	EnsureForWorkDate(ctx context.Context, userID int, workDate string, hasEligibleWork bool) error
	GetOldestOpen(ctx context.Context, userID int) (*model.CompensationDayClaim, error)
	MarkUsed(ctx context.Context, claimID int, absenceID int) error
	ReopenByAbsenceID(ctx context.Context, absenceID int) error
	Waive(ctx context.Context, userID int, claimID int) error
	ListByUser(ctx context.Context, userID int, status *model.CompensationDayClaimStatus) ([]model.CompensationDayClaim, error)
	CountOpen(ctx context.Context, userID int) (int, error)
}

type HolidayStore interface {
	Create(ctx context.Context, h *model.Holiday) error
	Delete(ctx context.Context, id int) error
	ListByYear(ctx context.Context, year int) ([]model.Holiday, error)
	GetForDate(ctx context.Context, date string) (*model.Holiday, error)
	DeleteAutoGenerated(ctx context.Context, year int) error
}

type ClosureDayStore interface {
	Create(ctx context.Context, c *model.ClosureDay) error
	Delete(ctx context.Context, id int) error
	List(ctx context.Context) ([]model.ClosureDay, error)
	GetForDate(ctx context.Context, date string) (*model.ClosureDay, error)
}

type SettingsStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetAll(ctx context.Context) ([]model.Setting, error)
}

// ApiPairedClientStore manages bearer secrets for the device/LAN API (superadmin only via JWT handlers).
type ApiPairedClientStore interface {
	Insert(ctx context.Context, c *model.ApiPairedClient) error
	List(ctx context.Context) ([]model.ApiPairedClient, error)
	GetByID(ctx context.Context, id string) (*model.ApiPairedClient, error)
	// ListAuthorizedSecrets returns active clients with non-empty secret (for bearer validation).
	ListAuthorizedSecrets(ctx context.Context) ([]model.ApiPairedClient, error)
	Delete(ctx context.Context, id string) error
}

// ApiPairingSessionStore manages short-lived pairing tokens for device registration.
type ApiPairingSessionStore interface {
	CreateSession(ctx context.Context, clientID, tokenHash, expiresAtUTC, createdAtUTC string) error
	ConsumeSession(ctx context.Context, tokenHash string) (clientID string, err error)
}
