package users_service

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
)

func (s *UsersService) GetUser(
	ctx context.Context,
	id uuid.UUID,
) (domain.User, error) {
	user, err := s.userRepository.GetUser(ctx, id)
	if err != nil {
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}

	if user.DeletedAt != nil {
		return domain.User{}, domain.ErrNotFound
	}

	return user, nil
}
