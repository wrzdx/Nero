package auth_transport_http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	auth_service "github.com/wrzdx/Nero/internal/features/auth/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	t.Run("returns created user and access token", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		user := newAuthTransportUser(t)
		tokens := auth.TokenPair{
			Access:  "access-token",
			Refresh: "refresh-token",
		}
		payload := auth_service.RegisterCommand{
			Username:  "Username_1",
			FirstName: "First Name",
			LastName:  new("Last Name"),
			Bio:       new("Bio"),
			Password:  "valid password value",
		}
		service.EXPECT().
			Register(mock.Anything, payload).
			Return(user, tokens, nil)
		cookieManager.EXPECT().
			SetRefreshToken(mock.Anything, tokens.Refresh).
			Return()
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/register",
			map[string]any{
				"username":   payload.Username,
				"first_name": payload.FirstName,
				"last_name":  *payload.LastName,
				"bio":        *payload.Bio,
				"password":   payload.Password,
			},
		)
		recorder := httptest.NewRecorder()

		handler.Register(recorder, req)

		require.Equal(t, http.StatusCreated, recorder.Code)
		var body struct {
			Data RegisterResponse `json:"data"`
		}
		require.NoError(t, decodeAuthTransportResponse(recorder, &body))
		require.Equal(t, tokens.Access, body.Data.Access)
		require.Equal(t, user.ID, body.Data.User.ID)
		require.Equal(t, user.Profile.Username, body.Data.User.Username)
		require.Equal(t, user.Profile.FirstName, body.Data.User.FirstName)
		require.Equal(t, user.Profile.LastName, body.Data.User.LastName)
		require.Equal(t, user.Profile.Bio, body.Data.User.Bio)
		require.True(t, user.CreatedAt.Equal(body.Data.User.CreatedAt))
		require.NotContains(t, recorder.Body.String(), `"success"`)
	})

	t.Run("returns conflict fields without setting refresh cookie", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		payload := auth_service.RegisterCommand{
			Username:  "Username_1",
			FirstName: "First Name",
			Password:  "valid password value",
		}
		createErr := fmt.Errorf("transaction: %w", domain.DetailedError{
			Err: fmt.Errorf("user: %w", domain.ErrAlreadyExists),
			Details: map[string]string{
				"username": "username already taken",
			},
		})
		service.EXPECT().
			Register(mock.Anything, payload).
			Return(domain.User{}, auth.TokenPair{}, createErr)
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/register",
			map[string]any{
				"username":   payload.Username,
				"first_name": payload.FirstName,
				"password":   payload.Password,
			},
		)
		recorder := httptest.NewRecorder()

		handler.Register(recorder, req)

		require.Equal(t, http.StatusConflict, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "user_already_exists",
			Message: "user already exists",
			Fields: map[string]string{
				"username": "username already taken",
			},
		}, decodeAuthTransportError(t, recorder))
	})

	t.Run("returns validation fields without calling service", func(t *testing.T) {
		service := NewMockAuthService(t)
		cookieManager := NewMockCookieManager(t)
		handler := NewAuthHTTPHandler(service, cookieManager)
		req := newAuthTransportRequest(
			t,
			http.MethodPost,
			"/auth/register",
			map[string]any{},
		)
		recorder := httptest.NewRecorder()

		handler.Register(recorder, req)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"username":   "username is required",
				"first_name": "first_name is required",
				"password":   "password is required",
			},
		}, decodeAuthTransportError(t, recorder))
	})
}

func newAuthTransportUser(t *testing.T) domain.User {
	t.Helper()

	profile, err := domain.NewUserProfile(
		"Username_1",
		"First Name",
		new("Last Name"),
		new("Bio"),
	)
	require.NoError(t, err)
	user, err := domain.NewUser(
		uuid.New(),
		profile,
		time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC),
		nil,
		"password-hash",
	)
	require.NoError(t, err)
	return user
}
