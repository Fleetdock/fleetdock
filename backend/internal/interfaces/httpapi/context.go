package httpapi

import (
	"context"

	authapp "github.com/mariadb-cp/db-manager/backend/internal/app/auth"
)

type ctxKey int

const principalCtxKey ctxKey = iota

func withPrincipal(ctx context.Context, p *authapp.Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey, p)
}

func principalFrom(ctx context.Context) *authapp.Principal {
	p, _ := ctx.Value(principalCtxKey).(*authapp.Principal)
	return p
}
