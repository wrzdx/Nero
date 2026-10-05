package users_postgres_repository

import (
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

type UserModel struct {
	ID           uuid.UUID
	Username     string
	FirstName    string
	LastName     *string
	CreatedAt    time.Time
	DeletedAt    *time.Time
	Bio          *string
	PasswordHash string
}

func UserDomainFromModel(user UserModel) (domain.User, error) {
	profile, err := domain.NewUserProfile(user.Username, user.FirstName, user.LastName, user.Bio)
	if err != nil {
		return domain.User{}, fmt.Errorf("new profile from model: %w", err)
	}
	domainUser, err := domain.NewUser(
		user.ID,
		profile,
		user.CreatedAt,
		user.DeletedAt,
		user.PasswordHash,
	)
	if err != nil {
		return domain.User{}, fmt.Errorf("new user from model: %w", err)
	}
	return domainUser, nil
}

func userDomainsFromModels(users []UserModel) ([]domain.User, error) {
	userDomains := make([]domain.User, len(users))
	for i, user := range users {
		userDomain, err := UserDomainFromModel(user)
		if err != nil {
			return nil, err
		}
		userDomains[i] = userDomain
	}

	return userDomains, nil
}
