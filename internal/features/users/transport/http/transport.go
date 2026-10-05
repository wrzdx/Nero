package users_transport_http

import (
	http_middleware "github.com/wrzdx/Nero/internal/core/transport/http/middleware"

	"github.com/go-chi/chi/v5"
)

type UsersHandler struct {
	usersService  UsersService
	cookieManager CookieManager
}

func NewUsersHandler(
	usersService UsersService,
	cookieManager CookieManager,
) *UsersHandler {
	return &UsersHandler{
		usersService:  usersService,
		cookieManager: cookieManager,
	}
}

func (h *UsersHandler) Router(authMW http_middleware.Middleware) chi.Router {
	router := chi.NewRouter()
	router.Use(authMW)
	router.Get("/search", h.SearchUsers)
	router.Get("/me", h.GetMe)
	router.Patch("/me", h.PatchMe)
	router.Delete("/me", h.DeleteMe)
	router.Get("/{id}", h.GetUser)

	return router
}
