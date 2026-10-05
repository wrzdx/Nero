package auth_transport_http

import (
	"context"
	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"
	auth_service "github.com/wrzdx/Nero/internal/features/auth/service"
	"net/http"

	"github.com/google/uuid"
)

type AuthService interface {
	Register(
		ctx context.Context,
		payload auth_service.RegisterCommand,
	) (domain.User, auth.TokenPair, error)

	Login(
		ctx context.Context,
		username string,
		password string,
	) (auth.TokenPair, error)

	Refresh(
		ctx context.Context,
		token string,
	) (auth.TokenPair, error)

	Logout(ctx context.Context, refreshToken string) error

	ChangePassword(
		ctx context.Context,
		userID uuid.UUID,
		currentPassword string,
		newPassword string,
	) error
}

type CookieManager interface {
	SetRefreshToken(
		w http.ResponseWriter,
		token string,
	)

	ClearRefreshToken(
		w http.ResponseWriter,
	)

	GetRefreshToken(
		r *http.Request,
	) (string, error)
}
