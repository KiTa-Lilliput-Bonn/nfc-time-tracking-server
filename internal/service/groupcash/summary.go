// Package groupcash berechnet Kassenstand und fiktives Ansparkonto einer Gruppenkasse.
//
// Jeder Kasse steht pro Monat ein Betrag zu (versioniert über CashAllowance). Was für einen
// abgelaufenen Monat nicht als Monatsbetrag ausgezahlt wurde, wandert ins Ansparkonto. Aus dem
// Ansparkonto bucht der Kassenwart per Einnahme mit Quelle „savings“ in die Kasse.
//
//	Ansparkonto = Anfangsbestand + Σ abgelaufene Monate (Anspruch − ausgezahlter Monatsbetrag) − Σ Entnahmen
//
// Der laufende Monat zählt noch nicht: sein Anspruch kann noch als Monatsbetrag abgerufen werden.
// Mit Anfangsbestand zählen nur Monate ab dem Monat des Stichtags; ein nachträglich gebuchter
// Monatsbetrag für einen früheren Monat verringert das Ansparkonto direkt (der Anfangsbestand enthielt ihn).
package groupcash

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// MonthRow ist eine Zeile der Monatsübersicht des Ansparkontos.
type MonthRow struct {
	Month          string `json:"month"`
	AllowanceCents int64  `json:"allowance_cents"`
	PaidCents      int64  `json:"paid_cents"`
	// SavedCents geht in diesem Monat ins Ansparkonto (Anspruch − ausgezahlt); im laufenden Monat 0.
	SavedCents int64 `json:"saved_cents"`
	// WithdrawnCents sind Entnahmen aus dem Ansparkonto mit Buchungsdatum in diesem Monat.
	WithdrawnCents int64 `json:"withdrawn_cents"`
	Current        bool  `json:"current"`
}

// Summary fasst eine Gruppenkasse zusammen.
type Summary struct {
	// OpeningCashCents / OpeningSavingsCents: Anfangsbestand (0 ohne Anfangsbestand).
	OpeningCashCents    int64  `json:"opening_cash_cents"`
	OpeningSavingsCents int64  `json:"opening_savings_cents"`
	OpeningDate         string `json:"opening_date,omitempty"`

	BalanceCents int64 `json:"balance_cents"`
	IncomeCents  int64 `json:"income_cents"`
	ExpenseCents int64 `json:"expense_cents"`
	SavingsCents int64 `json:"savings_cents"`

	CurrentMonth          string `json:"current_month"`
	CurrentAllowanceCents int64  `json:"current_allowance_cents"`
	CurrentPaidCents      int64  `json:"current_paid_cents"`
	// CurrentOpenCents ist der im laufenden Monat noch abrufbare Monatsbetrag.
	CurrentOpenCents int64 `json:"current_open_cents"`

	// Months: neueste zuerst, vom ersten relevanten Monat bis zum laufenden Monat.
	Months []MonthRow `json:"months"`
}

// MonthOf liefert YYYY-MM für t.
func MonthOf(t time.Time) string {
	return t.Format("2006-01")
}

// ValidMonth prüft das Format YYYY-MM.
func ValidMonth(m string) bool {
	_, err := time.Parse("2006-01", m)
	return err == nil && len(m) == 7
}

func nextMonth(m string) string {
	t, _ := time.Parse("2006-01", m)
	return t.AddDate(0, 1, 0).Format("2006-01")
}

// AllowanceFor liefert den Anspruch im Monat m (letzte Version mit ValidFrom ≤ m).
func AllowanceFor(allowances []model.CashAllowance, m string) int64 {
	var best string
	var amount int64
	for _, a := range allowances {
		if a.ValidFrom <= m && a.ValidFrom >= best {
			best = a.ValidFrom
			amount = a.AmountCents
		}
	}
	return amount
}

// Compute berechnet Kassenstand, Ansparkonto und Monatsübersicht. opening darf nil sein.
func Compute(opening *model.CashOpening, allowances []model.CashAllowance, entries []model.CashEntry, currentMonth string) Summary {
	s := Summary{CurrentMonth: currentMonth}
	openingMonth := ""
	if opening != nil {
		s.OpeningCashCents = opening.CashCents
		s.OpeningSavingsCents = opening.SavingsCents
		s.OpeningDate = opening.Date
		openingMonth = opening.Date[:7]
	}
	paid := map[string]int64{}
	withdrawn := map[string]int64{}
	start := ""
	consider := func(m string) {
		if m < openingMonth {
			m = openingMonth
		}
		if m != "" && m <= currentMonth && (start == "" || m < start) {
			start = m
		}
	}
	for _, a := range allowances {
		consider(a.ValidFrom)
	}
	for _, e := range entries {
		if e.Kind == model.CashExpense {
			s.ExpenseCents += e.AmountCents
		} else {
			s.IncomeCents += e.AmountCents
		}
		switch e.Source {
		case model.CashSourceAllowance:
			paid[e.ForMonth] += e.AmountCents
			consider(e.ForMonth)
		case model.CashSourceSavings:
			m := e.EntryDate
			if len(m) >= 7 {
				m = m[:7]
			}
			withdrawn[m] += e.AmountCents
			s.SavingsCents -= e.AmountCents
			consider(m)
		}
	}
	s.BalanceCents = s.OpeningCashCents + s.IncomeCents - s.ExpenseCents
	s.SavingsCents += s.OpeningSavingsCents
	for m, cents := range paid {
		if m < openingMonth {
			s.SavingsCents -= cents
		}
	}

	if start != "" {
		for m := start; m <= currentMonth; m = nextMonth(m) {
			row := MonthRow{
				Month:          m,
				AllowanceCents: AllowanceFor(allowances, m),
				PaidCents:      paid[m],
				WithdrawnCents: withdrawn[m],
				Current:        m == currentMonth,
			}
			if !row.Current {
				row.SavedCents = row.AllowanceCents - row.PaidCents
				s.SavingsCents += row.SavedCents
			}
			s.Months = append(s.Months, row)
		}
	}
	sort.Slice(s.Months, func(i, j int) bool { return s.Months[i].Month > s.Months[j].Month })

	s.CurrentAllowanceCents = AllowanceFor(allowances, currentMonth)
	s.CurrentPaidCents = paid[currentMonth]
	if open := s.CurrentAllowanceCents - s.CurrentPaidCents; open > 0 {
		s.CurrentOpenCents = open
	}
	return s
}

// CheckEntry prüft eine neue oder geänderte Buchung gegen Monatsanspruch und Ansparkonto.
// others sind alle übrigen Buchungen der Kasse (ohne die geprüfte). Die Fehlermeldung ist für Nutzer gedacht.
func CheckEntry(opening *model.CashOpening, allowances []model.CashAllowance, others []model.CashEntry, e model.CashEntry, currentMonth string) error {
	if opening != nil && e.EntryDate < opening.Date {
		return fmt.Errorf("Buchungen vor dem Anfangsbestand (%s) sind nicht möglich.", GermanDate(opening.Date))
	}
	switch e.Source {
	case model.CashSourceAllowance:
		if e.ForMonth > currentMonth {
			return fmt.Errorf("Der Monatsbetrag für %s kann erst ab diesem Monat gebucht werden.", MonthLabel(e.ForMonth))
		}
		var already int64
		for _, o := range others {
			if o.Source == model.CashSourceAllowance && o.ForMonth == e.ForMonth {
				already += o.AmountCents
			}
		}
		allowed := AllowanceFor(allowances, e.ForMonth)
		if already+e.AmountCents > allowed {
			open := allowed - already
			if open < 0 {
				open = 0
			}
			return fmt.Errorf("Für %s stehen der Kasse nur noch %s zu. Mehr bitte als Entnahme aus dem Ansparkonto buchen.",
				MonthLabel(e.ForMonth), FormatEuro(open))
		}
	case model.CashSourceSavings:
		before := Compute(opening, allowances, others, currentMonth).SavingsCents
		if e.AmountCents > before {
			if before < 0 {
				before = 0
			}
			return fmt.Errorf("Im Ansparkonto sind nur %s verfügbar.", FormatEuro(before))
		}
	}
	if e.Source == model.CashSourceAllowance {
		// Ein Monatsbetrag für einen vergangenen Monat verringert das Ansparkonto; es darf nicht negativ werden.
		after := Compute(opening, allowances, append(append([]model.CashEntry{}, others...), e), currentMonth).SavingsCents
		if after < 0 && e.ForMonth < currentMonth {
			return fmt.Errorf("Der Betrag für %s wurde bereits über das Ansparkonto entnommen.", MonthLabel(e.ForMonth))
		}
	}
	return nil
}

var monthNames = []string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}

// MonthLabel formatiert YYYY-MM als „April 2026“.
func MonthLabel(m string) string {
	t, err := time.Parse("2006-01", m)
	if err != nil {
		return m
	}
	return fmt.Sprintf("%s %d", monthNames[t.Month()-1], t.Year())
}

// GermanDate formatiert YYYY-MM-DD als TT.MM.JJJJ.
func GermanDate(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.Format("02.01.2006")
}

// FormatEuro formatiert Cent als „1.234,56 €“.
func FormatEuro(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	euros := strconv.FormatInt(cents/100, 10)
	var b strings.Builder
	for i, c := range euros {
		if i > 0 && (len(euros)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	out := fmt.Sprintf("%s,%02d €", b.String(), cents%100)
	if neg {
		out = "-" + out
	}
	return out
}
