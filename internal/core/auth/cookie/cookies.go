package auth_cookie

import (
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/auth"
	"net/http"
	"time"
)

const refreshCookieName = "refresh_token"

type CookieManager struct {
	refreshTTL time.Duration
	secure     bool
	path       string
}

func NewCookieManager(
	refreshTTL time.Duration,
	secure bool,
	path string,
) *CookieManager {
	return &CookieManager{
		refreshTTL: refreshTTL,
		secure:     secure,
		path:       path,
	}
}

func (c *CookieManager) GetRefreshToken(
	r *http.Request,
) (string, error) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return "", fmt.Errorf("refresh: %w", auth.ErrInvalidToken)
		}
		return "", err
	}

	return cookie.Value, nil
}

func (c *CookieManager) SetRefreshToken(
	w http.ResponseWriter,
	token string,
) {
	cookie := &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     c.path,
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(c.refreshTTL.Seconds()),
	}

	http.SetCookie(w, cookie)
}

func (c *CookieManager) ClearRefreshToken(
	w http.ResponseWriter,
) {
	cookie := &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     c.path,
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}

	http.SetCookie(w, cookie)
}
