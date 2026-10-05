package auth_transport_http

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/stretchr/testify/require"
)

func TestMap(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want http_response.HTTPError
	}{
		{
			name: "invalid user profile with wrapped fields",
			err: fmt.Errorf("new user profile: %w", domain.DetailedError{
				Err: domain.ErrInvalidUserProfile,
				Details: map[string]string{
					"username": "invalid username",
				},
			}),
			want: http_response.HTTPError{
				StatusCode: http.StatusBadRequest,
				Code:       "invalid_user_profile",
				Message:    "invalid user profile",
				Fields: map[string]string{
					"username": "invalid username",
				},
			},
		},
		{
			name: "invalid password",
			err:  fmt.Errorf("validate password: %w", auth.ErrInvalidPassword),
			want: http_response.HTTPError{
				StatusCode: http.StatusBadRequest,
				Code:       "invalid_password",
				Message:    "invalid password",
			},
		},
		{
			name: "invalid credentials",
			err:  fmt.Errorf("login: %w", auth.ErrInvalidCredentials),
			want: http_response.HTTPError{
				StatusCode: http.StatusUnauthorized,
				Code:       "invalid_credentials",
				Message:    "invalid credentials",
			},
		},
		{
			name: "invalid token",
			err:  fmt.Errorf("refresh: %w", auth.ErrInvalidToken),
			want: http_response.HTTPError{
				StatusCode: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "invalid token",
			},
		},
		{
			name: "already exists with username conflict",
			err: fmt.Errorf("transaction: %w", domain.DetailedError{
				Err: fmt.Errorf("user: %w", domain.ErrAlreadyExists),
				Details: map[string]string{
					"username": "username already taken",
				},
			}),
			want: http_response.HTTPError{
				StatusCode: http.StatusConflict,
				Code:       "user_already_exists",
				Message:    "user already exists",
				Fields: map[string]string{
					"username": "username already taken",
				},
			},
		},
		{
			name: "already exists with future email conflict",
			err: domain.DetailedError{
				Err: domain.ErrAlreadyExists,
				Details: map[string]string{
					"email": "email already taken",
				},
			},
			want: http_response.HTTPError{
				StatusCode: http.StatusConflict,
				Code:       "user_already_exists",
				Message:    "user already exists",
				Fields: map[string]string{
					"email": "email already taken",
				},
			},
		},
		{
			name: "not found is not a public auth outcome",
			err:  domain.ErrNotFound,
			want: http_response.HTTPError{
				StatusCode: http.StatusInternalServerError,
				Code:       "internal_error",
				Message:    "internal server error",
			},
		},
		{
			name: "invalid request delegates to generic mapper",
			err:  fmt.Errorf("decode login: %w", http_request.ErrInvalidRequest),
			want: http_response.HTTPError{
				StatusCode: http.StatusBadRequest,
				Code:       "invalid_request",
				Message:    "invalid request",
			},
		},
		{
			name: "unknown error delegates to internal error",
			err:  errors.New("database unavailable"),
			want: http_response.HTTPError{
				StatusCode: http.StatusInternalServerError,
				Code:       "internal_error",
				Message:    "internal server error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := errorMapper(tt.err)

			require.Equal(t, tt.want, mapped)
		})
	}
}
