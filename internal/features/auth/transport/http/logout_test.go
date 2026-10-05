package auth_transport_http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wrzdx/Nero/internal/core/auth"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLogout(t *testing.T) {
	t.Run("deletes server session, clears cookie and returns no content", func(t *testing.T) {
		service := &logoutServiceSpy{}
		cookieManager := NewMockCookieManager(t)
		cookieManager.EXPECT().
			GetRefreshToken(mock.Anything).
			Return("refresh-token", nil)
		cookieManager.EXPECT().ClearRefreshToken(mock.Anything).Return()
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/logout",
			nil,
		)
		recorder := httptest.NewRecorder()

		handler.Logout(recorder, req)

		require.Equal(t, 1, service.calls)
		require.Equal(t, "refresh-token", service.token)
		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("treats missing cookie as idempotent success", func(t *testing.T) {
		service := &logoutServiceSpy{}
		cookieManager := NewMockCookieManager(t)
		cookieManager.EXPECT().
			GetRefreshToken(mock.Anything).
			Return("", auth.ErrInvalidToken)
		cookieManager.EXPECT().ClearRefreshToken(mock.Anything).Return()
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/logout",
			nil,
		)
		recorder := httptest.NewRecorder()

		handler.Logout(recorder, req)

		require.Zero(t, service.calls)
		require.Equal(t, http.StatusNoContent, recorder.Code)
	})

	t.Run("treats invalid service token as idempotent success", func(t *testing.T) {
		service := &logoutServiceSpy{err: auth.ErrInvalidToken}
		cookieManager := NewMockCookieManager(t)
		cookieManager.EXPECT().
			GetRefreshToken(mock.Anything).
			Return("invalid-refresh-token", nil)
		cookieManager.EXPECT().ClearRefreshToken(mock.Anything).Return()
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/logout",
			nil,
		)
		recorder := httptest.NewRecorder()

		handler.Logout(recorder, req)

		require.Equal(t, 1, service.calls)
		require.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	t.Run("returns internal error without clearing cookie when logout fails", func(t *testing.T) {
		serviceErr := errors.New("database unavailable")
		service := &logoutServiceSpy{err: serviceErr}
		cookieManager := NewMockCookieManager(t)
		cookieManager.EXPECT().
			GetRefreshToken(mock.Anything).
			Return("refresh-token", nil)
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/logout",
			nil,
		)
		recorder := httptest.NewRecorder()

		handler.Logout(recorder, req)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeAuthTransportError(t, recorder))
	})
}

type logoutServiceSpy struct {
	AuthService
	calls int
	token string
	err   error
}

func (s *logoutServiceSpy) Logout(_ context.Context, token string) error {
	s.calls++
	s.token = token
	return s.err
}
