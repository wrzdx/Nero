package auth_transport_http

import (
	"fmt"
	logger "github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	"net/http"
)

type RefreshResponse struct {
	Access string `json:"access_token"`
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)

	token, err := h.cookieManager.GetRefreshToken(r)
	if err != nil {
		sender.Error(fmt.Errorf("get refresh token: %w", err))
		return
	}

	tokens, err := h.authService.Refresh(ctx, token)
	if err != nil {
		sender.Error(err)
		return
	}
	response := RefreshResponse{
		Access: tokens.Access,
	}

	h.cookieManager.SetRefreshToken(w, tokens.Refresh)
	sender.OK(http.StatusOK, response)
}
