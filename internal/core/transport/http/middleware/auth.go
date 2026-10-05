package http_middleware

import (
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/auth"
	core_context "github.com/wrzdx/Nero/internal/core/context"
	logger "github.com/wrzdx/Nero/internal/core/logger"
	http_errmap "github.com/wrzdx/Nero/internal/core/transport/http/errmap"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	"net/http"
	"strings"
)

func extractToken(r *http.Request) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", false
	}

	parts := strings.Fields(authHeader)
	if len(parts) != 2 {
		return "", false
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}

	return parts[1], true
}

func authErrorMapper(err error) http_response.HTTPError {
	switch {
	case errors.Is(err, auth.ErrInvalidToken),
		errors.Is(err, auth.ErrInvalidClaims):
		return http_response.HTTPError{
			StatusCode: http.StatusUnauthorized,
			Code:       "invalid_token",
			Message:    "invalid token",
		}
	default:
		return http_errmap.Map(err)
	}
}

type TokenProvider interface {
	ParseAccessToken(tokenStr string) (auth.ParsedAccessToken, error)
}

func Auth(jwt TokenProvider) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log := logger.FromContext(r.Context())
			sender := http_response.NewHTTPSender(log, w, authErrorMapper)
			token, ok := extractToken(r)
			if !ok {
				sender.Error(auth.ErrInvalidToken)
				return
			}

			aClaims, err := jwt.ParseAccessToken(token)
			if err != nil {
				sender.Error(fmt.Errorf("failed to authenticate: %w", err))
				return
			}
			appClaims := core_context.ContextClaims{
				UserID: aClaims.UserID,
			}
			ctx := core_context.WithClaims(r.Context(), appClaims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
