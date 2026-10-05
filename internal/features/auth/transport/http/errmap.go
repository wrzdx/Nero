package auth_transport_http

import (
	"errors"
	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"
	http_errmap "github.com/wrzdx/Nero/internal/core/transport/http/errmap"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	"net/http"
)

func errorMapper(err error) http_response.HTTPError {
	switch {
	case errors.Is(err, domain.ErrInvalidUserProfile):
		return http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_user_profile",
			Message:    "invalid user profile",
			Fields:     http_errmap.FieldsFrom(err),
		}

	case errors.Is(err, auth.ErrInvalidPassword):
		return http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_password",
			Message:    "invalid password",
			Fields:     http_errmap.FieldsFrom(err),
		}

	case errors.Is(err, auth.ErrPasswordMismatch):
		return http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "password_mismatch",
			Message:    "password mismatch",
			Fields:     http_errmap.FieldsFrom(err),
		}

	case errors.Is(err, auth.ErrInvalidCredentials):
		return http_response.HTTPError{
			StatusCode: http.StatusUnauthorized,
			Code:       "invalid_credentials",
			Message:    "invalid credentials",
		}

	case errors.Is(err, auth.ErrInvalidToken):
		return http_response.HTTPError{
			StatusCode: http.StatusUnauthorized,
			Code:       "invalid_token",
			Message:    "invalid token",
		}

	case errors.Is(err, domain.ErrAlreadyExists):
		return http_response.HTTPError{
			StatusCode: http.StatusConflict,
			Code:       "user_already_exists",
			Message:    "user already exists",
			Fields:     http_errmap.FieldsFrom(err),
		}
	default:
		return http_errmap.Map(err)
	}
}
