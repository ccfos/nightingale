package a2a

import (
	"context"

	"github.com/ccfos/nightingale/v6/models"
)

type userCtxKey struct{}

// WithUser stores the authenticated user in ctx so the AgentExecutor can pick
// it up after the gin middleware chain finishes.
func WithUser(ctx context.Context, user *models.User) context.Context {
	return context.WithValue(ctx, userCtxKey{}, user)
}

// UserFromContext returns the user attached to ctx, or nil if none is present.
func UserFromContext(ctx context.Context) *models.User {
	u, _ := ctx.Value(userCtxKey{}).(*models.User)
	return u
}
