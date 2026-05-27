package scheduleexport

import (
	"nfc-time-tracking-server/internal/store"
)

// Deps enthält die Stores für den Excel-Export.
type Deps struct {
	Users                store.UserStore
	Groups               store.GroupStore
	Schedules            store.ScheduleStore
	TeamMeetings         store.TeamMeetingStore
	Absences             store.AbsenceStore
	Holidays             store.HolidayStore
	FixedNonWorkWeekdays store.FixedNonWorkWeekdaysStore
}
