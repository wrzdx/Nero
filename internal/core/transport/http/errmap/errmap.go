package http_errmap

import (
	"errors"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	"net/http"
)

type hasFields interface {
	Fields() map[string]string
	Error() string
}

func FieldsFrom(err error) map[string]string {
	if hasFields, ok := errors.AsType[hasFields](err); ok {
		return hasFields.Fields()
	}
	return nil
}

func Map(err error) http_response.HTTPError {
	switch {
	case errors.Is(err, http_request.ErrInvalidRequest):
		return http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_request",
			Message:    "invalid request",
			Fields:     FieldsFrom(err),
		}
	default:
		return http_response.HTTPError{
			StatusCode: http.StatusInternalServerError,
			Code:       "internal_error",
			Message:    "internal server error",
		}
	}
}
