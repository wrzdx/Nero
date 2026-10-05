package users_transport_http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	http_middleware "github.com/wrzdx/Nero/internal/core/transport/http/middleware"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsersRoutesUseAuthMiddleware(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "get current user", method: http.MethodGet, path: "/me"},
		{name: "patch current user", method: http.MethodPatch, path: "/me"},
		{name: "delete current user", method: http.MethodDelete, path: "/me"},
		{name: "get user by id", method: http.MethodGet, path: "/" + uuid.NewString()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewMockUsersService(t)
			cookieManger := NewMockCookieManager(t)
			handler := NewUsersHandler(service, cookieManger)
			middlewareCalled := false
			authMW := http_middleware.Middleware(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					middlewareCalled = true
					w.WriteHeader(http.StatusUnauthorized)
				})
			})
			request := newUsersTransportRequest(t, tt.method, tt.path)
			recorder := httptest.NewRecorder()

			handler.Router(authMW).ServeHTTP(recorder, request)

			require.True(t, middlewareCalled)
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}
