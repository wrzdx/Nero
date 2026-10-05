package auth_transport_http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLogin(t *testing.T) {
	t.Run("returns access token and sets refresh cookie", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		tokens := auth.TokenPair{
			Access:  "access-token",
			Refresh: "refresh-token",
		}
		service.EXPECT().
			Login(mock.Anything, "Username_1", "valid password value").
			Return(tokens, nil)
		cookieManager.EXPECT().
			SetRefreshToken(mock.Anything, tokens.Refresh).
			Return()
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/login",
			map[string]string{
				"username": "Username_1",
				"password": "valid password value",
			},
		)
		recorder := httptest.NewRecorder()

		handler.Login(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code)
		var body struct {
			Data LoginResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, tokens.Access, body.Data.Access)
		require.NotContains(t, recorder.Body.String(), `"success"`)
	})

	t.Run("returns invalid credentials without setting refresh cookie", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		service.EXPECT().
			Login(mock.Anything, "Username_1", "wrong password").
			Return(auth.TokenPair{}, auth.ErrInvalidCredentials)
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/login",
			map[string]string{
				"username": "Username_1",
				"password": "wrong password",
			},
		)
		recorder := httptest.NewRecorder()

		handler.Login(recorder, req)

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_credentials",
			Message: "invalid credentials",
		}, decodeAuthTransportError(t, recorder))
	})

	t.Run("returns validation fields without calling service", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/login",
			map[string]string{},
		)
		recorder := httptest.NewRecorder()

		handler.Login(recorder, req)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"username": "username is required",
				"password": "password is required",
			},
		}, decodeAuthTransportError(t, recorder))
	})

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		serviceErr := errors.New("database unavailable")
		service.EXPECT().
			Login(mock.Anything, "Username_1", "valid password value").
			Return(auth.TokenPair{}, serviceErr)
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/login",
			map[string]string{
				"username": "Username_1",
				"password": "valid password value",
			},
		)
		recorder := httptest.NewRecorder()

		handler.Login(recorder, req)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeAuthTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}

func newAuthTransportRequest(
	t *testing.T,
	method string,
	target string,
	body any,
) *http.Request {
	t.Helper()

	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, target, bytes.NewReader(encoded))
	return req.WithContext(logger.WithLogger(req.Context(), logger.NewTestLogger()))
}

func decodeAuthTransportError(
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

func decodeAuthTransportResponse(
	recorder *httptest.ResponseRecorder,
	destination any,
) error {
	return json.Unmarshal(recorder.Body.Bytes(), destination)
}
