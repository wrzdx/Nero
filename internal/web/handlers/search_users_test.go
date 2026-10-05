package web_handlers

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	"net/http/httptest"
	"testing"
)

type searchUsersStub struct {
	WebUsers
	search func(uuid.UUID, users_service.SearchUsersQuery) ([]users_service.UserSearchResult, error)
}

func (s searchUsersStub) SearchUsers(_ context.Context, id uuid.UUID, q users_service.SearchUsersQuery) ([]users_service.UserSearchResult, error) {
	return s.search(id, q)
}

func TestHTMLUserSearchUsesAuthenticatedRequesterAndFixedLimit(t *testing.T) {
	id := uuid.New()
	h := NewAppHandler(nil, nil, searchUsersStub{search: func(requester uuid.UUID, q users_service.SearchUsersQuery) ([]users_service.UserSearchResult, error) {
		require.Equal(t, id, requester)
		require.Equal(t, users_service.SearchUsersQuery{Prefix: "@Da", Limit: 10}, q)
		return []users_service.UserSearchResult{{ID: uuid.New(), Username: "daniel_k", FirstName: "Даниил"}}, nil
	}}, nil, nil, nil)
	w := httptest.NewRecorder()
	h.SearchUsers(w, appRequest(httptest.NewRequest("GET", "/app/users/search?q=%40Da&limit=999&requester_id="+uuid.NewString(), nil), id, nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Contains(t, w.Body.String(), `"username":"daniel_k"`)
	require.NotContains(t, w.Body.String(), "password")
}
