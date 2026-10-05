package web_handlers

import (
	"encoding/json"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	"net/http"
)

func (h *AppHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.SearchUsers(r.Context(), requester(r), users_service.SearchUsersQuery{Prefix: r.URL.Query().Get("q"), Limit: 10})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		status, message := publicError(err)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
		return
	}
	type item struct {
		ID        string  `json:"id"`
		Username  string  `json:"username"`
		FirstName string  `json:"first_name"`
		LastName  *string `json:"last_name"`
	}
	response := struct {
		Users []item `json:"users"`
	}{Users: make([]item, 0, len(users))}
	for _, user := range users {
		response.Users = append(response.Users, item{user.ID.String(), user.Username, user.FirstName, user.LastName})
	}
	_ = json.NewEncoder(w).Encode(response)
}
