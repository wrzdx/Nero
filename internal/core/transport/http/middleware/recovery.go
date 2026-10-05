package http_middleware

import (
	"fmt"
	logger "github.com/wrzdx/Nero/internal/core/logger"
	http_errmap "github.com/wrzdx/Nero/internal/core/transport/http/errmap"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	"net/http"
)

func Recovery() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			log := logger.FromContext(ctx)
			sender := http_response.NewHTTPSender(log, w, http_errmap.Map)
			defer func() {
				if p := recover(); p != nil {
					sender.Error(fmt.Errorf("during handle HTTP request got unexpected panic: %v", p))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
