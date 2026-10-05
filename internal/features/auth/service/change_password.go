package auth_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
)

func (s *AuthService) ChangePassword(
	ctx context.Context,
	userID uuid.UUID,
	currentPassword string,
	newPassword string,
) error {
	if currentPassword == newPassword {
		return domain.DetailedError{
			Err:     auth.ErrInvalidPassword,
			Details: map[string]string{"new_password": "new password should differ from current"},
		}
	}
	user, err := s.usersRepository.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.hasher.DummyCompare()
			return auth.ErrInvalidToken
		}
		return fmt.Errorf("get user from repo: %w", err)
	}
	if user.DeletedAt != nil {
		s.hasher.DummyCompare()
		return auth.ErrInvalidToken
	}

	if err := s.hasher.Compare(user.PasswordHash, currentPassword); err != nil {
		if errors.Is(err, auth.ErrPasswordMismatch) {
			return domain.DetailedError{
				Err:     err,
				Details: map[string]string{"current_password": err.Error()},
			}
		}
		return fmt.Errorf(
			"compare passwords: %w",
			err,
		)
	}
	if err := auth.ValidatePassword(newPassword); err != nil {
		return domain.DetailedError{
			Err:     err,
			Details: map[string]string{"new_password": err.Error()},
		}
	}
	passwordHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	err = s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.usersRepository.ChangePassword(ctx, userID, passwordHash, user.PasswordHash); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return auth.ErrInvalidToken
			}
			return fmt.Errorf("change password: %w", err)
		}
		if err := s.sessionsRepository.DeleteAllSessions(ctx, userID); err != nil {
			return fmt.Errorf("delete all sessions: %w", err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("transaction: %w", err)
	}

	return nil
}
