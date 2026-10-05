package chats_transport_http

import (
	"errors"
	"github.com/wrzdx/Nero/internal/core/domain"
	http_errmap "github.com/wrzdx/Nero/internal/core/transport/http/errmap"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	"net/http"
)

func errorMapper(err error) http_response.HTTPError {
	switch {
	case errors.Is(err, chats_service.ErrInvalidInput):
		return http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_input",
			Message:    "invalid input",
			Fields:     http_errmap.FieldsFrom(err),
		}

	case errors.Is(err, domain.ErrInvalidDirectChat):
		return http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_direct_chat",
			Message:    "invalid direct chat",
			Fields:     http_errmap.FieldsFrom(err),
		}

	case errors.Is(err, domain.ErrInvalidGroupChat):
		return http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_group_chat",
			Message:    "invalid group chat",
			Fields:     http_errmap.FieldsFrom(err),
		}

	case errors.Is(err, chats_service.ErrNotEnoughRights):
		return http_response.HTTPError{
			StatusCode: http.StatusForbidden,
			Code:       "not_enough_rights",
			Message:    "not enough rights",
		}

	case errors.Is(err, chats_service.ErrOwnerCannotQuitGroup):
		return http_response.HTTPError{
			StatusCode: http.StatusForbidden,
			Code:       "owner_cannot_quit_group",
			Message:    "owner cannot quit group",
		}

	case errors.Is(err, domain.ErrNotFound):
		return http_response.HTTPError{
			StatusCode: http.StatusNotFound,
			Code:       "not_found",
			Message:    "not found",
			Fields:     http_errmap.FieldsFrom(err),
		}

	default:
		return http_errmap.Map(err)
	}
}
