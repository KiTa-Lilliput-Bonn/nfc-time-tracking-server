package audit

import (
	"context"
	"log"
	"time"
)

const RetentionDays = 60

// Store persists and queries audit events.
type Store interface {
	Append(ctx context.Context, e Entry) (int64, error)
	List(ctx context.Context, f ListFilter) ([]Event, error)
	ListAllOrdered(ctx context.Context) ([]Event, error)
	ListAnchors(ctx context.Context) ([]Anchor, error)
	PurgeOlderThan(ctx context.Context, before time.Time) error
	Tip(ctx context.Context) (*Tip, error)
}

// Logger records management audit events.
type Logger struct {
	Store Store
}

func (l *Logger) Log(ctx context.Context, e Entry) {
	if l == nil || l.Store == nil {
		return
	}
	if _, err := l.Store.Append(ctx, e); err != nil {
		log.Printf("audit append: %v", err)
	}
}

// LogWithActor fills actor from context when not set on Entry.
func (l *Logger) LogWithActor(ctx context.Context, e Entry) {
	if e.ActorRole == "" {
		uid, role := ActorFromContext(ctx)
		e.ActorUserID = uid
		e.ActorRole = role
	}
	if e.Summary == "" {
		e.Summary = "{}"
	}
	l.Log(ctx, e)
}

// RunRetention purges events older than RetentionDays.
func (l *Logger) RunRetention(ctx context.Context) error {
	if l == nil || l.Store == nil {
		return nil
	}
	before := time.Now().UTC().AddDate(0, 0, -RetentionDays)
	return l.Store.PurgeOlderThan(ctx, before)
}
