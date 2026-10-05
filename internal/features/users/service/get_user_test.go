package users_service

import (
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetUser(t *testing.T) {
	t.Run("returns active user", func(t *testing.T) {
		repository := NewMockUsersRepository(t)
		user := newGetUserTestUser(t, nil)
		ctx := t.Context()
		repository.EXPECT().
			GetUser(ctx, user.ID).
			Return(user, nil)
		txManager := NewMockTXManager(t)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		got, err := service.GetUser(ctx, user.ID)

		require.NoError(t, err)
		require.Equal(t, user, got)
	})

	t.Run("returns not found when repository cannot find user", func(t *testing.T) {
		repository := NewMockUsersRepository(t)
		userID := uuid.New()
		ctx := t.Context()
		repository.EXPECT().
			GetUser(ctx, userID).
			Return(domain.User{}, domain.ErrNotFound)
		txManager := NewMockTXManager(t)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		user, err := service.GetUser(ctx, userID)

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, user)
	})

	t.Run("hides deleted user", func(t *testing.T) {
		repository := NewMockUsersRepository(t)
		deletedAt := time.Now().UTC()
		user := newGetUserTestUser(t, &deletedAt)
		ctx := t.Context()
		repository.EXPECT().
			GetUser(ctx, user.ID).
			Return(user, nil)
		txManager := NewMockTXManager(t)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		got, err := service.GetUser(ctx, user.ID)

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, got)
	})

	t.Run("wraps repository error", func(t *testing.T) {
		repository := NewMockUsersRepository(t)
		userID := uuid.New()
		repositoryErr := errors.New("database unavailable")
		ctx := t.Context()
		repository.EXPECT().
			GetUser(ctx, userID).
			Return(domain.User{}, repositoryErr)
		txManager := NewMockTXManager(t)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		user, err := service.GetUser(ctx, userID)

		require.ErrorIs(t, err, repositoryErr)
		require.ErrorContains(t, err, "get user")
		require.Empty(t, user)
	})
}

func newGetUserTestUser(t *testing.T, deletedAt *time.Time) domain.User {
	t.Helper()

	profile, err := domain.NewUserProfile("Username_1", "First name", nil, nil)
	require.NoError(t, err)

	user, err := domain.NewUser(
		uuid.New(),
		profile,
		time.Now().UTC().Add(-time.Hour),
		deletedAt,
		"password-hash",
	)
	require.NoError(t, err)

	return user
}
