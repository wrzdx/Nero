package auth_service

import (
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"
)

type AuthConfig struct {
	AccessTokenTTL time.Duration
	SessionTTL     time.Duration
}

func (c AuthConfig) Validate() error {
	if c.AccessTokenTTL <= 0 {
		return errors.New("invalid access token ttl")
	}
	if c.SessionTTL <= 0 {
		return errors.New("invalid session ttl")
	}
	if c.AccessTokenTTL > c.SessionTTL {
		return errors.New("access token ttl should be no more than session ttl")
	}

	return nil
}

type AuthService struct {
	usersRepository    UsersRepository
	sessionsRepository SessionsRepository
	hasher             Hasher
	tokenProvider      TokenProvider
	txManager          TXManager
	config             AuthConfig
}

func NewAuthService(
	usersRepository UsersRepository,
	sessionsRepository SessionsRepository,
	hasher Hasher,
	tokenProvider TokenProvider,
	txManager TXManager,
	config AuthConfig,
) (*AuthService, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate auth config: %w", err)
	}
	return &AuthService{
		usersRepository:    usersRepository,
		sessionsRepository: sessionsRepository,
		hasher:             hasher,
		tokenProvider:      tokenProvider,
		txManager:          txManager,
		config:             config,
	}, nil
}

func (s AuthService) generateTokenPair(session domain.Session, issuedAt time.Time) (auth.TokenPair, error) {
	aToken, err := s.tokenProvider.GenerateAccessToken(
		auth.AccessTokenClaims{
			UserID: session.UserID,
		},
		auth.TokenLifetime{
			IssuedAt:  issuedAt,
			ExpiresAt: issuedAt.Add(s.config.AccessTokenTTL),
		},
	)

	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("generate access: %w", err)
	}

	rToken, err := s.tokenProvider.GenerateRefreshToken(
		auth.RefreshTokenClaims{
			SessionID: session.ID,
			TokenID:   session.CurrentTokenID,
		}, auth.TokenLifetime{
			IssuedAt:  issuedAt,
			ExpiresAt: session.ExpiresAt,
		},
	)
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("generate refresh: %w", err)
	}

	return auth.TokenPair{
		Access:  aToken,
		Refresh: rToken,
	}, nil
}
