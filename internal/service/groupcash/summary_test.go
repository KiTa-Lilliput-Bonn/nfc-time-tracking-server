package groupcash

import (
	"strings"
	"testing"

	"nfc-time-tracking-server/internal/model"
)

func income(src model.CashIncomeSource, date, forMonth string, cents int64) model.CashEntry {
	return model.CashEntry{Kind: model.CashIncome, Source: src, EntryDate: date, ForMonth: forMonth, AmountCents: cents}
}

func expense(date string, cents int64) model.CashEntry {
	return model.CashEntry{Kind: model.CashExpense, EntryDate: date, AmountCents: cents}
}

// Beispiel aus der Anforderung: 100 € pro Monat, Januar ausgezahlt, Februar und März nicht.
// Im April werden 100 € Monatsbetrag und 200 € aus dem Ansparkonto als zwei Einnahmen gebucht.
func TestCompute_SavingsExample(t *testing.T) {
	allow := []model.CashAllowance{{ValidFrom: "2026-01", AmountCents: 10000}}
	entries := []model.CashEntry{
		income(model.CashSourceAllowance, "2026-01-10", "2026-01", 10000),
		expense("2026-01-20", 4000),
	}

	s := Compute(allow, entries, "2026-04")
	if s.SavingsCents != 20000 {
		t.Fatalf("savings before April payout: got %d, want 20000", s.SavingsCents)
	}
	if s.CurrentOpenCents != 10000 {
		t.Fatalf("open in April: got %d", s.CurrentOpenCents)
	}

	april := income(model.CashSourceAllowance, "2026-04-02", "2026-04", 10000)
	if err := CheckEntry(allow, entries, april, "2026-04"); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, april)
	fromSavings := income(model.CashSourceSavings, "2026-04-02", "", 20000)
	if err := CheckEntry(allow, entries, fromSavings, "2026-04"); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, fromSavings, expense("2026-04-05", 25000))

	s = Compute(allow, entries, "2026-04")
	if s.SavingsCents != 0 {
		t.Fatalf("savings after withdrawal: got %d", s.SavingsCents)
	}
	if s.BalanceCents != 10000-4000+10000+20000-25000 {
		t.Fatalf("balance: got %d", s.BalanceCents)
	}
	if s.CurrentOpenCents != 0 {
		t.Fatalf("open after payout: got %d", s.CurrentOpenCents)
	}
	if len(s.Months) != 4 || s.Months[0].Month != "2026-04" || !s.Months[0].Current || s.Months[0].WithdrawnCents != 20000 {
		t.Fatalf("months: %+v", s.Months)
	}
	if s.Months[1].SavedCents != 10000 || s.Months[3].SavedCents != 0 {
		t.Fatalf("saved per month: %+v", s.Months)
	}

	// Nächster Monat: der April zählt jetzt als abgelaufen (vollständig ausgezahlt), Mai noch offen.
	if s := Compute(allow, entries, "2026-05"); s.SavingsCents != 0 || s.CurrentOpenCents != 10000 {
		t.Fatalf("May: %+v", s)
	}
}

func TestCompute_AllowanceVersions(t *testing.T) {
	allow := []model.CashAllowance{
		{ValidFrom: "2026-01", AmountCents: 10000},
		{ValidFrom: "2026-03", AmountCents: 15000},
	}
	s := Compute(allow, nil, "2026-04")
	if s.SavingsCents != 10000+10000+15000 {
		t.Fatalf("savings: %d", s.SavingsCents)
	}
	if s.CurrentAllowanceCents != 15000 {
		t.Fatalf("current allowance: %d", s.CurrentAllowanceCents)
	}
	if AllowanceFor(allow, "2025-12") != 0 {
		t.Fatal("no allowance before first version")
	}
}

func TestCheckEntry_Limits(t *testing.T) {
	allow := []model.CashAllowance{{ValidFrom: "2026-01", AmountCents: 10000}}
	cur := "2026-03"

	err := CheckEntry(allow, nil, income(model.CashSourceAllowance, "2026-03-01", "2026-03", 10001), cur)
	if err == nil || !strings.Contains(err.Error(), "100,00 €") {
		t.Fatalf("expected allowance limit error, got %v", err)
	}
	if err := CheckEntry(allow, nil, income(model.CashSourceAllowance, "2026-03-01", "2026-04", 100), cur); err == nil {
		t.Fatal("future month must be refused")
	}
	// Jan + Feb gespart = 200 €.
	if err := CheckEntry(allow, nil, income(model.CashSourceSavings, "2026-03-01", "", 20001), cur); err == nil ||
		!strings.Contains(err.Error(), "200,00 €") {
		t.Fatalf("expected savings limit error, got %v", err)
	}
	// Nachträglicher Januar-Betrag, nachdem das Ansparkonto schon leer ist.
	others := []model.CashEntry{income(model.CashSourceSavings, "2026-03-01", "", 20000)}
	if err := CheckEntry(allow, others, income(model.CashSourceAllowance, "2026-03-02", "2026-01", 5000), cur); err == nil {
		t.Fatal("late payout for a month already drawn from savings must be refused")
	}
	// Sonstige Einnahmen und Ausgaben sind unbegrenzt.
	if err := CheckEntry(allow, nil, income(model.CashSourceOther, "2026-03-01", "", 99999999), cur); err != nil {
		t.Fatal(err)
	}
}

func TestFormatEuro(t *testing.T) {
	for cents, want := range map[int64]string{0: "0,00 €", 5: "0,05 €", 123456: "1.234,56 €", -100000000: "-1.000.000,00 €"} {
		if got := FormatEuro(cents); got != want {
			t.Errorf("FormatEuro(%d) = %q, want %q", cents, got, want)
		}
	}
}
