package audit

import (
	"context"

	apimw "nfc-time-tracking-server/internal/api/middleware"
)

const RoleSystem = "system"

// ActorFromContext reads the JWT actor from request context.
func ActorFromContext(ctx context.Context) (userID *int, role string) {
	v := ctx.Value(apimw.CtxUserID)
	if v == nil {
		return nil, RoleSystem
	}
	id := v.(int)
	if id == 0 {
		return nil, RoleSystem
	}
	role = RoleSystem
	if r := ctx.Value(apimw.CtxRole); r != nil {
		if s, ok := r.(string); ok && s != "" {
			role = s
		}
	}
	return &id, role
}
