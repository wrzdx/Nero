package users_transport_http

import (
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

func (h *UsersHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sender := http_response.NewHTTPSender(logger.FromContext(ctx), w, errorMapper)
	w.Header().Set("Cache-Control", "no-store")
	query := users_service.SearchUsersQuery{Prefix: r.URL.Query().Get("q")}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			sender.Error(http_request.NewFieldError(map[string]string{"limit": "invalid limit"}))
			return
		}
		query.Limit = limit
	}
	users, err := h.usersService.SearchUsers(ctx, core_context.ClaimsRequired(ctx).UserID, query)
	if err != nil {
		sender.Error(err)
		return
	}
	response := SearchUsersResponse{Users: make([]UserSearchResponse, 0, len(users))}
	for _, user := range users {
		response.Users = append(response.Users, UserSearchResponse{user.ID, user.Username, user.FirstName, user.LastName})
	}
	sender.OK(http.StatusOK, response)
}

type UserSearchResponse struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	FirstName string    `json:"first_name"`
	LastName  *string   `json:"last_name"`
}
type SearchUsersResponse struct {
	Users []UserSearchResponse `json:"users"`
}
