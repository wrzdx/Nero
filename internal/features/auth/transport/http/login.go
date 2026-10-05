package auth_transport_http

import (
	logger "github.com/wrzdx/Nero/internal/core/logger"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	"net/http"
)

type LoginRequest struct {
	Username string `json:"username" validate:"required" example:"qwerty"`
	Password string `json:"password" validate:"required" example:"password"`
}

type LoginResponse struct {
	Access string `json:"access_token"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	var request LoginRequest
	if err := http_request.DecodeAndValidateRequestBody(r, &request); err != nil {
		sender.Error(err)
		return
	}

	tokens, err := h.authService.Login(
		ctx,
		request.Username,
		request.Password,
	)
	if err != nil {
		sender.Error(err)
		return
	}
	response := LoginResponse{
		Access: tokens.Access,
	}
	h.cookieManager.SetRefreshToken(w, tokens.Refresh)
	sender.OK(http.StatusOK, response)
}
