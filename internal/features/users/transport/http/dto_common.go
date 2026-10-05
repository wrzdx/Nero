package users_transport_http

import (
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
)

type UserDTOResponse struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username" example:"qwerty"`
	FirstName string    `json:"first_name" example:"Ivan"`
	LastName  *string   `json:"last_name" example:"Ivanov"`
	Bio       *string   `json:"bio" example:"We didn't choose this path. Circumstance chose it for us. We're simply trying to keep climbing."`
}

func userDTOFromDomain(user domain.User) UserDTOResponse {
	return UserDTOResponse{
		ID:        user.ID,
		Username:  user.Profile.Username,
		FirstName: user.Profile.FirstName,
		LastName:  user.Profile.LastName,
		Bio:       user.Profile.Bio,
	}
}

func usersDTOFromDomains(users []domain.User) []UserDTOResponse {
	usersDTO := make([]UserDTOResponse, len(users))

	for i, user := range users {
		usersDTO[i] = userDTOFromDomain(user)
	}

	return usersDTO
}
