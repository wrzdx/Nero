package users_transport_http

import (
	"encoding/json"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	core_context "github.com/wrzdx/Nero/internal/core/context"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchUsersRouteReturnsOnlyPublicSelectionFields(t *testing.T) {
	user := newUsersTransportTestUser(t)
	service := NewMockUsersService(t)
	service.EXPECT().SearchUsers(mock.Anything, user.ID, users_service.SearchUsersQuery{Prefix: "dan", Limit: 5}).Return([]users_service.UserSearchResult{{ID: user.ID, Username: user.Profile.Username, FirstName: user.Profile.FirstName, LastName: user.Profile.LastName}}, nil)
	h := NewUsersHandler(service, nil)
	router := h.Router(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(core_context.WithClaims(r.Context(), core_context.ContextClaims{UserID: user.ID})))
		})
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newUsersTransportRequest(t, "GET", "/search?q=dan&limit=5"))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var response struct {
		Data SearchUsersResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data.Users, 1)
	require.Equal(t, user.Profile.Username, response.Data.Users[0].Username)
	require.NotContains(t, w.Body.String(), "password")
	require.NotContains(t, w.Body.String(), "bio")
}

func TestSearchUsersRejectsMalformedLimit(t *testing.T) {
	user := newUsersTransportTestUser(t)
	r := newUsersTransportRequest(t, "GET", "/search?q=dan&limit=nope")
	r = r.WithContext(core_context.WithClaims(r.Context(), core_context.ContextClaims{UserID: user.ID}))
	w := httptest.NewRecorder()
	NewUsersHandler(NewMockUsersService(t), nil).SearchUsers(w, r)
	require.Equal(t, 400, w.Code)
}
