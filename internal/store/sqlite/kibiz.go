package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"nfc-time-tracking-server/internal/model"
)

// KibizStore speichert die Daten für die KiBiz-Rechnung im Dienstplan.
type KibizStore struct {
	db *DB
}

func NewKibizStore(db *DB) *KibizStore {
	return &KibizStore{db: db}
}

func (s *KibizStore) ListQualifications(ctx context.Context) (map[int]model.Qualification, error) {
	rows, err := s.db.DB.QueryContext(ctx, `SELECT user_id, qualification FROM staff_qualifications`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]model.Qualification{}
	for rows.Next() {
		var uid int
		var q string
		if err := rows.Scan(&uid, &q); err != nil {
			return nil, err
		}
		out[uid] = model.Qualification(q)
	}
	return out, rows.Err()
}

// SetQualification setzt die Qualifikation; leer entfernt sie.
func (s *KibizStore) SetQualification(ctx context.Context, userID int, q model.Qualification) error {
	if q == "" {
		_, err := s.db.DB.ExecContext(ctx, `DELETE FROM staff_qualifications WHERE user_id = ?`, userID)
		return err
	}
	if !q.Valid() {
		return fmt.Errorf("invalid qualification %q", q)
	}
	_, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO staff_qualifications (user_id, qualification, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET qualification = excluded.qualification, updated_at = excluded.updated_at`,
		userID, string(q), nowText())
	return err
}

func (s *KibizStore) ListRates(ctx context.Context) ([]model.KibizRate, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT group_form, care_hours, children, leitung_hours, total_hours, fachkraft_min_hours FROM kibiz_rates
		 ORDER BY CASE group_form WHEN 'I' THEN 1 WHEN 'II' THEN 2 ELSE 3 END, care_hours`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.KibizRate{}
	for rows.Next() {
		var r model.KibizRate
		var f string
		if err := rows.Scan(&f, &r.CareHours, &r.Children, &r.LeitungHours, &r.TotalHours, &r.FachkraftMinHours); err != nil {
			return nil, err
		}
		r.GroupForm = model.GroupForm(f)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PutRates schreibt die übergebenen Zeilen (Einfügen oder Ersetzen).
func (s *KibizStore) PutRates(ctx context.Context, rates []model.KibizRate) error {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range rates {
		if err := r.Validate(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO kibiz_rates (group_form, care_hours, children, leitung_hours, total_hours, fachkraft_min_hours)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(group_form, care_hours) DO UPDATE SET children = excluded.children,
			   leitung_hours = excluded.leitung_hours, total_hours = excluded.total_hours,
			   fachkraft_min_hours = excluded.fachkraft_min_hours`,
			string(r.GroupForm), r.CareHours, r.Children, r.LeitungHours, r.TotalHours, r.FachkraftMinHours); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func decodeCounts(s string) ([]model.ChildCount, error) {
	out := []model.ChildCount{}
	if s == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("decode child counts: %w", err)
	}
	return out, nil
}

func encodeCounts(c []model.ChildCount) (string, error) {
	if c == nil {
		c = []model.ChildCount{}
	}
	if err := model.ValidateChildCounts(c); err != nil {
		return "", err
	}
	b, err := json.Marshal(c)
	return string(b), err
}

func (s *KibizStore) ListChildPatterns(ctx context.Context) ([]model.ChildPattern, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT group_id, valid_from, counts FROM child_patterns ORDER BY group_id, valid_from`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ChildPattern{}
	for rows.Next() {
		var p model.ChildPattern
		var c string
		if err := rows.Scan(&p.GroupID, &p.ValidFrom, &c); err != nil {
			return nil, err
		}
		if p.Counts, err = decodeCounts(c); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *KibizStore) PutChildPattern(ctx context.Context, p model.ChildPattern) error {
	c, err := encodeCounts(p.Counts)
	if err != nil {
		return err
	}
	_, err = s.db.DB.ExecContext(ctx,
		`INSERT INTO child_patterns (group_id, valid_from, counts, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(group_id, valid_from) DO UPDATE SET counts = excluded.counts, updated_at = excluded.updated_at`,
		p.GroupID, p.ValidFrom, c, nowText())
	return err
}

func (s *KibizStore) DeleteChildPattern(ctx context.Context, groupID int, validFrom string) error {
	_, err := s.db.DB.ExecContext(ctx, `DELETE FROM child_patterns WHERE group_id = ? AND valid_from = ?`, groupID, validFrom)
	return err
}

func (s *KibizStore) ListChildDays(ctx context.Context, from, to string) ([]model.ChildCountDay, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT group_id, day, counts FROM child_count_days WHERE day >= ? AND day <= ? ORDER BY day, group_id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ChildCountDay{}
	for rows.Next() {
		var d model.ChildCountDay
		var c string
		if err := rows.Scan(&d.GroupID, &d.Date, &c); err != nil {
			return nil, err
		}
		if d.Counts, err = decodeCounts(c); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PutChildDays setzt dieselbe Kinderliste für mehrere Tage einer Gruppe.
func (s *KibizStore) PutChildDays(ctx context.Context, groupID int, dates []string, counts []model.ChildCount) error {
	c, err := encodeCounts(counts)
	if err != nil {
		return err
	}
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := nowText()
	for _, d := range dates {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO child_count_days (group_id, day, counts, updated_at) VALUES (?, ?, ?, ?)
			 ON CONFLICT(group_id, day) DO UPDATE SET counts = excluded.counts, updated_at = excluded.updated_at`,
			groupID, d, c, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteChildDays setzt die Tage einer Gruppe auf das Muster zurück.
func (s *KibizStore) DeleteChildDays(ctx context.Context, groupID int, dates []string) error {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, d := range dates {
		if _, err := tx.ExecContext(ctx, `DELETE FROM child_count_days WHERE group_id = ? AND day = ?`, groupID, d); err != nil {
			return err
		}
	}
	return tx.Commit()
}
