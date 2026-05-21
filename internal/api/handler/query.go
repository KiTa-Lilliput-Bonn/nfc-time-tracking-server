package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func queryDateRange(r *http.Request) (from, to string, err error) {
	from = r.URL.Query().Get("from")
	to = r.URL.Query().Get("to")
	if from == "" || to == "" {
		return "", "", fmt.Errorf("from and to are required (YYYY-MM-DD)")
	}
	if _, e := time.Parse("2006-01-02", from); e != nil {
		return "", "", fmt.Errorf("invalid from")
	}
	if _, e := time.Parse("2006-01-02", to); e != nil {
		return "", "", fmt.Errorf("invalid to")
	}
	return from, to, nil
}

func queryMonthYear(r *http.Request) (year, month int, err error) {
	my := r.URL.Query().Get("month")
	yr := r.URL.Query().Get("year")
	if my == "" || yr == "" {
		return 0, 0, fmt.Errorf("month and year are required")
	}
	m, e1 := strconv.Atoi(my)
	y, e2 := strconv.Atoi(yr)
	if e1 != nil || e2 != nil || m < 1 || m > 12 {
		return 0, 0, fmt.Errorf("invalid month or year")
	}
	return y, m, nil
}
