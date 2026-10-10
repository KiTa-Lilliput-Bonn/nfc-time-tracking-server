package model

import (
	"fmt"
	"strings"
	"time"
)

// RoleGroupAccount ist die Rolle im Token eines Gruppenaccounts. Gruppenaccounts stehen nicht in der
// Benutzertabelle, haben keine Arbeitszeiten und sehen nur die Anwesenheitsliste ihrer Gruppe.
const RoleGroupAccount = "gruppe"

// Child ist ein Kind in einer Gruppe (für die Anwesenheitsliste). Inaktive Kinder (abgemeldet)
// erscheinen nicht mehr in der Liste; nach der Löschfrist ab DeactivatedAt werden sie samt Daten gelöscht.
type Child struct {
	ID        int    `json:"id"`
	GroupID   int    `json:"group_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Active    bool   `json:"active"`
	// DeactivatedAt: Zeitpunkt der Abmeldung (RFC 3339, UTC); nil solange aktiv.
	DeactivatedAt *string `json:"deactivated_at"`
}

// Validate normalisiert die Namen und prüft Pflichtfelder.
func (c *Child) Validate() error {
	c.FirstName = strings.TrimSpace(c.FirstName)
	c.LastName = strings.TrimSpace(c.LastName)
	if c.FirstName == "" {
		return fmt.Errorf("first_name required")
	}
	if len(c.FirstName) > 80 || len(c.LastName) > 80 {
		return fmt.Errorf("name too long")
	}
	if c.GroupID <= 0 {
		return fmt.Errorf("group_id required")
	}
	return nil
}

// ChildAttendance hält Kommen und Gehen eines Kindes an einem Tag (HH:MM, Ortszeit). nil = noch nicht.
type ChildAttendance struct {
	ChildID   int     `json:"child_id"`
	Date      string  `json:"date"`
	ArrivedAt *string `json:"arrived_at"`
	LeftAt    *string `json:"left_at"`
}

// Validate prüft die Uhrzeiten; Gehen ohne Kommen und Gehen vor Kommen sind nicht erlaubt.
func (a *ChildAttendance) Validate() error {
	for _, p := range []**string{&a.ArrivedAt, &a.LeftAt} {
		if *p == nil {
			continue
		}
		v := strings.TrimSpace(**p)
		if v == "" {
			*p = nil
			continue
		}
		if !ValidClock(v) {
			return fmt.Errorf("invalid time %q", v)
		}
		*p = &v
	}
	if a.LeftAt != nil && a.ArrivedAt == nil {
		return fmt.Errorf("left_at requires arrived_at")
	}
	if a.LeftAt != nil && *a.LeftAt < *a.ArrivedAt {
		return fmt.Errorf("left_at before arrived_at")
	}
	return nil
}

// ValidClock prüft eine Uhrzeit im Format HH:MM.
func ValidClock(s string) bool {
	if len(s) != 5 {
		return false
	}
	_, err := time.Parse("15:04", s)
	return err == nil
}

// ChildNoticeReason ist der Grund für ein vorab gemeldetes Fehlen.
type ChildNoticeReason string

const (
	ChildNoticeVacation ChildNoticeReason = "vacation"
	ChildNoticeSick     ChildNoticeReason = "sick"
	ChildNoticeOther    ChildNoticeReason = "other"
)

func (r ChildNoticeReason) Valid() bool {
	return r == ChildNoticeVacation || r == ChildNoticeSick || r == ChildNoticeOther
}

// ChildNotice meldet vorab, dass ein Kind an Tagen fehlt (ohne Uhrzeiten) oder nur teilweise da ist:
// ArriveFrom = wird erst um … gebracht, LeaveAt = wird schon um … abgeholt.
type ChildNotice struct {
	ID         int               `json:"id"`
	ChildID    int               `json:"child_id"`
	DateFrom   string            `json:"date_from"`
	DateTo     string            `json:"date_to"`
	Reason     ChildNoticeReason `json:"reason"`
	ArriveFrom *string           `json:"arrive_from"`
	LeaveAt    *string           `json:"leave_at"`
	Note       string            `json:"note"`
}

// FullDay ist true, wenn das Kind an den Tagen gar nicht kommt.
func (n *ChildNotice) FullDay() bool {
	return n.ArriveFrom == nil && n.LeaveAt == nil
}

// Validate normalisiert und prüft eine Meldung.
func (n *ChildNotice) Validate() error {
	if !n.Reason.Valid() {
		return fmt.Errorf("invalid reason %q", n.Reason)
	}
	for _, d := range []string{n.DateFrom, n.DateTo} {
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return fmt.Errorf("invalid date %q", d)
		}
	}
	if n.DateTo < n.DateFrom {
		return fmt.Errorf("date_to before date_from")
	}
	from, _ := time.Parse("2006-01-02", n.DateFrom)
	to, _ := time.Parse("2006-01-02", n.DateTo)
	if to.Sub(from) > 366*24*time.Hour {
		return fmt.Errorf("range too long")
	}
	for _, p := range []**string{&n.ArriveFrom, &n.LeaveAt} {
		if *p == nil {
			continue
		}
		v := strings.TrimSpace(**p)
		if v == "" {
			*p = nil
			continue
		}
		if !ValidClock(v) {
			return fmt.Errorf("invalid time %q", v)
		}
		*p = &v
	}
	if n.ArriveFrom != nil && n.LeaveAt != nil && *n.LeaveAt <= *n.ArriveFrom {
		return fmt.Errorf("leave_at must be after arrive_from")
	}
	n.Note = strings.TrimSpace(n.Note)
	if len(n.Note) > 500 {
		return fmt.Errorf("note too long")
	}
	return nil
}

// GroupAccount ist das Konto eines Gruppengeräts (z. B. Tablet im Flur).
type GroupAccount struct {
	ID             int    `json:"id"`
	GroupID        int    `json:"group_id"`
	Username       string `json:"username"`
	PasswordHash   string `json:"-"`
	Active         bool   `json:"active"`
	SessionVersion int    `json:"-"`
}

// AttendanceStaffAccess sagt, ob eine Person die Anwesenheitslisten aller Gruppen sieht: Leitung und
// Admin immer, sonst pädagogisches Personal (Kraft Fachkraft, Ergänzungskraft oder Leitung). Ohne
// eingetragene Kraft zählt die Person dazu; Hauswirtschaft und Sonstige nicht.
func AttendanceStaffAccess(role Role, q Qualification, hasQ bool) bool {
	if role == RoleLeitung || role == RoleSuperadmin {
		return true
	}
	if role != RoleUser {
		return false
	}
	if !hasQ {
		return true
	}
	return q == QualificationFachkraft || q == QualificationErgaenzungskraft || q == QualificationLeitung
}
