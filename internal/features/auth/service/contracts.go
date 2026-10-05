package auth_service

import (
	"context"
	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

type SessionsRepository interface {
	CreateSession(
		ctx context.Context,
		session domain.Session,
	) error

	RotateSession(
		ctx context.Context,
		sessionID, currentTokenID, newTokenID uuid.UUID,
		usedAt time.Time,
	) (domain.Session, error)

	DeleteSession(
		ctx context.Context,
		sessionID, currentTokenID uuid.UUID,
	) error

	DeleteAllSessions(
		ctx context.Context,
		userID uuid.UUID,
	) error
}

type UsersRepository interface {
	GetUser(
		ctx context.Context,
		id uuid.UUID,
	) (domain.User, error)

	GetUserForUpdate(
		ctx context.Context,
		userID uuid.UUID,
	) (domain.User, error)

	GetUserByUsername(
		ctx context.Context,
		username string,
	) (domain.User, error)

	CreateUser(
		ctx context.Context,
		user domain.User,
	) error

	ChangePassword(
		ctx context.Context,
		id uuid.UUID,
		newPasswordHash string,
		currentPasswordHash string,
	) error
}

type Hasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
	DummyCompare()
}

type TokenProvider interface {
	GenerateAccessToken(
		accessTokenClaims auth.AccessTokenClaims,
		tokenLifetime auth.TokenLifetime,
	) (string, error)
	GenerateRefreshToken(
		refreshTokenClaims auth.RefreshTokenClaims,
		tokenLifetime auth.TokenLifetime,
	) (string, error)
	ParseRefreshToken(tokenStr string) (auth.RefreshTokenClaims, error)
}

type TXManager interface {
	WithinTransaction(
		ctx context.Context,
		fn func(ctx context.Context) error,
	) error
}
