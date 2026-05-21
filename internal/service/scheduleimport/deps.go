package scheduleimport

import (
	"nfc-time-tracking-server/internal/store"
)

// Deps enthält die Stores für den Import (Leitungspfad).
type Deps struct {
	Users     store.UserStore
	Schedules store.ScheduleStore
	Absences  store.AbsenceStore
	Holidays  store.HolidayStore
	Closures  store.ClosureDayStore
	Claims    store.CompensationDayClaimStore
	TeamMeetings store.TeamMeetingStore
}
