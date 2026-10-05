package auth_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s *AuthService) Refresh(
	ctx context.Context,
	token string,
) (auth.TokenPair, error) {
	rClaims, err := s.tokenProvider.ParseRefreshToken(token)
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("parse refresh: %w", err)
	}
	newTokenID := uuid.New()
	usedAt := time.Now()
	var tokens auth.TokenPair
	err = s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		session, err := s.sessionsRepository.RotateSession(
			ctx,
			rClaims.SessionID,
			rClaims.TokenID,
			newTokenID,
			usedAt,
		)

		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return auth.ErrInvalidToken
			}
			return fmt.Errorf("rotate: %w", err)
		}

		user, err := s.usersRepository.GetUser(ctx, session.UserID)

		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return auth.ErrInvalidToken
			}
			return fmt.Errorf("get user: %w", err)
		}
		if user.DeletedAt != nil {
			return auth.ErrInvalidToken
		}

		tokens, err = s.generateTokenPair(session, usedAt)
		if err != nil {
			return fmt.Errorf("generate token pair: %w", err)
		}

		return nil
	})
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf(
			"transaction: %w",
			err,
		)
	}

	return tokens, nil
}
