package vacationentitlement

import (
	"fmt"
	"strings"
	"time"
)

// NormalizeVacationEntDate kürzt DB-/ISO-Strings auf YYYY-MM-DD (wie bei Urlaubsvergleichen).
func NormalizeVacationEntDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

// ParseValidFromCalendarDay prüft, dass valid_from ein gültiger Kalendertag ist (YYYY-MM-DD).
func ParseValidFromCalendarDay(validFrom string) (string, error) {
	s := NormalizeVacationEntDate(validFrom)
	if s == "" {
		return "", fmt.Errorf("valid_from is required")
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return "", fmt.Errorf("valid_from: invalid date")
	}
	return t.Format("2006-01-02"), nil
}
