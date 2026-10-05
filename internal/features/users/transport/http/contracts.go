package users_transport_http

import (
	"context"
	"github.com/wrzdx/Nero/internal/core/domain"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	"net/http"

	"github.com/google/uuid"
)

type UsersService interface {
	SearchUsers(ctx context.Context, requesterID uuid.UUID, query users_service.SearchUsersQuery) ([]users_service.UserSearchResult, error)

	GetUser(
		ctx context.Context,
		id uuid.UUID,
	) (domain.User, error)
	UpdateProfile(
		ctx context.Context,
		userID uuid.UUID,
		command users_service.UpdateProfileCommand,
	) (domain.User, error)
	DeleteAccount(
		ctx context.Context,
		userID uuid.UUID,
	) error
}

type CookieManager interface {
	ClearRefreshToken(
		w http.ResponseWriter,
	)
}
