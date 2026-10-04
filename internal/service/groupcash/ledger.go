package groupcash

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"

	"nfc-time-tracking-server/internal/model"
)

// LedgerRow ist eine Zeile des Kassenbuchs eines Jahres.
type LedgerRow struct {
	// No ist die laufende Nummer im Jahr (Belegnummer), chronologisch ab 1.
	No           int
	Entry        model.CashEntry
	BalanceCents int64
	Receipts     []model.CashReceipt
}

// Ledger ist das Kassenbuch einer Gruppenkasse für ein Kalenderjahr.
type Ledger struct {
	GroupName string
	Year      int
	// CarryLabel/CarryCents: Stand zu Beginn („Übertrag aus 2025“ oder „Anfangsbestand“).
	CarryLabel   string
	CarryDate    string
	CarryCents   int64
	Rows         []LedgerRow
	IncomeCents  int64
	ExpenseCents int64
	EndCents     int64
	Summary      Summary
}

// BuildLedger stellt das Kassenbuch für year zusammen. entries müssen chronologisch sortiert sein.
func BuildLedger(groupName string, year int, opening *model.CashOpening, entries []model.CashEntry,
	receipts []model.CashReceipt, summary Summary) Ledger {
	l := Ledger{GroupName: groupName, Year: year, Summary: summary}
	prefix := strconv.Itoa(year)
	l.CarryLabel = fmt.Sprintf("Übertrag aus %d", year-1)
	l.CarryDate = fmt.Sprintf("%d-01-01", year)
	if opening != nil && opening.Date[:4] <= prefix {
		l.CarryCents = opening.CashCents
		if strings.HasPrefix(opening.Date, prefix) {
			l.CarryLabel = "Anfangsbestand"
			l.CarryDate = opening.Date
		}
	}
	byEntry := map[int][]model.CashReceipt{}
	for _, r := range receipts {
		byEntry[r.EntryID] = append(byEntry[r.EntryID], r)
	}
	bal := l.CarryCents
	for _, e := range entries {
		y := e.EntryDate[:4]
		if y < prefix {
			bal += e.SignedCents()
			l.CarryCents = bal
			continue
		}
		if y > prefix {
			break
		}
		bal += e.SignedCents()
		if e.Kind == model.CashExpense {
			l.ExpenseCents += e.AmountCents
		} else {
			l.IncomeCents += e.AmountCents
		}
		l.Rows = append(l.Rows, LedgerRow{No: len(l.Rows) + 1, Entry: e, BalanceCents: bal, Receipts: byEntry[e.ID]})
	}
	l.EndCents = bal
	return l
}

// EntryText ist der Buchungstext einer Buchung (Art und Beschreibung).
func EntryText(e model.CashEntry) string {
	var head string
	switch {
	case e.Kind == model.CashExpense:
		return e.Description
	case e.Source == model.CashSourceAllowance:
		head = "Monatsbetrag " + MonthLabel(e.ForMonth)
	case e.Source == model.CashSourceSavings:
		head = "Entnahme aus dem Ansparkonto"
	default:
		return e.Description
	}
	if e.Description != "" {
		return head + " – " + e.Description
	}
	return head
}

func decimal(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := fmt.Sprintf("%d,%02d", cents/100, cents%100)
	if neg {
		s = "-" + s
	}
	return s
}

// ReceiptName ist der Dateiname des i-ten Belegs einer Zeile im ZIP-Export: „007_2026-04-03_Kassenbon.pdf“,
// bei mehreren Belegen „007a_…“, „007b_…“.
func ReceiptName(row LedgerRow, i int) string {
	suffix := ""
	if len(row.Receipts) > 1 {
		suffix = string(rune('a' + i%26))
	}
	return fmt.Sprintf("%03d%s_%s_%s", row.No, suffix, row.Entry.EntryDate, row.Receipts[i].Filename)
}

// WriteCSV schreibt das Kassenbuch als CSV für Excel (UTF-8 mit BOM, Semikolon, Komma als Dezimaltrennzeichen).
func (l Ledger) WriteCSV(w io.Writer, receiptNames bool) error {
	if _, err := io.WriteString(w, "\uFEFF"); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	if err := cw.Write([]string{"Nr", "Datum", "Buchungstext", "Herkunft", "Einnahme", "Ausgabe", "Kassenstand", "Belege"}); err != nil {
		return err
	}
	_ = cw.Write([]string{"", GermanDate(l.CarryDate), l.CarryLabel, "", "", "", decimal(l.CarryCents), ""})
	for _, r := range l.Rows {
		in, out := "", ""
		if r.Entry.Kind == model.CashExpense {
			out = decimal(r.Entry.AmountCents)
		} else {
			in = decimal(r.Entry.AmountCents)
		}
		names := make([]string, 0, len(r.Receipts))
		for i, rc := range r.Receipts {
			if receiptNames {
				names = append(names, ReceiptName(r, i))
			} else {
				names = append(names, rc.Filename)
			}
		}
		if err := cw.Write([]string{
			strconv.Itoa(r.No), GermanDate(r.Entry.EntryDate), EntryText(r.Entry), SourceLabel(r.Entry),
			in, out, decimal(r.BalanceCents), strings.Join(names, ", "),
		}); err != nil {
			return err
		}
	}
	_ = cw.Write([]string{"", "", "Summe " + strconv.Itoa(l.Year), "", decimal(l.IncomeCents), decimal(l.ExpenseCents), decimal(l.EndCents), ""})
	cw.Flush()
	return cw.Error()
}

// SourceLabel beschreibt die Art einer Buchung.
func SourceLabel(e model.CashEntry) string {
	if e.Kind == model.CashExpense {
		return "Ausgabe"
	}
	switch e.Source {
	case model.CashSourceAllowance:
		return "Monatsbetrag"
	case model.CashSourceSavings:
		return "Ansparkonto"
	default:
		return "Sonstige Einnahme"
	}
}

// PDF erzeugt das Kassenbuch als A4-Querformat mit Ansparkonto-Übersicht des Jahres.
func (l Ledger) PDF(generated time.Time) ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("") // cp1252: Umlaute und €
	title := fmt.Sprintf("Kassenbuch %s %d", l.GroupName, l.Year)
	pdf.SetTitle(tr(title), false)
	pdf.SetAutoPageBreak(true, 12)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("Arial", "", 7)
		pdf.CellFormat(0, 5, tr(fmt.Sprintf("%s · erstellt am %s · Seite %d", title, generated.Format("02.01.2006 15:04"), pdf.PageNo())), "", 0, "R", false, 0, "")
	})
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 8, tr(title), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	w := []float64{12, 22, 98, 30, 25, 25, 27, 38}
	head := []string{"Nr", "Datum", "Buchungstext", "Herkunft", "Einnahme", "Ausgabe", "Stand", "Belege"}
	align := []string{"R", "L", "L", "L", "R", "R", "R", "C"}
	header := func() {
		pdf.SetFont("Arial", "B", 8)
		pdf.SetFillColor(226, 232, 240)
		for i, h := range head {
			pdf.CellFormat(w[i], 6, tr(h), "1", 0, align[i], true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Arial", "", 8)
	}
	row := func(cells []string, bold bool) {
		if pdf.GetY() > 190 {
			pdf.AddPage()
			header()
		}
		if bold {
			pdf.SetFont("Arial", "B", 8)
		}
		for i, c := range cells {
			pdf.CellFormat(w[i], 5.5, tr(fitText(pdf, tr, c, w[i]-2)), "1", 0, align[i], false, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Arial", "", 8)
	}
	euro := func(c int64) string { return FormatEuro(c) }
	header()
	row([]string{"", GermanDate(l.CarryDate), l.CarryLabel, "", "", "", euro(l.CarryCents), ""}, false)
	for _, r := range l.Rows {
		in, out := "", ""
		if r.Entry.Kind == model.CashExpense {
			out = euro(r.Entry.AmountCents)
		} else {
			in = euro(r.Entry.AmountCents)
		}
		rec := ""
		if n := len(r.Receipts); n > 0 {
			rec = strconv.Itoa(n)
		} else if r.Entry.Kind == model.CashExpense {
			rec = "fehlt"
		}
		row([]string{strconv.Itoa(r.No), GermanDate(r.Entry.EntryDate), EntryText(r.Entry), SourceLabel(r.Entry), in, out, euro(r.BalanceCents), rec}, false)
	}
	row([]string{"", "", fmt.Sprintf("Summe %d", l.Year), "", euro(l.IncomeCents), euro(l.ExpenseCents), euro(l.EndCents), ""}, true)

	// Ansparkonto
	pdf.Ln(6)
	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(0, 7, tr("Ansparkonto"), "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	line := fmt.Sprintf("Aktueller Stand: %s", FormatEuro(l.Summary.SavingsCents))
	if l.Summary.OpeningDate != "" {
		line += fmt.Sprintf(" (Anfangsbestand %s am %s)", FormatEuro(l.Summary.OpeningSavingsCents), GermanDate(l.Summary.OpeningDate))
	}
	pdf.CellFormat(0, 6, tr(line), "", 1, "L", false, 0, "")
	mw := []float64{40, 35, 35, 35, 35}
	mh := []string{"Monat", "Anspruch", "Ausgezahlt", "Ins Ansparkonto", "Entnommen"}
	var months []MonthRow
	for _, m := range l.Summary.Months {
		if strings.HasPrefix(m.Month, strconv.Itoa(l.Year)) {
			months = append([]MonthRow{m}, months...)
		}
	}
	if len(months) > 0 {
		if pdf.GetY() > 150 {
			pdf.AddPage()
		}
		pdf.SetFont("Arial", "B", 8)
		for i, h := range mh {
			a := "R"
			if i == 0 {
				a = "L"
			}
			pdf.CellFormat(mw[i], 6, tr(h), "1", 0, a, true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Arial", "", 8)
		for _, m := range months {
			saved := FormatEuro(m.SavedCents)
			name := MonthLabel(m.Month)
			if m.Current {
				saved = "–"
				name += " (läuft)"
			}
			cells := []string{name, FormatEuro(m.AllowanceCents), FormatEuro(m.PaidCents), saved, FormatEuro(m.WithdrawnCents)}
			for i, c := range cells {
				a := "R"
				if i == 0 {
					a = "L"
				}
				pdf.CellFormat(mw[i], 5.5, tr(c), "1", 0, a, false, 0, "")
			}
			pdf.Ln(-1)
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fitText kürzt s, bis es in width passt (Breite gemessen nach cp1252-Übersetzung).
func fitText(pdf *gofpdf.Fpdf, tr func(string) string, s string, width float64) string {
	if pdf.GetStringWidth(tr(s)) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 1 && pdf.GetStringWidth(tr(string(r)+"…")) > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
