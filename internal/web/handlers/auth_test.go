package web_handlers

import (
	"context"
	"errors"
	"messenger/internal/core/auth"
	auth_cookie "messenger/internal/core/auth/cookie"
	"messenger/internal/core/domain"
	auth_service "messenger/internal/features/auth/service"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type authStub struct {
	register func(auth_service.RegisterCommand) (auth.TokenPair, error)
	login    func(string, string) (auth.TokenPair, error)
	parse    func(string) (auth.ParsedAccessToken, error)
}

func (s authStub) Register(_ context.Context, command auth_service.RegisterCommand) (domain.User, auth.TokenPair, error) {
	tokens, err := s.register(command)
	return domain.User{}, tokens, err
}

func (s authStub) Login(_ context.Context, username, password string) (auth.TokenPair, error) {
	return s.login(username, password)
}

func (s authStub) ParseAccessToken(token string) (auth.ParsedAccessToken, error) {
	return s.parse(token)
}

func testHandler(stub authStub) *AuthHandler {
	return NewAuthHandler(stub, auth_cookie.NewCookieManager(time.Hour, true, "/api/v1/auth"), stub, 15*time.Minute, true, zap.NewNop())
}

func formRequest(path string, values url.Values, partial bool) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if partial {
		r.Header.Set("HX-Request", "true")
	}
	return r
}

func TestRegisterRejectsInvalidDataBeforeCallingService(t *testing.T) {
	for _, tc := range []struct{ name, username, firstName, password, message string }{
		{"blank name", "daniel_k", "  ", "long-password-123", "Введите имя"},
		{"invalid username", "bad!", "Даниил", "long-password-123", "От 5 до 32"},
		{"short password", "daniel_k", "Даниил", "short", "от 15 символов"},
		{"bcrypt byte limit", "daniel_k", "Даниил", strings.Repeat("я", 37), "72 байт"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler(authStub{register: func(auth_service.RegisterCommand) (auth.TokenPair, error) {
				t.Fatal("invalid form must not reach service")
				return auth.TokenPair{}, nil
			}})
			w := httptest.NewRecorder()
			h.Register(w, formRequest("/register", url.Values{"username": {tc.username}, "first_name": {tc.firstName}, "password": {tc.password}}, true))
			require.Equal(t, http.StatusUnprocessableEntity, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			require.NotContains(t, w.Body.String(), "<html")
			require.Empty(t, w.Result().Cookies())
		})
	}
}

func TestRegisterPreservesPasswordAndSetsCookies(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary form", true: "htmx"}[partial], func(t *testing.T) {
			called := false
			h := testHandler(authStub{register: func(command auth_service.RegisterCommand) (auth.TokenPair, error) {
				called = true
				require.Equal(t, "daniel_k", command.Username)
				require.Equal(t, "Даниил", command.FirstName)
				require.Equal(t, "  long-password-123  ", command.Password)
				return auth.TokenPair{Access: "access-value", Refresh: "refresh-value"}, nil
			}})
			w := httptest.NewRecorder()
			h.Register(w, formRequest("/register", url.Values{"username": {" daniel_k "}, "first_name": {" Даниил "}, "password": {"  long-password-123  "}}, partial))
			require.True(t, called)
			if partial {
				require.Equal(t, http.StatusNoContent, w.Code)
				require.Equal(t, "/welcome", w.Header().Get("HX-Redirect"))
			} else {
				require.Equal(t, http.StatusSeeOther, w.Code)
				require.Equal(t, "/welcome", w.Header().Get("Location"))
			}
			cookies := w.Result().Cookies()
			require.Len(t, cookies, 2)
			for _, cookie := range cookies {
				require.True(t, cookie.HttpOnly)
				require.True(t, cookie.Secure)
				require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
				if cookie.Name == accessCookieName {
					require.Equal(t, "/", cookie.Path)
					require.Equal(t, 900, cookie.MaxAge)
				} else {
					require.Equal(t, "refresh_token", cookie.Name)
					require.Equal(t, "/api/v1/auth", cookie.Path)
				}
			}
			require.NotContains(t, w.Body.String(), "access-value")
		})
	}
}

func TestRegisterConflictKeepsSafeValuesAndClearsPassword(t *testing.T) {
	h := testHandler(authStub{register: func(auth_service.RegisterCommand) (auth.TokenPair, error) {
		return auth.TokenPair{}, domain.ErrAlreadyExists
	}})
	w := httptest.NewRecorder()
	h.Register(w, formRequest("/register", url.Values{
		"username": {"daniel_k"}, "first_name": {`<script>alert(1)</script>`}, "password": {"secret-password-123"},
	}, false))
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "уже занято")
	require.Contains(t, w.Body.String(), "<html")
	require.Contains(t, w.Body.String(), "&lt;script&gt;")
	require.NotContains(t, w.Body.String(), "<script>alert(1)</script>")
	require.NotContains(t, w.Body.String(), "secret-password-123")
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestLoginErrorsDoNotLeakInternals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"credentials", auth.ErrInvalidCredentials, 401, "Неверное имя пользователя или пароль"},
		{"database", errors.New("private-database-connection-info"), 500, "Попробуйте позже"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler(authStub{login: func(username, password string) (auth.TokenPair, error) {
				require.Equal(t, "daniel_k", username)
				require.Equal(t, "secret-password-123", password)
				return auth.TokenPair{}, tc.err
			}})
			w := httptest.NewRecorder()
			h.Login(w, formRequest("/login", url.Values{"username": {"daniel_k"}, "password": {"secret-password-123"}}, true))
			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			require.NotContains(t, w.Body.String(), "private-database")
			require.NotContains(t, w.Body.String(), "secret-password-123")
			require.Empty(t, w.Result().Cookies())
		})
	}
}

func TestLoginSuccessAndRequestBoundaries(t *testing.T) {
	called := 0
	h := testHandler(authStub{login: func(string, string) (auth.TokenPair, error) {
		called++
		return auth.TokenPair{Access: "access", Refresh: "refresh"}, nil
	}})
	w := httptest.NewRecorder()
	h.Login(w, formRequest("/login", url.Values{"username": {"daniel_k"}, "password": {"valid-password-123"}}, true))
	require.Equal(t, 1, called)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Len(t, w.Result().Cookies(), 2)

	for _, tc := range []struct {
		name, path, body, contentType string
		status                        int
	}{
		{"query credentials", "/login?username=daniel_k&password=secret", "", "application/x-www-form-urlencoded", 422},
		{"JSON", "/login", `{"username":"daniel_k"}`, "application/json", 415},
		{"oversized", "/login", "username=" + strings.Repeat("x", 9000), "application/x-www-form-urlencoded", 413},
		{"malformed encoding", "/login", "username=%ZZ", "application/x-www-form-urlencoded", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			h.Login(w, r)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, 1, called)
		})
	}
}

func TestWelcomeRequiresValidAccessToken(t *testing.T) {
	h := testHandler(authStub{parse: func(token string) (auth.ParsedAccessToken, error) {
		if token == "valid" {
			return auth.ParsedAccessToken{}, nil
		}
		return auth.ParsedAccessToken{}, auth.ErrInvalidToken
	}})
	for _, token := range []string{"", "expired", "valid"} {
		r := httptest.NewRequest("GET", "/welcome", nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: accessCookieName, Value: token})
		}
		w := httptest.NewRecorder()
		h.Welcome(w, r)
		if token == "valid" {
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), "Вы вошли в Nero")
		} else {
			require.Equal(t, 302, w.Code)
			require.Equal(t, "/login", w.Header().Get("Location"))
		}
	}
}

func TestCrossOriginFormIsRejected(t *testing.T) {
	h := testHandler(authStub{login: func(string, string) (auth.TokenPair, error) {
		t.Fatal("cross-origin form reached service")
		return auth.TokenPair{}, nil
	}})
	r := formRequest("https://nero.example/login", url.Values{"username": {"daniel_k"}, "password": {"secret-password-123"}}, false)
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	http.NewCrossOriginProtection().Handler(http.HandlerFunc(h.Login)).ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden, w.Code)
}
