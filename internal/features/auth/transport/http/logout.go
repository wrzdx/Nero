package auth_transport_http

import (
	"errors"
	"github.com/wrzdx/Nero/internal/core/auth"
	logger "github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	"net/http"
)

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)

	token, err := h.cookieManager.GetRefreshToken(r)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			h.cookieManager.ClearRefreshToken(w)
			sender.OK(http.StatusNoContent, nil)
			return
		}
		sender.Error(err)
		return
	}
	if err := h.authService.Logout(ctx, token); err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			h.cookieManager.ClearRefreshToken(w)
		}
		sender.Error(err)
		return
	}

	h.cookieManager.ClearRefreshToken(w)
	sender.OK(http.StatusNoContent, nil)
}
