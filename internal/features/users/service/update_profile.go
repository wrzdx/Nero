package users_service

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	core_types "github.com/wrzdx/Nero/internal/core/types"

	"github.com/google/uuid"
)

func (s *UsersService) UpdateProfile(
	ctx context.Context,
	userID uuid.UUID,
	command UpdateProfileCommand,
) (domain.User, error) {
	if command.isEmpty() {
		user, err := s.GetUser(ctx, userID)
		if err != nil {
			return domain.User{}, err
		}
		return user, nil
	}
	var result domain.User
	err := s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		user, err := s.userRepository.GetUserForUpdate(ctx, userID)
		if err != nil {
			return fmt.Errorf("get user for update: %w", err)
		}
		if user.DeletedAt != nil {
			return domain.ErrNotFound
		}
		updatedProfile, err := applyProfileUpdate(user.Profile, command)
		if err != nil {
			return fmt.Errorf("update profile: %w", err)
		}
		if err := s.userRepository.UpdateUserProfile(ctx, userID, updatedProfile); err != nil {
			return fmt.Errorf("update profile repo: %w", err)
		}
		user.Profile = updatedProfile
		result = user
		return nil
	})

	if err != nil {
		return domain.User{}, fmt.Errorf("transaction: %w", err)
	}
	return result, nil
}

type UpdateProfileCommand struct {
	Username  *string
	FirstName *string
	LastName  core_types.Nullable[string]
	Bio       core_types.Nullable[string]
}

func (c UpdateProfileCommand) isEmpty() bool {
	return c.Username == nil && c.FirstName == nil && !c.LastName.Set && !c.Bio.Set
}

func applyProfileUpdate(
	profile domain.UserProfile,
	command UpdateProfileCommand,
) (domain.UserProfile, error) {
	username := profile.Username
	firstName := profile.FirstName
	lastname := profile.LastName
	bio := profile.Bio
	if command.Username != nil {
		username = *command.Username
	}
	if command.FirstName != nil {
		firstName = *command.FirstName
	}
	if command.LastName.Set {
		lastname = command.LastName.Value
	}
	if command.Bio.Set {
		bio = command.Bio.Value
	}

	return domain.NewUserProfile(username, firstName, lastname, bio)
}
