package saldocalc

import (
	"context"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// Stundenkonto: eine gemeinsame Definition für Team-Übersicht, Mitarbeiter-Detail und „Mein Saldo“.
//
//   - Kontobeginn: frühestes valid_from der Wochenstunden; ohne Wochenstunden der 1.1. des Jahres von „gestern“.
//   - Gezählt wird bis einschließlich „gestern“ (Ortszeit). Der laufende Tag ist noch nicht abgeschlossen.
//   - Der Startsaldo (users.opening_hours_balance) zählt, wenn das Anlagedatum des Kontos (created_at)
//     zwischen Kontobeginn und Stichtag liegt.

// AccountStart liefert den ersten Tag des Stundenkontos (lokales Datum, 00:00).
func AccountStart(whRows []model.WeeklyHours, yesterday time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	yy, _, _ := yesterday.In(loc).Date()
	fallback := time.Date(yy, 1, 1, 0, 0, 0, 0, loc)
	var best time.Time
	found := false
	for i := range whRows {
		d, err := time.ParseInLocation("2006-01-02", normDate(whRows[i].ValidFrom), loc)
		if err != nil {
			continue
		}
		if !found || d.Before(best) {
			best = d
			found = true
		}
	}
	if !found {
		return fallback
	}
	return best
}

// OpeningApplies meldet, ob der Startsaldo im Zeitraum accountStart..through enthalten ist.
func OpeningApplies(createdAt, accountStart, through time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = time.Local
	}
	if createdAt.IsZero() || accountStart.After(through) {
		return false
	}
	cy, cm, cd := createdAt.In(loc).Date()
	openingDay := time.Date(cy, cm, cd, 0, 0, 0, 0, loc)
	return !openingDay.Before(accountStart) && !openingDay.After(through)
}

// Yesterday ist der letzte gezählte Tag des Stundenkontos relativ zu now (lokales Datum, 00:00).
func Yesterday(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc).AddDate(0, 0, -1)
}

// AccountBalance ist der Stand des Stundenkontos zum Ende von through (inklusive).
type AccountBalance struct {
	Start          time.Time
	Through        time.Time
	Balance        float64
	OpeningApplied bool
}

// Account berechnet den aktuellen Stand des Stundenkontos (bis einschließlich gestern relativ zu now).
func Account(ctx context.Context, d Deps, u *model.User, now time.Time) (AccountBalance, error) {
	loc := time.Local
	if u == nil {
		return AccountBalance{}, fmt.Errorf("user required")
	}
	var whRows []model.WeeklyHours
	if d.WeeklyHours != nil {
		var err error
		whRows, err = d.WeeklyHours.ListByUser(ctx, u.ID)
		if err != nil {
			return AccountBalance{}, err
		}
	}
	yesterday := Yesterday(now, loc)
	return accountFrom(ctx, d, u, AccountStart(whRows, yesterday, loc), yesterday, loc)
}

func accountFrom(ctx context.Context, d Deps, u *model.User, start, through time.Time, loc *time.Location) (AccountBalance, error) {
	out := AccountBalance{Start: start, Through: through}
	if start.After(through) {
		return out, nil
	}
	totals, err := SumRange(ctx, d, u.ID, start.Format("2006-01-02"), through.Format("2006-01-02"))
	if err != nil {
		return AccountBalance{}, err
	}
	out.Balance = totals.BalanceHours
	if OpeningApplies(u.CreatedAt, start, through, loc) {
		out.Balance = round2(out.Balance + u.OpeningHoursBalance)
		out.OpeningApplied = true
	}
	return out, nil
}

// Month liefert die Monatsansicht des Stundenkontos.
// Ist/Soll/Saldo Monat zählen nur Tage ab Kontobeginn bis einschließlich gestern; „Gesamt“ ist der Kontostand
// am letzten gezählten Tag, „Vortrag“ der Stand vor dem Monat.
func Month(ctx context.Context, d Deps, u *model.User, year, month int, now time.Time) (model.MonthBalance, error) {
	if month < 1 || month > 12 {
		return model.MonthBalance{}, fmt.Errorf("invalid month")
	}
	if u == nil {
		return model.MonthBalance{}, fmt.Errorf("user required")
	}
	loc := time.Local
	yesterday := Yesterday(now, loc)

	var whRows []model.WeeklyHours
	if d.WeeklyHours != nil {
		var err error
		whRows, err = d.WeeklyHours.ListByUser(ctx, u.ID)
		if err != nil {
			return model.MonthBalance{}, err
		}
	}
	start := AccountStart(whRows, yesterday, loc)

	monthStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, -1)
	through := monthEnd
	if yesterday.Before(through) {
		through = yesterday
	}

	out := model.MonthBalance{
		Year: year, Month: month,
		AccountStart: start.Format("2006-01-02"),
	}

	acc, err := accountFrom(ctx, d, u, start, through, loc)
	if err != nil {
		return model.MonthBalance{}, err
	}
	if through.Before(monthStart) {
		// Monat liegt nach gestern (oder ist noch nicht begonnen): nichts gezählt, Gesamt = aktueller Kontostand.
		out.TotalBalance = acc.Balance
		out.Carryover = acc.Balance
		out.IsFuture = true
		return out, nil
	}

	from := monthStart
	if start.After(from) {
		from = start
	}
	if !from.After(through) {
		totals, err := SumRange(ctx, d, u.ID, from.Format("2006-01-02"), through.Format("2006-01-02"))
		if err != nil {
			return model.MonthBalance{}, err
		}
		out.WorkedHours = totals.WorkedHours
		out.TargetHours = totals.TargetHours
		out.BalanceHours = totals.BalanceHours
		out.CountedFrom = from.Format("2006-01-02")
		out.CountedThrough = through.Format("2006-01-02")
	}
	out.TotalBalance = acc.Balance
	out.Carryover = round2(acc.Balance - out.BalanceHours)
	out.IsPartial = through.Before(monthEnd)
	return out, nil
}
