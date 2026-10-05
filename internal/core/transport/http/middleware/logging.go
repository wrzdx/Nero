package http_middleware

import (
	logger "github.com/wrzdx/Nero/internal/core/logger"
	"net/http"

	"go.uber.org/zap"
)

func Logging(log *logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(requestIDHeader)

			l := log.With(
				zap.String("request_id", requestID),
				zap.String("method", r.Method),
				zap.String("url", r.URL.String()),
			)

			ctx := logger.WithLogger(r.Context(), l)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
