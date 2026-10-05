package users_service

import (
	"errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/wrzdx/Nero/internal/core/domain"
	"testing"
	"time"
)

func TestSearchUsers(t *testing.T) {
	t.Run("normalizes prefix and bounds public results", func(t *testing.T) {
		user := newGetUserTestUser(t, nil)
		repo := NewMockUsersRepository(t)
		repo.EXPECT().GetUser(t.Context(), user.ID).Return(user, nil)
		expected := []UserSearchResult{{ID: uuid.New(), Username: "Daniel_k", FirstName: "Даниил"}}
		repo.EXPECT().SearchUsers(t.Context(), user.ID, "dan", 10).Return(expected, nil)
		actual, err := NewUsersService(repo, nil, nil).SearchUsers(t.Context(), user.ID, SearchUsersQuery{Prefix: " @DaN "})
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})
	t.Run("short prefix never scans users", func(t *testing.T) {
		user := newGetUserTestUser(t, nil)
		repo := NewMockUsersRepository(t)
		repo.EXPECT().GetUser(t.Context(), user.ID).Return(user, nil)
		actual, err := NewUsersService(repo, nil, nil).SearchUsers(t.Context(), user.ID, SearchUsersQuery{Prefix: "@a"})
		require.NoError(t, err)
		require.NotNil(t, actual)
		require.Empty(t, actual)
	})
	for _, query := range []SearchUsersQuery{{Prefix: "da%"}, {Prefix: "da\\"}, {Prefix: "дан"}, {Prefix: "dan", Limit: 21}, {Prefix: "dan", Limit: -1}} {
		t.Run("reject invalid query", func(t *testing.T) {
			_, err := NewUsersService(NewMockUsersRepository(t), nil, nil).SearchUsers(t.Context(), uuid.New(), query)
			require.ErrorIs(t, err, ErrInvalidSearchQuery)
		})
	}
	t.Run("deleted requester cannot search with an unexpired token", func(t *testing.T) {
		now := time.Now()
		user := newGetUserTestUser(t, &now)
		repo := NewMockUsersRepository(t)
		repo.EXPECT().GetUser(t.Context(), user.ID).Return(user, nil)
		_, err := NewUsersService(repo, nil, nil).SearchUsers(t.Context(), user.ID, SearchUsersQuery{Prefix: "dan"})
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
	t.Run("propagates infrastructure failure", func(t *testing.T) {
		user := newGetUserTestUser(t, nil)
		repo := NewMockUsersRepository(t)
		repo.EXPECT().GetUser(t.Context(), user.ID).Return(user, nil)
		dbErr := errors.New("database down")
		repo.EXPECT().SearchUsers(t.Context(), user.ID, "dan", 10).Return(nil, dbErr)
		_, err := NewUsersService(repo, nil, nil).SearchUsers(t.Context(), user.ID, SearchUsersQuery{Prefix: "dan"})
		require.ErrorIs(t, err, dbErr)
	})
}
