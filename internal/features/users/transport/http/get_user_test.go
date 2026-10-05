package users_transport_http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wrzdx/Nero/internal/core/domain"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetUser(t *testing.T) {
	t.Run("returns user", func(t *testing.T) {
		user := newUsersTransportTestUser(t)
		service := NewMockUsersService(t)
		service.EXPECT().
			GetUser(mock.Anything, user.ID).
			Return(user, nil)
		cookieManger := NewMockCookieManager(t)
		handler := NewUsersHandler(service, cookieManger)
		request := newGetUserRequest(t, user.ID.String())
		recorder := httptest.NewRecorder()

		handler.GetUser(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, userDTOFromDomain(user), decodeUsersTransportData(t, recorder))
		require.NotContains(t, recorder.Body.String(), user.PasswordHash)
	})

	t.Run("rejects malformed id without calling service", func(t *testing.T) {
		service := NewMockUsersService(t)
		cookieManger := NewMockCookieManager(t)
		handler := NewUsersHandler(service, cookieManger)
		request := newGetUserRequest(t, "not-a-uuid")
		recorder := httptest.NewRecorder()

		handler.GetUser(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"id": "invalid user uuid",
			},
		}, decodeUsersTransportError(t, recorder))
	})

	t.Run("treats nil uuid as an unknown user", func(t *testing.T) {
		service := NewMockUsersService(t)
		service.EXPECT().
			GetUser(mock.Anything, uuid.Nil).
			Return(domain.User{}, domain.ErrNotFound)
		cookieManger := NewMockCookieManager(t)
		handler := NewUsersHandler(service, cookieManger)
		request := newGetUserRequest(t, uuid.Nil.String())
		recorder := httptest.NewRecorder()

		handler.GetUser(recorder, request)

		require.Equal(t, http.StatusNotFound, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "user_not_found",
			Message: "user not found",
		}, decodeUsersTransportError(t, recorder))
	})

	t.Run("returns not found", func(t *testing.T) {
		userID := uuid.New()
		service := NewMockUsersService(t)
		service.EXPECT().
			GetUser(mock.Anything, userID).
			Return(domain.User{}, domain.ErrNotFound)
		cookieManger := NewMockCookieManager(t)
		handler := NewUsersHandler(service, cookieManger)
		request := newGetUserRequest(t, userID.String())
		recorder := httptest.NewRecorder()

		handler.GetUser(recorder, request)

		require.Equal(t, http.StatusNotFound, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "user_not_found",
			Message: "user not found",
		}, decodeUsersTransportError(t, recorder))
	})

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		userID := uuid.New()
		serviceErr := errors.New("database unavailable")
		service := NewMockUsersService(t)
		service.EXPECT().
			GetUser(mock.Anything, userID).
			Return(domain.User{}, serviceErr)
		cookieManger := NewMockCookieManager(t)
		handler := NewUsersHandler(service, cookieManger)
		request := newGetUserRequest(t, userID.String())
		recorder := httptest.NewRecorder()

		handler.GetUser(recorder, request)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeUsersTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}
