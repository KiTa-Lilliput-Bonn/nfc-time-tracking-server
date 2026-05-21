package importer

import (
	"sort"

	"nfc-time-tracking-server/internal/model"
)

// PairPunches builds work periods from raw punches: each pair (p[0],p[1]), (p[2],p[3]), … is a closed
// work interval; time between punch-out and the next punch-in is implicit pause (not stored).
// An odd number of punches yields one open work period from the last stamp. All periods have IsBreak false.
// Source is empty (Standard-Stempel/Import); nur explizit angelegte Zeilen sind source=manual.
func PairPunches(userID int, workDate string, punches []model.RawPunch) []model.WorkPeriod {
	if len(punches) == 0 {
		return nil
	}
	sorted := append([]model.RawPunch(nil), punches...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PunchTime.Before(sorted[j].PunchTime)
	})

	var out []model.WorkPeriod
	for j := 0; j+1 < len(sorted); j += 2 {
		in := sorted[j].PunchTime
		outT := sorted[j+1].PunchTime
		out = append(out, model.WorkPeriod{
			UserID:   userID,
			WorkDate: workDate,
			PunchIn:  in,
			PunchOut: &outT,
			IsBreak:  false,
			Source:   "",
		})
	}
	if len(sorted)%2 == 1 {
		last := len(sorted) - 1
		out = append(out, model.WorkPeriod{
			UserID:   userID,
			WorkDate: workDate,
			PunchIn:  sorted[last].PunchTime,
			PunchOut: nil,
			IsBreak:  false,
			Source:   "",
		})
	}
	return out
}
