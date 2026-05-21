package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"nfc-time-tracking-server/internal/model"
)

type GroupStore struct {
	db *DB
}

func NewGroupStore(db *DB) *GroupStore {
	return &GroupStore{db: db}
}

func (s *GroupStore) List(ctx context.Context) ([]model.Group, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, name, sort_order, created_at, updated_at FROM user_groups ORDER BY sort_order ASC, name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Group
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

func (s *GroupStore) GetByID(ctx context.Context, id int) (*model.Group, error) {
	g := &model.Group{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, name, sort_order, created_at, updated_at FROM user_groups WHERE id = ?`, id).
		Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("group not found: %d", id)
	}
	return g, err
}

func (s *GroupStore) Create(ctx context.Context, g *model.Group) error {
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO user_groups (name, sort_order) VALUES (?, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM user_groups))`,
		g.Name)
	if err != nil {
		return fmt.Errorf("insert group: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	g.ID = int(id)
	return s.db.DB.QueryRowContext(ctx,
		`SELECT sort_order, created_at, updated_at FROM user_groups WHERE id = ?`, g.ID).
		Scan(&g.SortOrder, &g.CreatedAt, &g.UpdatedAt)
}

func (s *GroupStore) Update(ctx context.Context, g *model.Group) error {
	res, err := s.db.DB.ExecContext(ctx,
		`UPDATE user_groups SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		g.Name, g.ID)
	if err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("group not found: %d", g.ID)
	}
	return s.db.DB.QueryRowContext(ctx,
		`SELECT updated_at FROM user_groups WHERE id = ?`, g.ID).Scan(&g.UpdatedAt)
}

func (s *GroupStore) Delete(ctx context.Context, id int) error {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE users SET group_id = NULL WHERE group_id = ?`, id); err != nil {
		return fmt.Errorf("clear users group: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM user_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("group not found: %d", id)
	}
	return tx.Commit()
}

func (s *GroupStore) Reorder(ctx context.Context, ids []int) error {
	var count int
	if err := s.db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_groups`).Scan(&count); err != nil {
		return fmt.Errorf("count groups: %w", err)
	}
	if count == 0 {
		if len(ids) == 0 {
			return nil
		}
		return fmt.Errorf("invalid order")
	}
	if len(ids) != count {
		return fmt.Errorf("must include every group exactly once")
	}
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			return fmt.Errorf("duplicate group id")
		}
		seen[id] = struct{}{}
	}

	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, id := range ids {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_groups WHERE id = ?`, id).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("unknown group id")
		}
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE user_groups SET sort_order = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			i, id); err != nil {
			return fmt.Errorf("reorder: %w", err)
		}
	}
	return tx.Commit()
}
