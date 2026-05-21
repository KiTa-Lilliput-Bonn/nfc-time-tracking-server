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
}

type VacationBalance struct {
	Year           int     `json:"year"`
	Entitlement    float64 `json:"entitlement"`
	Carryover      float64 `json:"carryover"`
	Taken          float64 `json:"taken"`
	Planned        float64 `json:"planned"`
	Remaining      float64 `json:"remaining"`
	CarriedOver    float64 `json:"carried_over"`
}
