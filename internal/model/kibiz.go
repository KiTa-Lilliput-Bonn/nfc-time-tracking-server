package model

import "fmt"

// Qualification ist die Einstufung einer Person für die KiBiz-Rechnung (Personalkraftstunden).
type Qualification string

const (
	QualificationFachkraft        Qualification = "fachkraft"
	QualificationErgaenzungskraft Qualification = "ergaenzungskraft"
	// QualificationSonstige: z. B. Praktikum, Auszubildende; zählt nicht zu Fach- oder Ergänzungskraftstunden.
	QualificationSonstige Qualification = "sonstige"
)

func (q Qualification) Valid() bool {
	switch q {
	case QualificationFachkraft, QualificationErgaenzungskraft, QualificationSonstige:
		return true
	}
	return false
}

// GroupForm ist die KiBiz-Gruppenform: I = 2 Jahre bis Schule, II = unter 3, III = ab 3 Jahren.
type GroupForm string

const (
	GroupFormI   GroupForm = "I"
	GroupFormII  GroupForm = "II"
	GroupFormIII GroupForm = "III"
)

func (f GroupForm) Valid() bool {
	return f == GroupFormI || f == GroupFormII || f == GroupFormIII
}

// ValidCareHours sind die Betreuungszeiten nach KiBiz (Stunden pro Woche).
func ValidCareHours(h int) bool {
	return h == 25 || h == 35 || h == 45
}

// KibizRate ist eine Zeile der KiBiz-Tabelle (Anlage zu § 33 KiBiz) für eine Gruppenform und
// Betreuungszeit, je Gruppe und Woche: Kinderzahl, Leitungsstunden, Gesamtpersonalkraftstunden und
// davon mindestens Fachkraftstunden. Den Rest dürfen Ergänzungskräfte (oder Fachkräfte) abdecken.
// Vorbelegt mit den Werten ab 01.08.2020, änderbar.
type KibizRate struct {
	GroupForm         GroupForm `json:"group_form"`
	CareHours         int       `json:"care_hours"`
	Children          float64   `json:"children"`
	LeitungHours      float64   `json:"leitung_hours"`
	TotalHours        float64   `json:"total_hours"`
	FachkraftMinHours float64   `json:"fachkraft_min_hours"`
}

func (r KibizRate) Validate() error {
	if !r.GroupForm.Valid() {
		return fmt.Errorf("invalid group_form %q", r.GroupForm)
	}
	if !ValidCareHours(r.CareHours) {
		return fmt.Errorf("invalid care_hours %d", r.CareHours)
	}
	if r.Children <= 0 || r.Children > 100 {
		return fmt.Errorf("children must be between 0 and 100")
	}
	for _, h := range []float64{r.LeitungHours, r.TotalHours, r.FachkraftMinHours} {
		if h < 0 || h > 1000 {
			return fmt.Errorf("hours must be between 0 and 1000")
		}
	}
	if r.FachkraftMinHours > r.TotalHours {
		return fmt.Errorf("fachkraft_min_hours must not exceed total_hours")
	}
	return nil
}

// ChildCount ist die Anzahl Kinder einer Gruppenform mit einer gebuchten Betreuungszeit (ohne Namen).
type ChildCount struct {
	GroupForm GroupForm `json:"group_form"`
	CareHours int       `json:"care_hours"`
	Count     int       `json:"count"`
}

// ValidateChildCounts prüft eine Kinderliste; doppelte Kombinationen sind nicht erlaubt.
func ValidateChildCounts(counts []ChildCount) error {
	seen := map[string]bool{}
	for _, c := range counts {
		if !c.GroupForm.Valid() {
			return fmt.Errorf("invalid group_form %q", c.GroupForm)
		}
		if !ValidCareHours(c.CareHours) {
			return fmt.Errorf("invalid care_hours %d", c.CareHours)
		}
		if c.Count < 0 || c.Count > 200 {
			return fmt.Errorf("count must be between 0 and 200")
		}
		k := fmt.Sprintf("%s/%d", c.GroupForm, c.CareHours)
		if seen[k] {
			return fmt.Errorf("duplicate entry %s", k)
		}
		seen[k] = true
	}
	return nil
}

// ChildPattern ist das feste Wochenmuster einer Gruppe ab valid_from (gilt Mo–Fr bis zur nächsten Version).
type ChildPattern struct {
	GroupID   int          `json:"group_id"`
	ValidFrom string       `json:"valid_from"`
	Counts    []ChildCount `json:"counts"`
}

// ChildCountDay ersetzt das Muster einer Gruppe an einem einzelnen Tag (Abweichung für Tag oder Woche).
type ChildCountDay struct {
	GroupID int          `json:"group_id"`
	Date    string       `json:"date"`
	Counts  []ChildCount `json:"counts"`
}

// ChildPatternForDate liefert das an date gültige Muster (größtes valid_from ≤ date) oder nil.
func ChildPatternForDate(patterns []ChildPattern, groupID int, date string) *ChildPattern {
	var best *ChildPattern
	for i := range patterns {
		p := &patterns[i]
		if p.GroupID != groupID || p.ValidFrom > date {
			continue
		}
		if best == nil || p.ValidFrom > best.ValidFrom {
			best = p
		}
	}
	return best
}
