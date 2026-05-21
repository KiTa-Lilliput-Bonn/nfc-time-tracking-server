package sqlite

import (
	"context"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/model"
)

type PunchStore struct {
	db *DB
}

func NewPunchStore(db *DB) *PunchStore {
	return &PunchStore{db: db}
}

// InsertBatch inserts punches; duplicates (same punch_time + nfc_tag_uid) are ignored.
func (s *PunchStore) InsertBatch(ctx context.Context, punches []model.RawPunch) (int, error) {
	if len(punches) == 0 {
		return 0, nil
	}
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO raw_punches (punch_time, nfc_tag_uid, source_file, device_name, imported_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	inserted := 0
	for _, p := range punches {
		imp := p.ImportedAt
		if imp.IsZero() {
			imp = time.Now().UTC()
		}
		res, err := stmt.ExecContext(ctx, p.PunchTime.UTC().Format(time.RFC3339Nano), p.NFCTagUID, p.SourceFile, p.DeviceName, imp.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return inserted, fmt.Errorf("insert punch: %w", err)
		}
		n, _ := res.RowsAffected()
		inserted += int(n)
	}
	if err := tx.Commit(); err != nil {
		return inserted, err
	}
	return inserted, nil
}

// ListByUserAndDate returns raw punches for user on date (YYYY-MM-DD) via NFC tag assignment overlap.
func (s *PunchStore) ListByUserAndDate(ctx context.Context, userID int, date string) ([]model.RawPunch, error) {
	q := `
SELECT DISTINCT rp.id, rp.punch_time, rp.nfc_tag_uid, rp.source_file, rp.device_name, rp.imported_at
FROM raw_punches rp
INNER JOIN nfc_tags nt ON nt.tag_uid = rp.nfc_tag_uid
WHERE nt.user_id = ?
  AND date(rp.punch_time) = ?
  AND nt.assigned_from = (
    SELECT MAX(nt2.assigned_from) FROM nfc_tags nt2
    WHERE nt2.tag_uid = rp.nfc_tag_uid AND nt2.assigned_from <= date(rp.punch_time)
  )
ORDER BY rp.punch_time ASC`
	rows, err := s.db.DB.QueryContext(ctx, q, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.RawPunch
	for rows.Next() {
		var p model.RawPunch
		var pt, imp string
		if err := rows.Scan(&p.ID, &pt, &p.NFCTagUID, &p.SourceFile, &p.DeviceName, &imp); err != nil {
			return nil, err
		}
		p.PunchTime, _ = time.Parse(time.RFC3339Nano, pt)
		if t2, err2 := time.Parse(time.RFC3339, pt); err2 == nil && p.PunchTime.IsZero() {
			p.PunchTime = t2
		}
		p.ImportedAt, _ = time.Parse(time.RFC3339Nano, imp)
		if t2, err2 := time.Parse(time.RFC3339, imp); err2 == nil && p.ImportedAt.IsZero() {
			p.ImportedAt = t2
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByUTCDateForLanSync returns raw punches for utcDate (YYYY-MM-DD in UTC, same convention as date(rp.punch_time)) for all active users.
func (s *PunchStore) ListByUTCDateForLanSync(ctx context.Context, utcDate string) ([]model.LanSyncPunch, error) {
	q := `
SELECT DISTINCT rp.id, rp.punch_time, rp.nfc_tag_uid, rp.source_file, rp.device_name, rp.imported_at, nt.user_id
FROM raw_punches rp
INNER JOIN nfc_tags nt ON nt.tag_uid = rp.nfc_tag_uid
INNER JOIN users u ON u.id = nt.user_id AND u.active = 1
WHERE date(rp.punch_time) = ?
  AND nt.assigned_from = (
    SELECT MAX(nt2.assigned_from) FROM nfc_tags nt2
    WHERE nt2.tag_uid = rp.nfc_tag_uid AND nt2.assigned_from <= date(rp.punch_time)
  )
ORDER BY rp.punch_time ASC`
	rows, err := s.db.DB.QueryContext(ctx, q, utcDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.LanSyncPunch
	for rows.Next() {
		var p model.RawPunch
		var uid int
		var pt, imp string
		if err := rows.Scan(&p.ID, &pt, &p.NFCTagUID, &p.SourceFile, &p.DeviceName, &imp, &uid); err != nil {
			return nil, err
		}
		p.PunchTime, _ = time.Parse(time.RFC3339Nano, pt)
		if t2, err2 := time.Parse(time.RFC3339, pt); err2 == nil && p.PunchTime.IsZero() {
			p.PunchTime = t2
		}
		p.ImportedAt, _ = time.Parse(time.RFC3339Nano, imp)
		if t2, err2 := time.Parse(time.RFC3339, imp); err2 == nil && p.ImportedAt.IsZero() {
			p.ImportedAt = t2
		}
		out = append(out, model.LanSyncPunch{UserID: uid, Punch: p})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
