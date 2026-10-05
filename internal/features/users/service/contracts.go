package users_service

import (
	"context"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

type UsersRepository interface {
	SearchUsers(ctx context.Context, requesterID uuid.UUID, prefix string, limit int) ([]UserSearchResult, error)

	GetUser(
		ctx context.Context,
		id uuid.UUID,
	) (domain.User, error)

	GetUserForUpdate(
		ctx context.Context,
		userID uuid.UUID,
	) (domain.User, error)

	DeleteUser(
		ctx context.Context,
		id uuid.UUID,
		profile domain.UserProfile,
		deletedAt time.Time,
	) error

	UpdateUserProfile(
		ctx context.Context,
		id uuid.UUID,
		profile domain.UserProfile,
	) error
}

type SessionsRepository interface {
	DeleteAllSessions(ctx context.Context, userID uuid.UUID) error
}

type TXManager interface {
	WithinTransaction(
		ctx context.Context,
		fn func(context.Context) error,
	) error
}
