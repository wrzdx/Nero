package users_transport_http

import (
	"errors"
	"github.com/wrzdx/Nero/internal/core/domain"
	http_errmap "github.com/wrzdx/Nero/internal/core/transport/http/errmap"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	"net/http"
)

func errorMapper(err error) http_response.HTTPError {
	switch {
	case errors.Is(err, users_service.ErrInvalidSearchQuery):
		return http_response.HTTPError{StatusCode: http.StatusBadRequest, Code: "invalid_user_search", Message: "invalid user search query", Fields: http_errmap.FieldsFrom(err)}
	case errors.Is(err, domain.ErrInvalidUserProfile):
		return http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_user_profile",
			Message:    "invalid user profile",
			Fields:     http_errmap.FieldsFrom(err),
		}

	case errors.Is(err, domain.ErrAlreadyExists):
		return http_response.HTTPError{
			StatusCode: http.StatusConflict,
			Code:       "user_already_exists",
			Message:    "user already exists",
			Fields:     http_errmap.FieldsFrom(err),
		}

	case errors.Is(err, domain.ErrNotFound):
		return http_response.HTTPError{
			StatusCode: http.StatusNotFound,
			Code:       "user_not_found",
			Message:    "user not found",
		}

	default:
		return http_errmap.Map(err)
	}
}
