package users_transport_http

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/wrzdx/Nero/internal/core/domain"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/stretchr/testify/require"
)

func TestErrorMapper(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected http_response.HTTPError
	}{
		{
			name: "invalid profile with fields",
			err: domain.DetailedError{
				Err:     domain.ErrInvalidUserProfile,
				Details: map[string]string{"username": "invalid username"},
			},
			expected: http_response.HTTPError{
				StatusCode: http.StatusBadRequest,
				Code:       "invalid_user_profile",
				Message:    "invalid user profile",
				Fields:     map[string]string{"username": "invalid username"},
			},
		},
		{
			name: "username conflict with fields",
			err: domain.DetailedError{
				Err:     fmt.Errorf("user: %w", domain.ErrAlreadyExists),
				Details: map[string]string{"username": "username already exists"},
			},
			expected: http_response.HTTPError{
				StatusCode: http.StatusConflict,
				Code:       "user_already_exists",
				Message:    "user already exists",
				Fields:     map[string]string{"username": "username already exists"},
			},
		},
		{
			name: "missing user",
			err:  fmt.Errorf("get user: %w", domain.ErrNotFound),
			expected: http_response.HTTPError{
				StatusCode: http.StatusNotFound,
				Code:       "user_not_found",
				Message:    "user not found",
			},
		},
		{
			name: "unexpected error",
			err:  errors.New("database unavailable"),
			expected: http_response.HTTPError{
				StatusCode: http.StatusInternalServerError,
				Code:       "internal_error",
				Message:    "internal server error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, errorMapper(tt.err))
		})
	}
}
