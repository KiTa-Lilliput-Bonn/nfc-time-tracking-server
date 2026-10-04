package groupcash

import (
	"strings"
	"testing"
	"time"

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

	s := Compute(nil, allow, entries, "2026-04")
	if s.SavingsCents != 20000 {
		t.Fatalf("savings before April payout: got %d, want 20000", s.SavingsCents)
	}
	if s.CurrentOpenCents != 10000 {
		t.Fatalf("open in April: got %d", s.CurrentOpenCents)
	}

	april := income(model.CashSourceAllowance, "2026-04-02", "2026-04", 10000)
	if err := CheckEntry(nil, allow, entries, april, "2026-04"); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, april)
	fromSavings := income(model.CashSourceSavings, "2026-04-02", "", 20000)
	if err := CheckEntry(nil, allow, entries, fromSavings, "2026-04"); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, fromSavings, expense("2026-04-05", 25000))

	s = Compute(nil, allow, entries, "2026-04")
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
	if s := Compute(nil, allow, entries, "2026-05"); s.SavingsCents != 0 || s.CurrentOpenCents != 10000 {
		t.Fatalf("May: %+v", s)
	}
}

func TestCompute_AllowanceVersions(t *testing.T) {
	allow := []model.CashAllowance{
		{ValidFrom: "2026-01", AmountCents: 10000},
		{ValidFrom: "2026-03", AmountCents: 15000},
	}
	s := Compute(nil, allow, nil, "2026-04")
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

	err := CheckEntry(nil, allow, nil, income(model.CashSourceAllowance, "2026-03-01", "2026-03", 10001), cur)
	if err == nil || !strings.Contains(err.Error(), "100,00 €") {
		t.Fatalf("expected allowance limit error, got %v", err)
	}
	if err := CheckEntry(nil, allow, nil, income(model.CashSourceAllowance, "2026-03-01", "2026-04", 100), cur); err == nil {
		t.Fatal("future month must be refused")
	}
	// Jan + Feb gespart = 200 €.
	if err := CheckEntry(nil, allow, nil, income(model.CashSourceSavings, "2026-03-01", "", 20001), cur); err == nil ||
		!strings.Contains(err.Error(), "200,00 €") {
		t.Fatalf("expected savings limit error, got %v", err)
	}
	// Nachträglicher Januar-Betrag, nachdem das Ansparkonto schon leer ist.
	others := []model.CashEntry{income(model.CashSourceSavings, "2026-03-01", "", 20000)}
	if err := CheckEntry(nil, allow, others, income(model.CashSourceAllowance, "2026-03-02", "2026-01", 5000), cur); err == nil {
		t.Fatal("late payout for a month already drawn from savings must be refused")
	}
	// Sonstige Einnahmen und Ausgaben sind unbegrenzt.
	if err := CheckEntry(nil, allow, nil, income(model.CashSourceOther, "2026-03-01", "", 99999999), cur); err != nil {
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

func TestCompute_Opening(t *testing.T) {
	// Stichtag 15.03.: 80 € in der Kasse, 300 € im Ansparkonto. Anspruch 100 €/Monat seit Jahresbeginn.
	opening := &model.CashOpening{Date: "2026-03-15", CashCents: 8000, SavingsCents: 30000}
	allow := []model.CashAllowance{{ValidFrom: "2026-01", AmountCents: 10000}}
	entries := []model.CashEntry{expense("2026-03-20", 3000)}

	s := Compute(opening, allow, entries, "2026-05")
	// Januar/Februar stecken im Anfangsbestand; März und April laufen ins Ansparkonto.
	if s.SavingsCents != 30000+20000 {
		t.Fatalf("savings: %d", s.SavingsCents)
	}
	if s.BalanceCents != 5000 {
		t.Fatalf("balance: %d", s.BalanceCents)
	}
	if len(s.Months) != 3 || s.Months[2].Month != "2026-03" {
		t.Fatalf("months: %+v", s.Months)
	}
	// Nachgezahlter Februar-Betrag verringert das Ansparkonto.
	late := income(model.CashSourceAllowance, "2026-04-01", "2026-02", 10000)
	if err := CheckEntry(opening, allow, entries, late, "2026-05"); err != nil {
		t.Fatal(err)
	}
	if got := Compute(opening, allow, append(entries, late), "2026-05").SavingsCents; got != 40000 {
		t.Fatalf("savings after late payout: %d", got)
	}
	if err := CheckEntry(opening, allow, entries, expense("2026-03-14", 100), "2026-05"); err == nil {
		t.Fatal("entry before opening date must be refused")
	}
}

func TestLedger_CarryAndExports(t *testing.T) {
	opening := &model.CashOpening{Date: "2025-11-01", CashCents: 5000, SavingsCents: 0}
	entries := []model.CashEntry{
		{ID: 1, Kind: model.CashExpense, EntryDate: "2025-12-01", AmountCents: 1000, Description: "Nikolaus"},
		{ID: 2, Kind: model.CashIncome, Source: model.CashSourceAllowance, ForMonth: "2026-01", EntryDate: "2026-01-05", AmountCents: 10000},
		{ID: 3, Kind: model.CashExpense, EntryDate: "2026-02-10", AmountCents: 2550, Description: "Bastelmaterial für Ostern"},
	}
	receipts := []model.CashReceipt{{ID: 9, EntryID: 3, Filename: "bon.pdf"}}
	l := BuildLedger("Mäuse", 2026, opening, entries, receipts, Summary{})
	if l.CarryLabel != "Übertrag aus 2025" || l.CarryCents != 4000 || len(l.Rows) != 2 || l.EndCents != 11450 {
		t.Fatalf("ledger: %+v", l)
	}
	if l.Rows[1].No != 2 || ReceiptName(l.Rows[1], 0) != "002_2026-02-10_bon.pdf" {
		t.Fatalf("row numbering: %+v", l.Rows[1])
	}
	if l25 := BuildLedger("Mäuse", 2025, opening, entries, receipts, Summary{}); l25.CarryLabel != "Anfangsbestand" || l25.CarryCents != 5000 {
		t.Fatalf("2025: %+v", l25)
	}

	var buf strings.Builder
	if err := l.WriteCSV(&buf, true); err != nil {
		t.Fatal(err)
	}
	csv := buf.String()
	for _, want := range []string{"\uFEFFNr;Datum;", "Monatsbetrag Januar 2026", ";25,50;114,50;002_2026-02-10_bon.pdf", "Summe 2026;;100,00;25,50;114,50"} {
		if !strings.Contains(csv, want) {
			t.Errorf("csv missing %q:\n%s", want, csv)
		}
	}
	pdf, err := l.PDF(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	if err != nil || !strings.HasPrefix(string(pdf), "%PDF") {
		t.Fatalf("pdf: %v", err)
	}
}
