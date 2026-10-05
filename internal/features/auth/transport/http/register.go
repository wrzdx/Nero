package auth_transport_http

import (
	logger "github.com/wrzdx/Nero/internal/core/logger"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	auth_service "github.com/wrzdx/Nero/internal/features/auth/service"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	var request RegisterRequest
	if err := http_request.DecodeAndValidateRequestBody(r, &request); err != nil {
		sender.Error(err)
		return
	}
	payload := auth_service.RegisterCommand{
		Username:  request.Username,
		FirstName: request.FirstName,
		LastName:  request.LastName,
		Bio:       request.Bio,
		Password:  request.Password,
	}
	userDomain, tokens, err := h.authService.Register(ctx, payload)
	if err != nil {
		sender.Error(err)
		return
	}

	userResponse := UserResponse{
		userDomain.ID,
		userDomain.Profile.Username,
		userDomain.Profile.FirstName,
		userDomain.Profile.LastName,
		userDomain.CreatedAt,
		userDomain.Profile.Bio,
	}
	response := RegisterResponse{
		User:   userResponse,
		Access: tokens.Access,
	}

	h.cookieManager.SetRefreshToken(w, tokens.Refresh)
	sender.OK(http.StatusCreated, response)
}

type RegisterRequest struct {
	Username  string  `json:"username" validate:"required" example:"qwerty"`
	FirstName string  `json:"first_name" validate:"required" example:"Ivan"`
	LastName  *string `json:"last_name"  example:"Ivanov"`
	Bio       *string `json:"bio"  example:"We didn't choose this path. Circumstance chose it for us. We're simply trying to keep climbing."`
	Password  string  `json:"password" validate:"required" example:"password"`
}

type RegisterResponse struct {
	User   UserResponse `json:"user"`
	Access string       `json:"access_token"`
}

type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"  example:"qwerty"`
	FirstName string    `json:"first_name"  example:"Ivan"`
	LastName  *string   `json:"last_name"  example:"Ivanov"`
	CreatedAt time.Time `json:"created_at" example:"2026-02-26T10:30:00Z"`
	Bio       *string   `json:"bio"  example:"We didn't choose this path. Circumstance chose it for us. We're simply trying to keep climbing."`
}
