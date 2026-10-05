package http_middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wrzdx/Nero/internal/core/auth"
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAuth(t *testing.T) {
	t.Run("passes authenticated user claims to next handler", func(t *testing.T) {
		userID := uuid.New()
		provider := &tokenProviderStub{
			parse: func(token string) (auth.ParsedAccessToken, error) {
				require.Equal(t, "access-token", token)
				return auth.ParsedAccessToken{
					AccessTokenClaims: auth.AccessTokenClaims{
						UserID: userID,
					},
				}, nil
			},
		}
		nextCalls := 0
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalls++
			claims, ok := core_context.ClaimsFromCtx(r.Context())
			require.True(t, ok)
			require.Equal(t, userID, claims.UserID)
			w.WriteHeader(http.StatusNoContent)
		})
		recorder := httptest.NewRecorder()
		request := newAuthMiddlewareRequest("Bearer access-token")

		Auth(provider)(next).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.Equal(t, 1, provider.calls)
		require.Equal(t, 1, nextCalls)
	})

	t.Run("accepts case-insensitive bearer scheme", func(t *testing.T) {
		provider := &tokenProviderStub{
			parse: func(token string) (auth.ParsedAccessToken, error) {
				require.Equal(t, "access-token", token)
				return auth.ParsedAccessToken{
					AccessTokenClaims: auth.AccessTokenClaims{
						UserID: uuid.New(),
					},
				}, nil
			},
		}
		nextCalls := 0
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalls++
		})
		recorder := httptest.NewRecorder()
		request := newAuthMiddlewareRequest("bEaReR access-token")

		Auth(provider)(next).ServeHTTP(recorder, request)

		require.Equal(t, 1, provider.calls)
		require.Equal(t, 1, nextCalls)
	})

	invalidHeaders := []struct {
		name   string
		header string
	}{
		{name: "missing header"},
		{name: "wrong scheme", header: "Basic access-token"},
		{name: "missing token", header: "Bearer"},
		{name: "extra parts", header: "Bearer access-token extra"},
	}
	for _, test := range invalidHeaders {
		t.Run("rejects "+test.name, func(t *testing.T) {
			provider := &tokenProviderStub{
				parse: func(string) (auth.ParsedAccessToken, error) {
					t.Fatal("token provider must not be called for malformed authorization header")
					return auth.ParsedAccessToken{}, nil
				},
			}
			nextCalls := 0
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				nextCalls++
			})
			recorder := httptest.NewRecorder()
			request := newAuthMiddlewareRequest(test.header)

			Auth(provider)(next).ServeHTTP(recorder, request)

			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			require.Equal(t, 0, provider.calls)
			require.Equal(t, 0, nextCalls)
			require.Equal(t, http_response.APIErrorDetail{
				Code:    "invalid_token",
				Message: "invalid token",
			}, decodeMiddlewareError(t, recorder))
		})
	}

	t.Run("returns unauthorized when provider rejects token", func(t *testing.T) {
		provider := &tokenProviderStub{
			parse: func(string) (auth.ParsedAccessToken, error) {
				return auth.ParsedAccessToken{}, auth.ErrInvalidToken
			},
		}
		nextCalls := 0
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalls++
		})
		recorder := httptest.NewRecorder()
		request := newAuthMiddlewareRequest("Bearer invalid-token")

		Auth(provider)(next).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		require.Equal(t, 1, provider.calls)
		require.Equal(t, 0, nextCalls)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_token",
			Message: "invalid token",
		}, decodeMiddlewareError(t, recorder))
	})

	t.Run("does not expose unexpected provider error", func(t *testing.T) {
		providerErr := errors.New("key storage unavailable")
		provider := &tokenProviderStub{
			parse: func(string) (auth.ParsedAccessToken, error) {
				return auth.ParsedAccessToken{}, providerErr
			},
		}
		nextCalls := 0
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalls++
		})
		recorder := httptest.NewRecorder()
		request := newAuthMiddlewareRequest("Bearer access-token")

		Auth(provider)(next).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, 1, provider.calls)
		require.Equal(t, 0, nextCalls)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeMiddlewareError(t, recorder))
		require.NotContains(t, recorder.Body.String(), providerErr.Error())
	})
}

type tokenProviderStub struct {
	parse func(token string) (auth.ParsedAccessToken, error)
	calls int
}

func (s *tokenProviderStub) ParseAccessToken(token string) (auth.ParsedAccessToken, error) {
	s.calls++
	return s.parse(token)
}

func newAuthMiddlewareRequest(authorization string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}

func decodeMiddlewareError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) http_response.APIErrorDetail {
	t.Helper()
	var response http_response.APIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotNil(t, response.Error)
	return *response.Error
}
