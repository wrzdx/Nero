package users_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s *UsersService) DeleteAccount(
	ctx context.Context,
	userID uuid.UUID,
) error {
	err := s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		user, err := s.userRepository.GetUserForUpdate(ctx, userID)
		if err != nil {
			return fmt.Errorf("get user for delete: %w", err)
		}
		now := time.Now()
		deletedUser, err := user.Delete(now)
		if err != nil {
			if errors.Is(err, domain.ErrAlreadyDeleted) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("delete user: %w", err)
		}
		if err := s.userRepository.DeleteUser(ctx, userID, deletedUser.Profile, now); err != nil {
			return fmt.Errorf("update profile repo: %w", err)
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
