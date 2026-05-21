package handler

import (
	"context"
	"strconv"

	"nfc-time-tracking-server/internal/audit"
)

func logAudit(l *audit.Logger, ctx context.Context, e audit.Entry) {
	if l != nil {
		l.LogWithActor(ctx, e)
	}
}

func auditID(id int) string {
	return strconv.Itoa(id)
}

func auditTarget(id int) *int {
	return &id
}
