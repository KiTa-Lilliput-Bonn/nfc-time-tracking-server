package model

import "time"

// Group ist eine von der Leitung verwaltbare Benutzergruppe (max. eine Gruppe pro Benutzer).
type Group struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
