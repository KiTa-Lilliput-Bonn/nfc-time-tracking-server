package model

import "time"

// CashEntryKind unterscheidet Ausgaben und Einnahmen einer Gruppenkasse.
type CashEntryKind string

const (
	CashExpense CashEntryKind = "expense"
	CashIncome  CashEntryKind = "income"
)

// CashIncomeSource sagt, woher eine Einnahme kommt. Bei Ausgaben leer (aus der Kasse) oder
// CashSourceSavings (direkt aus dem Ansparkonto bezahlt).
type CashIncomeSource string

const (
	// CashSourceAllowance ist die Auszahlung des Monatsanspruchs (z. B. vom Finanzvorstand) für ForMonth.
	CashSourceAllowance CashIncomeSource = "allowance"
	// CashSourceSavings ist bei Einnahmen eine Entnahme aus dem fiktiven Ansparkonto in die Kasse,
	// bei Ausgaben eine Zahlung direkt aus dem Ansparkonto (ohne Umweg über die Kasse).
	CashSourceSavings CashIncomeSource = "savings"
	// CashSourceOther ist jede andere Einnahme (z. B. Spende); sie berührt das Ansparkonto nicht.
	CashSourceOther CashIncomeSource = "other"
)

// CashAllowance ist der monatliche Anspruch einer Gruppenkasse ab ValidFrom (YYYY-MM).
type CashAllowance struct {
	ID          int       `json:"id"`
	GroupID     int       `json:"group_id"`
	ValidFrom   string    `json:"valid_from"`
	AmountCents int64     `json:"amount_cents"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CashEntry ist eine Buchung (Ausgabe oder Einnahme) einer Gruppenkasse. Beträge sind positiv in Cent.
type CashEntry struct {
	ID          int              `json:"id"`
	GroupID     int              `json:"group_id"`
	Kind        CashEntryKind    `json:"kind"`
	Source      CashIncomeSource `json:"source"`
	ForMonth    string           `json:"for_month"`
	EntryDate   string           `json:"entry_date"`
	AmountCents int64            `json:"amount_cents"`
	Description string           `json:"description"`
	CreatedBy   *int             `json:"created_by,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedBy   *int             `json:"updated_by,omitempty"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// PaidFromSavings: Ausgabe direkt aus dem Ansparkonto; sie ändert den Kassenstand nicht.
func (e CashEntry) PaidFromSavings() bool {
	return e.Kind == CashExpense && e.Source == CashSourceSavings
}

// SignedCents liefert den Betrag mit Vorzeichen für den Kassenstand.
func (e CashEntry) SignedCents() int64 {
	if e.PaidFromSavings() {
		return 0
	}
	if e.Kind == CashExpense {
		return -e.AmountCents
	}
	return e.AmountCents
}

// CashReceipt beschreibt einen Beleg (ohne Dateiinhalt).
type CashReceipt struct {
	ID          int       `json:"id"`
	EntryID     int       `json:"entry_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	UploadedBy  *int      `json:"uploaded_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CashOpening ist der Anfangsbestand einer Gruppenkasse zum Stichtag Date (YYYY-MM-DD):
// Bargeld in der Kasse und Guthaben im Ansparkonto, die vor der Erfassung in der App bestanden.
type CashOpening struct {
	GroupID      int       `json:"group_id"`
	Date         string    `json:"date"`
	CashCents    int64     `json:"cash_cents"`
	SavingsCents int64     `json:"savings_cents"`
	UpdatedBy    *int      `json:"updated_by,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}
