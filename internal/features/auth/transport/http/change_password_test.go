package auth_transport_http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wrzdx/Nero/internal/core/auth"
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/domain"
	http_middleware "github.com/wrzdx/Nero/internal/core/transport/http/middleware"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestChangePassword(t *testing.T) {
	const (
		currentPassword = "current password value"
		newPassword     = "new password value"
	)

	t.Run("changes password, clears refresh cookie and returns no content", func(t *testing.T) {
		userID := uuid.New()
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		service.EXPECT().
			ChangePassword(mock.Anything, userID, currentPassword, newPassword).
			Return(nil)
		cookieManager.EXPECT().ClearRefreshToken(mock.Anything).Return()
		handler := NewAuthHTTPHandler(service, cookieManager)
		request := newChangePasswordRequest(t, userID, map[string]string{
			"current_password": currentPassword,
			"new_password":     newPassword,
		})
		recorder := httptest.NewRecorder()

		handler.ChangePassword(recorder, request)

		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.Empty(t, recorder.Body.Bytes())
	})

	t.Run("returns request validation fields without calling service", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		handler := NewAuthHTTPHandler(service, cookieManager)
		request := newChangePasswordRequest(t, uuid.New(), map[string]string{})
		recorder := httptest.NewRecorder()

		handler.ChangePassword(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"current_password": "current_password is required",
				"new_password":     "new_password is required",
			},
		}, decodeAuthTransportError(t, recorder))
	})

	t.Run("returns current password mismatch without clearing cookie", func(t *testing.T) {
		userID := uuid.New()
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		serviceErr := domain.DetailedError{
			Err: auth.ErrPasswordMismatch,
			Details: map[string]string{
				"current_password": auth.ErrPasswordMismatch.Error(),
			},
		}
		service.EXPECT().
			ChangePassword(mock.Anything, userID, currentPassword, newPassword).
			Return(serviceErr)
		handler := NewAuthHTTPHandler(service, cookieManager)
		request := newChangePasswordRequest(t, userID, map[string]string{
			"current_password": currentPassword,
			"new_password":     newPassword,
		})
		recorder := httptest.NewRecorder()

		handler.ChangePassword(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "password_mismatch",
			Message: "password mismatch",
			Fields: map[string]string{
				"current_password": auth.ErrPasswordMismatch.Error(),
			},
		}, decodeAuthTransportError(t, recorder))
	})

	t.Run("returns new password validation details without clearing cookie", func(t *testing.T) {
		userID := uuid.New()
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		serviceErr := domain.DetailedError{
			Err: auth.ErrInvalidPassword,
			Details: map[string]string{
				"new_password": "password must be at least 15 characters",
			},
		}
		service.EXPECT().
			ChangePassword(mock.Anything, userID, currentPassword, "too short").
			Return(serviceErr)
		handler := NewAuthHTTPHandler(service, cookieManager)
		request := newChangePasswordRequest(t, userID, map[string]string{
			"current_password": currentPassword,
			"new_password":     "too short",
		})
		recorder := httptest.NewRecorder()

		handler.ChangePassword(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_password",
			Message: "invalid password",
			Fields: map[string]string{
				"new_password": "password must be at least 15 characters",
			},
		}, decodeAuthTransportError(t, recorder))
	})

	t.Run("clears refresh cookie and returns unauthorized for invalid token", func(t *testing.T) {
		userID := uuid.New()
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		service.EXPECT().
			ChangePassword(mock.Anything, userID, currentPassword, newPassword).
			Return(auth.ErrInvalidToken)
		cookieManager.EXPECT().ClearRefreshToken(mock.Anything).Return()
		handler := NewAuthHTTPHandler(service, cookieManager)
		request := newChangePasswordRequest(t, userID, map[string]string{
			"current_password": currentPassword,
			"new_password":     newPassword,
		})
		recorder := httptest.NewRecorder()

		handler.ChangePassword(recorder, request)

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_token",
			Message: "invalid token",
		}, decodeAuthTransportError(t, recorder))
	})

	t.Run("does not clear cookie or expose unexpected service error", func(t *testing.T) {
		userID := uuid.New()
		serviceErr := errors.New("database unavailable")
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		service.EXPECT().
			ChangePassword(mock.Anything, userID, currentPassword, newPassword).
			Return(serviceErr)
		handler := NewAuthHTTPHandler(service, cookieManager)
		request := newChangePasswordRequest(t, userID, map[string]string{
			"current_password": currentPassword,
			"new_password":     newPassword,
		})
		recorder := httptest.NewRecorder()

		handler.ChangePassword(recorder, request)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeAuthTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}

func TestChangePasswordRouteUsesAuthMiddleware(t *testing.T) {
	service := NewMockAuthService(t)
	cookieManager := NewMockCookieManager(t)
	handler := NewAuthHTTPHandler(service, cookieManager)
	middlewareCalled := false
	authMW := http_middleware.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middlewareCalled = true
			w.WriteHeader(http.StatusUnauthorized)
		})
	})
	request := newAuthTransportRequest(
		t,
		http.MethodPut,
		"/password",
		map[string]string{},
	)
	recorder := httptest.NewRecorder()

	handler.Router(authMW).ServeHTTP(recorder, request)

	require.True(t, middlewareCalled)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func newChangePasswordRequest(
	t *testing.T,
	userID uuid.UUID,
	body any,
) *http.Request {
	t.Helper()
	request := newAuthTransportRequest(t, http.MethodPut, "/auth/password", body)
	ctx := core_context.WithClaims(request.Context(), core_context.ContextClaims{UserID: userID})
	return request.WithContext(ctx)
}
