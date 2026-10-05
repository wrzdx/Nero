package auth_transport_http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wrzdx/Nero/internal/core/auth"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRefresh(t *testing.T) {
	t.Run("returns new access token and rotates refresh cookie", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		tokens := auth.TokenPair{
			Access:  "new-access-token",
			Refresh: "new-refresh-token",
		}
		cookieManager.EXPECT().
			GetRefreshToken(mock.Anything).
			Return("old-refresh-token", nil)
		service.EXPECT().
			Refresh(mock.Anything, "old-refresh-token").
			Return(tokens, nil)
		cookieManager.EXPECT().
			SetRefreshToken(mock.Anything, tokens.Refresh).
			Return()
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/refresh",
			nil,
		)
		recorder := httptest.NewRecorder()

		handler.Refresh(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code)
		var body struct {
			Data struct {
				AccessToken string `json:"access_token"`
			} `json:"data"`
		}
		require.NoError(t, decodeAuthTransportResponse(recorder, &body))
		require.Equal(t, tokens.Access, body.Data.AccessToken)
		require.NotContains(t, recorder.Body.String(), `"success"`)
	})

	t.Run("returns invalid token when refresh cookie is missing", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		cookieManager.EXPECT().
			GetRefreshToken(mock.Anything).
			Return("", auth.ErrInvalidToken)
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/refresh",
			nil,
		)
		recorder := httptest.NewRecorder()

		handler.Refresh(recorder, req)

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_token",
			Message: "invalid token",
		}, decodeAuthTransportError(t, recorder))
	})

	t.Run("does not rotate cookie when service rejects token", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		cookieManager.EXPECT().
			GetRefreshToken(mock.Anything).
			Return("refresh-token", nil)
		service.EXPECT().
			Refresh(mock.Anything, "refresh-token").
			Return(auth.TokenPair{}, auth.ErrInvalidToken)
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/refresh",
			nil,
		)
		recorder := httptest.NewRecorder()

		handler.Refresh(recorder, req)

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		require.Equal(t, "invalid_token", decodeAuthTransportError(t, recorder).Code)
	})

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		serviceErr := errors.New("database unavailable")
		cookieManager.EXPECT().
			GetRefreshToken(mock.Anything).
			Return("refresh-token", nil)
		service.EXPECT().
			Refresh(mock.Anything, "refresh-token").
			Return(auth.TokenPair{}, serviceErr)
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/refresh",
			nil,
		)
		recorder := httptest.NewRecorder()

		handler.Refresh(recorder, req)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeAuthTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}
