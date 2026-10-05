package core_context

import (
	"context"
	logger "github.com/wrzdx/Nero/internal/core/logger"

	"github.com/google/uuid"
)

type ContextClaims struct {
	UserID uuid.UUID
}

type ctxKeyClaims struct{}

var (
	key = ctxKeyClaims{}
)

func WithClaims(ctx context.Context, claims ContextClaims) context.Context {
	return context.WithValue(ctx, ctxKeyClaims{}, claims)
}

func ClaimsFromCtx(ctx context.Context) (ContextClaims, bool) {
	claims, ok := ctx.Value(ctxKeyClaims{}).(ContextClaims)
	return claims, ok
}

func ClaimsRequired(ctx context.Context) ContextClaims {
	claims, ok := ClaimsFromCtx(ctx)
	if !ok {
		log := logger.FromContext(ctx)
		log.Error("no claims found in context")
		return ContextClaims{}
	}
	return claims
}
