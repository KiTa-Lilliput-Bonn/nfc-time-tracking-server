package scheduleimport

import "strings"

// ImportScope steuert, welche Kalendertage beim Excel-Import verarbeitet werden.
type ImportScope int

const (
	// ImportScopeFuture: nur date >= today (Default); Vergangenheit wird gezählt/übersprungen.
	ImportScopeFuture ImportScope = iota
	// ImportScopePastOnly: nur date < today (zweiter Lauf nach Nutzer-Bestätigung).
	ImportScopePastOnly
	// ImportScopeAll: kein Datumsfilter (Tests).
	ImportScopeAll
)

// ParseImportScope parst multipart/query-Werte: leer, "future", "past", "all".
func ParseImportScope(s string) ImportScope {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "past":
		return ImportScopePastOnly
	case "all":
		return ImportScopeAll
	default:
		return ImportScopeFuture
	}
}

func dateInImportScope(date, todayLocal string, scope ImportScope) bool {
	if date == "" {
		return false
	}
	switch scope {
	case ImportScopeAll:
		return true
	case ImportScopePastOnly:
		return date < todayLocal
	default:
		return date >= todayLocal
	}
}

func weekNotesInImportScope(w ParsedWeek, todayLocal string, scope ImportScope) (apply bool, countPastSkipped bool) {
	if strings.TrimSpace(w.Notes) == "" {
		return false, false
	}
	fullyPast := weekFullyPast(w, todayLocal)
	switch scope {
	case ImportScopePastOnly:
		return fullyPast, false
	case ImportScopeAll:
		return true, false
	default:
		if fullyPast {
			return false, true
		}
		return true, false
	}
}

func teamMeetingsInImportScope(monday, todayLocal string, scope ImportScope) bool {
	if monday == "" {
		return false
	}
	switch scope {
	case ImportScopePastOnly:
		return monday < todayLocal
	case ImportScopeAll:
		return true
	default:
		return monday >= todayLocal
	}
}
