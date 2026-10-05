package web_handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/wrzdx/Nero/internal/core/auth"
	core_context "github.com/wrzdx/Nero/internal/core/context"
	web_views "github.com/wrzdx/Nero/internal/web/views"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

const refreshCookieName = "nero_refresh"

type SessionService interface {
	Refresh(context.Context, string) (auth.TokenPair, error)
	Logout(context.Context, string) error
}

type refreshedSession struct {
	tokens auth.TokenPair
	until  time.Time
	userID uuid.UUID
}

// SessionManager bridges existing JWT auth to HTML pages. Concurrent requests
// with an expired access cookie must share one rotation of the refresh token.
type SessionManager struct {
	service    SessionService
	auth       *AuthHandler
	group      singleflight.Group
	mu         sync.Mutex
	recent     map[[32]byte]refreshedSession
	refreshTTL time.Duration
}

func NewSessionManager(service SessionService, handler *AuthHandler) *SessionManager {
	s := &SessionManager{service: service, auth: handler, recent: make(map[[32]byte]refreshedSession), refreshTTL: 24 * time.Hour}
	handler.session = s
	return s
}

func (s *SessionManager) setCookies(w http.ResponseWriter, tokens auth.TokenPair) {
	s.auth.refreshCookie.SetRefreshToken(w, tokens.Refresh)
	http.SetCookie(w, &http.Cookie{Name: refreshCookieName, Value: tokens.Refresh, Path: "/", HttpOnly: true, Secure: s.auth.secure, SameSite: http.SameSiteLaxMode, MaxAge: int(s.refreshTTL.Seconds())})
}

// Configure the refresh cookie lifetime from the same config as AuthService.
func (s *SessionManager) SetRefreshTTL(ttl time.Duration) { s.refreshTTL = ttl }

func (s *SessionManager) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{accessCookieName, refreshCookieName} {
		http.SetCookie(w, &http.Cookie{Name: name, Path: "/", HttpOnly: true, Secure: s.auth.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	}
	if manager, ok := s.auth.refreshCookie.(interface{ ClearRefreshToken(http.ResponseWriter) }); ok {
		manager.ClearRefreshToken(w)
	}
}

func (s *SessionManager) rotate(ctx context.Context, token string) (auth.TokenPair, error) {
	key := sha256.Sum256([]byte(token))
	result, err, _ := s.group.Do(string(key[:]), func() (any, error) {
		s.mu.Lock()
		cached, ok := s.recent[key]
		s.mu.Unlock()
		if ok && time.Now().Before(cached.until) {
			return cached.tokens, nil
		}
		tokens, err := s.service.Refresh(ctx, token)
		if err != nil {
			return auth.TokenPair{}, err
		}
		claims, err := s.auth.tokens.ParseAccessToken(tokens.Access)
		if err != nil {
			return auth.TokenPair{}, err
		}
		s.mu.Lock()
		for k, value := range s.recent {
			if time.Now().After(value.until) {
				delete(s.recent, k)
			}
		}
		if len(s.recent) < 1024 {
			s.recent[key] = refreshedSession{tokens: tokens, until: time.Now().Add(5 * time.Second), userID: claims.UserID}
		}
		s.mu.Unlock()
		return tokens, nil
	})
	if err != nil {
		return auth.TokenPair{}, err
	}
	return result.(auth.TokenPair), nil
}

func (s *SessionManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var claims auth.ParsedAccessToken
		cookie, err := r.Cookie(accessCookieName)
		if err == nil {
			claims, err = s.auth.tokens.ParseAccessToken(cookie.Value)
		}
		if err != nil {
			refresh, cookieErr := r.Cookie(refreshCookieName)
			if cookieErr != nil {
				redirectPage(w, r, "/login")
				return
			}
			tokens, refreshErr := s.rotate(r.Context(), refresh.Value)
			if refreshErr != nil {
				if errors.Is(refreshErr, auth.ErrInvalidToken) || errors.Is(refreshErr, auth.ErrInvalidClaims) {
					s.clearCookies(w)
					redirectPage(w, r, "/login")
					return
				}
				message := "Не удалось обновить сессию. Попробуй позже."
				if r.Header.Get("HX-Request") == "true" {
					w.Header().Set("HX-Retarget", "#app-toast")
					w.Header().Set("HX-Reswap", "innerHTML")
					s.auth.render(w, r, http.StatusServiceUnavailable, web_views.ErrorNotice(message))
				} else {
					http.Error(w, message, http.StatusServiceUnavailable)
				}
				return
			}
			claims, err = s.auth.tokens.ParseAccessToken(tokens.Access)
			if err != nil {
				redirectPage(w, r, "/login")
				return
			}
			s.auth.setAccessCookie(w, tokens.Access)
			s.setCookies(w, tokens)
		}
		ctx := core_context.WithClaims(r.Context(), core_context.ContextClaims{UserID: claims.UserID})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *SessionManager) Token(w http.ResponseWriter, r *http.Request) {
	// WS authenticates using a JSON message. Its token lives only in JS memory.
	// During rotation the response cookie already contains the new token.
	token := ""
	for _, cookie := range w.Header().Values("Set-Cookie") {
		parsed, err := http.ParseSetCookie(cookie)
		if err == nil && parsed.Name == accessCookieName {
			token = parsed.Value
		}
	}
	if token == "" {
		cookie, _ := r.Cookie(accessCookieName)
		if cookie != nil {
			token = cookie.Value
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"access_token": token})
}

func (s *SessionManager) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookieName); err == nil {
		token := cookie.Value
		s.mu.Lock()
		if cached, ok := s.recent[sha256.Sum256([]byte(token))]; ok && time.Now().Before(cached.until) {
			token = cached.tokens.Refresh
		}
		s.mu.Unlock()
		if err := s.service.Logout(r.Context(), token); err != nil && !errors.Is(err, auth.ErrInvalidToken) {
			http.Error(w, "Не удалось выйти. Попробуйте ещё раз.", http.StatusServiceUnavailable)
			return
		}
		s.mu.Lock()
		for key, cached := range s.recent {
			if key == sha256.Sum256([]byte(cookie.Value)) || cached.tokens.Refresh == token {
				delete(s.recent, key)
			}
		}
		s.mu.Unlock()
	}
	s.clearCookies(w)
	redirectPage(w, r, "/")
}

func (s *SessionManager) invalidateUser(userID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, cached := range s.recent {
		if cached.userID == userID {
			delete(s.recent, key)
		}
	}
}

func redirectPage(w http.ResponseWriter, r *http.Request, path string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", path)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}
