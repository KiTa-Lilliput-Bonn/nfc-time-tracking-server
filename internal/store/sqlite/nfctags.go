package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

type NFCTagStore struct {
	db *DB
}

func NewNFCTagStore(db *DB) *NFCTagStore {
	return &NFCTagStore{db: db}
}

func (s *NFCTagStore) Assign(ctx context.Context, tag *model.NFCTag) error {
	var role string
	err := s.db.DB.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, tag.UserID).Scan(&role)
	if err == sql.ErrNoRows {
		return fmt.Errorf("user not found: %d", tag.UserID)
	}
	if err != nil {
		return err
	}
	if !model.RoleMayHaveNFCTag(model.Role(role)) {
		return fmt.Errorf("nfc tags may only be assigned to role user or leitung")
	}

	if err := s.checkTagConflict(ctx, tag.TagUID, tag.UserID, tag.AssignedFrom); err != nil {
		return err
	}

	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO nfc_tags (tag_uid, user_id, assigned_from) VALUES (?, ?, ?)`,
		tag.TagUID, tag.UserID, tag.AssignedFrom)
	if err != nil {
		return fmt.Errorf("assign nfc tag: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	tag.ID = int(id)
	return nil
}

func (s *NFCTagStore) checkTagConflict(ctx context.Context, tagUID string, assigneeUserID int, assignedFrom string) error {
	var ownerID int
	var ownerFrom string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT user_id, assigned_from FROM nfc_tags WHERE tag_uid = ? AND assigned_from <= ?
		 ORDER BY assigned_from DESC LIMIT 1`,
		tagUID, assignedFrom).Scan(&ownerID, &ownerFrom)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if ownerID == assigneeUserID {
		return nil
	}
	var displayName string
	var active int
	err = s.db.DB.QueryRowContext(ctx,
		`SELECT display_name, active FROM users WHERE id = ?`, ownerID).Scan(&displayName, &active)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if active != 1 {
		return nil
	}
	return &store.NFCTagAssignedError{DisplayName: displayName, AssignedFrom: ownerFrom}
}

func (s *NFCTagStore) GetActiveByTagUID(ctx context.Context, tagUID string) (*model.NFCTag, error) {
	t := &model.NFCTag{}
	today := time.Now().UTC().Format("2006-01-02")
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, tag_uid, user_id, assigned_from FROM nfc_tags
		 WHERE tag_uid = ? AND assigned_from <= ?
		 ORDER BY assigned_from DESC LIMIT 1`, tagUID, today).
		Scan(&t.ID, &t.TagUID, &t.UserID, &t.AssignedFrom)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *NFCTagStore) ListByUser(ctx context.Context, userID int) ([]model.NFCTag, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, tag_uid, user_id, assigned_from FROM nfc_tags WHERE user_id = ? ORDER BY assigned_from DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.NFCTag
	for rows.Next() {
		var t model.NFCTag
		if err := rows.Scan(&t.ID, &t.TagUID, &t.UserID, &t.AssignedFrom); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *NFCTagStore) ListActiveUserIDsWithOpenNFCTag(ctx context.Context) ([]int, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT DISTINCT n.user_id FROM nfc_tags n
		 INNER JOIN users u ON u.id = n.user_id
		 WHERE u.role IN (?, ?) AND u.active = 1
		 ORDER BY n.user_id`,
		string(model.RoleUser), string(model.RoleLeitung))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *NFCTagStore) LatestOpenTagUID(ctx context.Context, userID int) (string, error) {
	today := time.Now().UTC().Format("2006-01-02")
	var uid string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT tag_uid FROM nfc_tags
		 WHERE user_id = ? AND assigned_from <= ?
		 ORDER BY assigned_from DESC LIMIT 1`,
		userID, today).Scan(&uid)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return uid, nil
}

func (s *NFCTagStore) ResolveUserID(ctx context.Context, tagUID string, at time.Time) (int, error) {
	d := at.Format("2006-01-02")
	var uid int
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT user_id FROM nfc_tags WHERE tag_uid = ? AND assigned_from <= ?
		 ORDER BY assigned_from DESC LIMIT 1`,
		tagUID, d).Scan(&uid)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("no assignment for tag %s at %s", tagUID, d)
	}
	if err != nil {
		return 0, err
	}
	return uid, nil
}

// TagUIDForUserAt returns the tag_uid for userID valid on at's UTC calendar date.
func (s *NFCTagStore) TagUIDForUserAt(ctx context.Context, userID int, at time.Time) (string, error) {
	d := at.UTC().Format("2006-01-02")
	var tagUID string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT tag_uid FROM nfc_tags WHERE user_id = ? AND assigned_from <= ?
		 ORDER BY assigned_from DESC LIMIT 1`,
		userID, d).Scan(&tagUID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(tagUID), nil
}
