package model

type DayResult struct {
	Date         string  `json:"date"`
	NetWorkHours float64 `json:"net_work_hours"`
	TargetHours  float64 `json:"target_hours"`
	IsHoliday    bool    `json:"is_holiday"`
	IsClosureDay bool    `json:"is_closure_day"`
	AbsenceType  *string `json:"absence_type"`
	HalfDay      bool    `json:"half_day"`
	IsWeekend    bool    `json:"is_weekend"`
}

type MonthBalance struct {
	Year         int     `json:"year"`
	Month        int     `json:"month"`
	WorkedHours  float64 `json:"worked_hours"`
	TargetHours  float64 `json:"target_hours"`
	BalanceHours float64 `json:"balance_hours"`
	Carryover    float64 `json:"carryover"`
	TotalBalance float64 `json:"total_balance"`
	// AccountStart ist der erste Tag des Stundenkontos (YYYY-MM-DD).
	AccountStart string `json:"account_start"`
	// CountedFrom/CountedThrough: tatsächlich gezählter Zeitraum dieses Monats (leer, wenn nichts gezählt).
	CountedFrom    string `json:"counted_from"`
	CountedThrough string `json:"counted_through"`
	// IsPartial: laufender Monat, gezählt nur bis gestern. IsFuture: Monat beginnt nach gestern.
	IsPartial bool `json:"is_partial"`
	IsFuture  bool `json:"is_future"`
}

// VacationBalance: Urlaubsrechnung (siehe vacationbalance.Compute).
// Gesamt = Startsaldo + Übertrag + Anspruch; Rest = Gesamt − genommen; frei = Rest − geplant.
type VacationBalance struct {
	Year int `json:"year"`
	// CarriedOver ist der Startsaldo aus dem Import (users.opening_vacation_days).
	CarriedOver float64 `json:"carried_over"`
	// Carryover ist der Übertrag aus den Vorjahren.
	Carryover   float64 `json:"carryover"`
	Entitlement float64 `json:"entitlement"`
	Total       float64 `json:"total"`
	Taken       float64 `json:"taken"`
	Remaining   float64 `json:"remaining"`
	Planned     float64 `json:"planned"`
	Free        float64 `json:"free"`
}
