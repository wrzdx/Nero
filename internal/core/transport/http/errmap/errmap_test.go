package http_errmap_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	http_errmap "github.com/wrzdx/Nero/internal/core/transport/http/errmap"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/stretchr/testify/require"
)

func TestMap(t *testing.T) {
	t.Run("maps wrapped invalid request and preserves fields", func(t *testing.T) {
		type request struct {
			Username string `json:"username" validate:"required"`
		}
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(nil))
		var decoded request
		err := http_request.DecodeAndValidateRequestBody(r, &decoded)
		require.Error(t, err)
		err = fmt.Errorf("decode register request: %w", err)

		mapped := http_errmap.Map(err)

		require.Equal(t, http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_request",
			Message:    "invalid request",
			Fields: map[string]string{
				"username": "username is required",
			},
		}, mapped)
	})

	t.Run("maps wrapped invalid request without fields", func(t *testing.T) {
		err := fmt.Errorf("read request: %w", http_request.ErrInvalidRequest)

		mapped := http_errmap.Map(err)

		require.Equal(t, http_response.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_request",
			Message:    "invalid request",
		}, mapped)
	})

	t.Run("does not expose fields from unknown error", func(t *testing.T) {
		err := testFieldError{
			err:    errors.New("database failure"),
			fields: map[string]string{"query": "internal SQL"},
		}

		mapped := http_errmap.Map(err)

		require.Equal(t, http_response.HTTPError{
			StatusCode: http.StatusInternalServerError,
			Code:       "internal_error",
			Message:    "internal server error",
		}, mapped)
	})
}

func TestFieldsFrom(t *testing.T) {
	original := testFieldError{
		err:    errors.New("validation failure"),
		fields: map[string]string{"username": "invalid"},
	}
	err := fmt.Errorf("wrapped: %w", original)

	fields := http_errmap.FieldsFrom(err)

	require.Equal(t, original.fields, fields)
}

type testFieldError struct {
	err    error
	fields map[string]string
}

func (e testFieldError) Error() string {
	return e.err.Error()
}

func (e testFieldError) Unwrap() error {
	return e.err
}

func (e testFieldError) Fields() map[string]string {
	return e.fields
}
