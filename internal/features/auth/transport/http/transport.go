package auth_transport_http

import (
	http_middleware "github.com/wrzdx/Nero/internal/core/transport/http/middleware"

	"github.com/go-chi/chi/v5"
)

type AuthHandler struct {
	authService   AuthService
	cookieManager CookieManager
}

func NewAuthHTTPHandler(
	authService AuthService,
	cookieManager CookieManager,
) *AuthHandler {
	return &AuthHandler{
		cookieManager: cookieManager,
		authService:   authService,
	}
}

func (h *AuthHandler) Router(authMW http_middleware.Middleware) chi.Router {
	router := chi.NewRouter()
	router.Post("/login", h.Login)
	router.Post("/register", h.Register)
	router.Post("/logout", h.Logout)
	router.Post("/refresh", h.Refresh)
	router.Group(func(protected chi.Router) {
		protected.Use(authMW)
		protected.Put("/password", h.ChangePassword)
	})

	return router
}
