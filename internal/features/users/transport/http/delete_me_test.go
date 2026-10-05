package users_transport_http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDeleteMe(t *testing.T) {
	t.Run("deletes current account, clears refresh cookie and returns no content", func(t *testing.T) {
		userID := uuid.New()
		service := NewMockUsersService(t)
		cookieManager := NewMockCookieManager(t)
		handler := NewUsersHandler(service, cookieManager)
		request := newDeleteMeRequest(t, userID)
		recorder := httptest.NewRecorder()
		service.EXPECT().DeleteAccount(mock.Anything, userID).Return(nil)
		cookieManager.EXPECT().ClearRefreshToken(recorder).Return()

		handler.DeleteMe(recorder, request)

		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.Empty(t, recorder.Body.Bytes())
		require.Empty(t, recorder.Header().Get("Content-Type"))
	})

	t.Run("maps missing account without clearing refresh cookie", func(t *testing.T) {
		userID := uuid.New()
		service := NewMockUsersService(t)
		cookieManager := NewMockCookieManager(t)
		handler := NewUsersHandler(service, cookieManager)
		request := newDeleteMeRequest(t, userID)
		recorder := httptest.NewRecorder()
		service.EXPECT().
			DeleteAccount(mock.Anything, userID).
			Return(domain.ErrNotFound)

		handler.DeleteMe(recorder, request)

		require.Equal(t, http.StatusNotFound, recorder.Code)
		require.Equal(t, "user_not_found", decodeUsersTransportError(t, recorder).Code)
	})

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		userID := uuid.New()
		service := NewMockUsersService(t)
		cookieManager := NewMockCookieManager(t)
		handler := NewUsersHandler(service, cookieManager)
		request := newDeleteMeRequest(t, userID)
		recorder := httptest.NewRecorder()
		service.EXPECT().
			DeleteAccount(mock.Anything, userID).
			Return(errors.New("database unavailable"))

		handler.DeleteMe(recorder, request)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		responseError := decodeUsersTransportError(t, recorder)
		require.Equal(t, "internal_error", responseError.Code)
		require.NotContains(t, recorder.Body.String(), "database unavailable")
	})
}

func newDeleteMeRequest(t *testing.T, userID uuid.UUID) *http.Request {
	t.Helper()

	request := newUsersTransportRequest(t, http.MethodDelete, "/users/me")
	ctx := core_context.WithClaims(
		request.Context(),
		core_context.ContextClaims{UserID: userID},
	)
	return request.WithContext(ctx)
}
