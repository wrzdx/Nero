package users_transport_http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func newUsersTransportTestUser(t *testing.T) domain.User {
	t.Helper()

	lastName := "Anderson"
	bio := "Transport test user"
	profile, err := domain.NewUserProfile("Username_1", "Elliot", &lastName, &bio)
	require.NoError(t, err)
	user, err := domain.NewUser(
		uuid.New(),
		profile,
		time.Now().UTC(),
		nil,
		"secret-password-hash",
	)
	require.NoError(t, err)
	return user
}

func newUsersTransportRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()

	request := httptest.NewRequest(method, target, nil)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}

func newGetUserRequest(t *testing.T, userID string) *http.Request {
	t.Helper()

	request := newUsersTransportRequest(t, http.MethodGet, "/users/"+userID)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", userID)
	ctx := context.WithValue(request.Context(), chi.RouteCtxKey, routeContext)
	return request.WithContext(ctx)
}

func decodeUsersTransportData(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) UserDTOResponse {
	t.Helper()

	var body struct {
		Data UserDTOResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Data
}

func decodeUsersTransportError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) http_response.APIErrorDetail {
	t.Helper()

	var body struct {
		Error http_response.APIErrorDetail `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Error
}
