package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/audit"
)

type AuditStore struct {
	db *DB
}

func NewAuditStore(db *DB) *AuditStore {
	return &AuditStore{db: db}
}

func (s *AuditStore) Append(ctx context.Context, e audit.Entry) (int64, error) {
	if e.Summary == "" {
		e.Summary = "{}"
	}
	if e.ActorRole == "" {
		e.ActorRole = audit.RoleSystem
	}

	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var prevHash []byte
	err = tx.QueryRowContext(ctx,
		`SELECT event_hash FROM audit_events ORDER BY id DESC LIMIT 1`,
	).Scan(&prevHash)
	if err == sql.ErrNoRows {
		prevHash = audit.GenesisHash[:]
	} else if err != nil {
		return 0, err
	}

	var nextID int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) + 1 FROM audit_events`).Scan(&nextID); err != nil {
		return 0, err
	}

	createdAt := time.Now().UTC()
	ev := audit.Event{
		ID:           nextID,
		CreatedAt:    createdAt,
		ActorUserID:  e.ActorUserID,
		ActorRole:    e.ActorRole,
		Action:       e.Action,
		EntityType:   e.EntityType,
		EntityID:     e.EntityID,
		TargetUserID: e.TargetUserID,
		Summary:      e.Summary,
		PrevHash:     prevHash,
	}
	canonical := audit.Canonical(ev)
	eventHash := audit.ComputeEventHash(prevHash, canonical)
	ev.EventHash = eventHash[:]

	var actorID sql.NullInt64
	if e.ActorUserID != nil {
		actorID = sql.NullInt64{Int64: int64(*e.ActorUserID), Valid: true}
	}
	var targetID sql.NullInt64
	if e.TargetUserID != nil {
		targetID = sql.NullInt64{Int64: int64(*e.TargetUserID), Valid: true}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_events (
			id, created_at, actor_user_id, actor_role, action, entity_type, entity_id,
			target_user_id, summary, prev_hash, event_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, createdAt.Format(time.RFC3339Nano), actorID, ev.ActorRole, ev.Action,
		ev.EntityType, ev.EntityID, targetID, ev.Summary, ev.PrevHash, ev.EventHash,
	)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return nextID, nil
}

func (s *AuditStore) List(ctx context.Context, f audit.ListFilter) ([]audit.Event, error) {
	q := `SELECT id, created_at, actor_user_id, actor_role, action, entity_type, entity_id,
		target_user_id, summary, prev_hash, event_hash FROM audit_events WHERE 1=1`
	var args []any
	if f.From != nil {
		q += ` AND created_at >= ?`
		args = append(args, f.From.UTC().Format(time.RFC3339Nano))
	}
	if f.To != nil {
		q += ` AND created_at <= ?`
		args = append(args, f.To.UTC().Format(time.RFC3339Nano))
	}
	if f.EntityType != "" {
		q += ` AND entity_type = ?`
		args = append(args, f.EntityType)
	}
	if f.ActorUserID != nil {
		q += ` AND actor_user_id = ?`
		args = append(args, *f.ActorUserID)
	}
	if f.TargetUserID != nil {
		q += ` AND target_user_id = ?`
		args = append(args, *f.TargetUserID)
	}
	q += ` ORDER BY id DESC`
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	q += ` LIMIT ? OFFSET ?`
	args = append(args, limit, f.Offset)
	return s.queryEvents(ctx, q, args...)
}

func (s *AuditStore) ListAllOrdered(ctx context.Context) ([]audit.Event, error) {
	return s.queryEvents(ctx,
		`SELECT id, created_at, actor_user_id, actor_role, action, entity_type, entity_id,
			target_user_id, summary, prev_hash, event_hash FROM audit_events ORDER BY id ASC`)
}

func (s *AuditStore) ListAnchors(ctx context.Context) ([]audit.Anchor, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, anchored_at, last_deleted_id, last_event_hash FROM audit_anchors ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []audit.Anchor
	for rows.Next() {
		var a audit.Anchor
		var at string
		if err := rows.Scan(&a.ID, &at, &a.LastDeletedID, &a.LastEventHash); err != nil {
			return nil, err
		}
		a.AnchoredAt, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			a.AnchoredAt, _ = time.Parse("2006-01-02 15:04:05", at)
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *AuditStore) PurgeOlderThan(ctx context.Context, before time.Time) error {
	cutoff := before.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var lastID sql.NullInt64
	var lastHash []byte
	err = tx.QueryRowContext(ctx,
		`SELECT id, event_hash FROM audit_events WHERE created_at < ? ORDER BY id DESC LIMIT 1`,
		cutoff,
	).Scan(&lastID, &lastHash)
	if err == sql.ErrNoRows {
		return tx.Commit()
	}
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_anchors (anchored_at, last_deleted_id, last_event_hash) VALUES (?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339Nano), lastID.Int64, lastHash,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE audit_retention_guard SET allow_purge = 1 WHERE id = 1`,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM audit_events WHERE created_at < ?`, cutoff,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE audit_retention_guard SET allow_purge = 0 WHERE id = 1`,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *AuditStore) Tip(ctx context.Context) (*audit.Tip, error) {
	var id int64
	var hash []byte
	var at string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, event_hash, created_at FROM audit_events ORDER BY id DESC LIMIT 1`,
	).Scan(&id, &hash, &at)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		t, _ = time.Parse("2006-01-02 15:04:05", at)
	}
	return &audit.Tip{LastID: id, EventHash: hash, WrittenAt: t}, nil
}

func (s *AuditStore) Verify(ctx context.Context) audit.VerifyResult {
	events, err := s.ListAllOrdered(ctx)
	if err != nil {
		return audit.VerifyResult{OK: false, GenesisOK: false}
	}
	return audit.VerifyChain(events)
}

func (s *AuditStore) queryEvents(ctx context.Context, query string, args ...any) ([]audit.Event, error) {
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []audit.Event
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, ev)
	}
	return list, rows.Err()
}

type eventScanner interface {
	Scan(dest ...any) error
}

func scanEvent(rows eventScanner) (audit.Event, error) {
	var ev audit.Event
	var createdAt string
	var actorID, targetID sql.NullInt64
	if err := rows.Scan(
		&ev.ID, &createdAt, &actorID, &ev.ActorRole, &ev.Action, &ev.EntityType, &ev.EntityID,
		&targetID, &ev.Summary, &ev.PrevHash, &ev.EventHash,
	); err != nil {
		return ev, err
	}
	var err error
	ev.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		ev.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return ev, fmt.Errorf("parse created_at: %w", err)
		}
	}
	if actorID.Valid {
		v := int(actorID.Int64)
		ev.ActorUserID = &v
	}
	if targetID.Valid {
		v := int(targetID.Int64)
		ev.TargetUserID = &v
	}
	return ev, nil
}
