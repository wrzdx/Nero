package http_response_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/stretchr/testify/require"
)

func TestHTTPSenderOK(t *testing.T) {
	t.Run("writes successful JSON response without invoking error mapper", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		mapper := func(error) http_response.HTTPError {
			t.Fatal("error mapper must not be called for successful response")
			return http_response.HTTPError{}
		}
		sender := http_response.NewHTTPSender(
			logger.NewTestLogger(),
			recorder,
			mapper,
		)
		data := struct {
			AccessToken string `json:"access_token"`
		}{AccessToken: "access-token"}

		sender.OK(http.StatusOK, data)

		result := recorder.Result()
		defer result.Body.Close()
		require.Equal(t, http.StatusOK, result.StatusCode)
		require.Equal(t, "application/json", result.Header.Get("Content-Type"))
		var body struct {
			Data struct {
				AccessToken string `json:"access_token"`
			} `json:"data"`
		}
		require.NoError(t, json.NewDecoder(result.Body).Decode(&body))
		require.Equal(t, "access-token", body.Data.AccessToken)
	})

	t.Run("writes no body or JSON content type for no content", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		mapper := func(error) http_response.HTTPError {
			t.Fatal("error mapper must not be called for successful response")
			return http_response.HTTPError{}
		}
		sender := http_response.NewHTTPSender(
			logger.NewTestLogger(),
			recorder,
			mapper,
		)

		sender.OK(http.StatusNoContent, struct{}{})

		result := recorder.Result()
		defer result.Body.Close()
		require.Equal(t, http.StatusNoContent, result.StatusCode)
		require.Empty(t, recorder.Body.Bytes())
		require.Empty(t, result.Header.Get("Content-Type"))
	})
}

func TestHTTPSenderError(t *testing.T) {
	t.Run("maps source error and writes public error response", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		sourceErr := errors.New("wrapped internal details")
		mapperCalled := false
		mapper := func(err error) http_response.HTTPError {
			mapperCalled = true
			require.ErrorIs(t, err, sourceErr)
			return http_response.HTTPError{
				StatusCode: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "invalid token",
			}
		}
		sender := http_response.NewHTTPSender(
			logger.NewTestLogger(),
			recorder,
			mapper,
		)

		sender.Error(sourceErr)

		require.True(t, mapperCalled)
		result := recorder.Result()
		defer result.Body.Close()
		require.Equal(t, http.StatusUnauthorized, result.StatusCode)
		require.Equal(t, "application/json", result.Header.Get("Content-Type"))
		var body struct {
			Error http_response.APIErrorDetail `json:"error"`
		}
		require.NoError(t, json.NewDecoder(result.Body).Decode(&body))
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_token",
			Message: "invalid token",
		}, body.Error)
	})

	t.Run("writes validation fields", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		mapper := func(error) http_response.HTTPError {
			return http_response.HTTPError{
				StatusCode: http.StatusBadRequest,
				Code:       "invalid_request",
				Message:    "invalid request",
				Fields: map[string]string{
					"username": "username is required",
				},
			}
		}
		sender := http_response.NewHTTPSender(
			logger.NewTestLogger(),
			recorder,
			mapper,
		)

		sender.Error(errors.New("request validation failed"))

		result := recorder.Result()
		defer result.Body.Close()
		var body struct {
			Error http_response.APIErrorDetail `json:"error"`
		}
		require.NoError(t, json.NewDecoder(result.Body).Decode(&body))
		require.Equal(t, map[string]string{
			"username": "username is required",
		}, body.Error.Fields)
	})

	t.Run("omits absent validation fields", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		mapper := func(error) http_response.HTTPError {
			return http_response.HTTPError{
				StatusCode: http.StatusInternalServerError,
				Code:       "internal_error",
				Message:    "internal server error",
			}
		}
		sender := http_response.NewHTTPSender(
			logger.NewTestLogger(),
			recorder,
			mapper,
		)

		sender.Error(errors.New("database unavailable"))

		require.NotContains(t, recorder.Body.String(), `"fields"`)
	})
}
