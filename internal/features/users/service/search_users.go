package users_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidSearchQuery = errors.New("invalid user search query")

type SearchUsersQuery struct {
	Prefix string
	Limit  int
}

// UserSearchResult contains only public fields needed to select a participant.
type UserSearchResult struct {
	ID        uuid.UUID
	Username  string
	FirstName string
	LastName  *string
}

func (s *UsersService) SearchUsers(ctx context.Context, requesterID uuid.UUID, query SearchUsersQuery) ([]UserSearchResult, error) {
	if requesterID == uuid.Nil {
		return nil, domain.ErrNotFound
	}
	query.Prefix = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(query.Prefix), "@"))
	if query.Limit == 0 {
		query.Limit = 10
	}
	fields := map[string]string{}
	if query.Limit < 1 || query.Limit > 20 {
		fields["limit"] = "must be between 1 and 20"
	}
	if len(query.Prefix) > 32 {
		fields["q"] = "must contain at most 32 ASCII letters, digits or underscores"
	}
	for _, c := range query.Prefix {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			fields["q"] = "must contain ASCII letters, digits or underscores"
			break
		}
	}
	if len(fields) > 0 {
		return nil, domain.DetailedError{Err: ErrInvalidSearchQuery, Details: fields}
	}
	if _, err := s.GetUser(ctx, requesterID); err != nil {
		return nil, err
	}
	if len(query.Prefix) < 2 {
		return []UserSearchResult{}, nil
	}
	users, err := s.userRepository.SearchUsers(ctx, requesterID, query.Prefix, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	if users == nil {
		users = []UserSearchResult{}
	}
	return users, nil
}
