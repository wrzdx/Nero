package auth_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
)

func (s *AuthService) Logout(
	ctx context.Context,
	refreshToken string,
) error {
	rClaims, err := s.tokenProvider.ParseRefreshToken(refreshToken)
	if err != nil {
		return fmt.Errorf("parse refresh: %w", err)
	}

	if err := s.sessionsRepository.DeleteSession(ctx, rClaims.SessionID, rClaims.TokenID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
