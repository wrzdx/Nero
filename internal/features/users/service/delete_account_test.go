package users_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDeleteAccount(t *testing.T) {
	t.Run("anonymizes user and deletes sessions in one transaction", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		txManager := NewMockTXManager(t)
		user := newUpdateProfileTestUser(t, nil)
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteAccountTxContextKey{}, "tx")
		var savedProfile domain.UserProfile
		var deletedAt time.Time
		usersRepository.EXPECT().GetUserForUpdate(txCtx, user.ID).Return(user, nil)
		usersRepository.EXPECT().
			DeleteUser(txCtx, user.ID, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ uuid.UUID, profile domain.UserProfile, at time.Time) {
				savedProfile = profile
				deletedAt = at
			}).
			Return(nil)
		sessionsRepository.EXPECT().DeleteAllSessions(txCtx, user.ID).Return(nil)
		expectDeleteAccountTransaction(txManager, outerCtx, txCtx)
		service := NewUsersService(usersRepository, sessionsRepository, txManager)
		before := time.Now()

		err := service.DeleteAccount(outerCtx, user.ID)

		after := time.Now()
		require.NoError(t, err)
		require.False(t, deletedAt.Before(before))
		require.False(t, deletedAt.After(after))
		expectedDeletedUser, err := user.Delete(deletedAt)
		require.NoError(t, err)
		require.Equal(t, expectedDeletedUser.Profile, savedProfile)
	})

	t.Run("returns user repository error", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		txManager := NewMockTXManager(t)
		userID := uuid.New()
		getErr := errors.New("get user failure")
		usersRepository.EXPECT().GetUserForUpdate(mock.Anything, userID).Return(domain.User{}, getErr)
		expectDeleteAccountTransaction(txManager, t.Context(), t.Context())
		service := NewUsersService(usersRepository, sessionsRepository, txManager)

		err := service.DeleteAccount(t.Context(), userID)

		require.ErrorIs(t, err, getErr)
	})

	t.Run("returns not found for already deleted user", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		txManager := NewMockTXManager(t)
		deletedAt := time.Now().UTC()
		user := newUpdateProfileTestUser(t, &deletedAt)
		usersRepository.EXPECT().GetUserForUpdate(mock.Anything, user.ID).Return(user, nil)
		expectDeleteAccountTransaction(txManager, t.Context(), t.Context())
		service := NewUsersService(usersRepository, sessionsRepository, txManager)

		err := service.DeleteAccount(t.Context(), user.ID)

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("does not persist invalid deletion transition", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		txManager := NewMockTXManager(t)
		profile := newUpdateProfileTestProfile(t, "Username_1", "First name", nil, nil)
		user, err := domain.NewUser(
			uuid.New(),
			profile,
			time.Now().Add(time.Hour),
			nil,
			"password-hash",
		)
		require.NoError(t, err)
		usersRepository.EXPECT().GetUserForUpdate(mock.Anything, user.ID).Return(user, nil)
		expectDeleteAccountTransaction(txManager, t.Context(), t.Context())
		service := NewUsersService(usersRepository, sessionsRepository, txManager)

		err = service.DeleteAccount(t.Context(), user.ID)

		require.ErrorIs(t, err, domain.ErrInvalidUser)
	})

	t.Run("returns user deletion error without deleting sessions", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		txManager := NewMockTXManager(t)
		user := newUpdateProfileTestUser(t, nil)
		deleteErr := errors.New("delete user failure")
		usersRepository.EXPECT().GetUserForUpdate(mock.Anything, user.ID).Return(user, nil)
		usersRepository.EXPECT().
			DeleteUser(mock.Anything, user.ID, mock.Anything, mock.Anything).
			Return(deleteErr)
		expectDeleteAccountTransaction(txManager, t.Context(), t.Context())
		service := NewUsersService(usersRepository, sessionsRepository, txManager)

		err := service.DeleteAccount(t.Context(), user.ID)

		require.ErrorIs(t, err, deleteErr)
	})

	t.Run("returns session deletion error so transaction can roll back", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		txManager := NewMockTXManager(t)
		user := newUpdateProfileTestUser(t, nil)
		deleteSessionsErr := errors.New("delete sessions failure")
		usersRepository.EXPECT().GetUserForUpdate(mock.Anything, user.ID).Return(user, nil)
		usersRepository.EXPECT().
			DeleteUser(mock.Anything, user.ID, mock.Anything, mock.Anything).
			Return(nil)
		sessionsRepository.EXPECT().
			DeleteAllSessions(mock.Anything, user.ID).
			Return(deleteSessionsErr)
		expectDeleteAccountTransaction(txManager, t.Context(), t.Context())
		service := NewUsersService(usersRepository, sessionsRepository, txManager)

		err := service.DeleteAccount(t.Context(), user.ID)

		require.ErrorIs(t, err, deleteSessionsErr)
	})

	t.Run("returns transaction manager error before repositories are used", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		txManager := NewMockTXManager(t)
		txErr := errors.New("begin transaction failure")
		txManager.EXPECT().WithinTransaction(t.Context(), mock.Anything).Return(txErr)
		service := NewUsersService(usersRepository, sessionsRepository, txManager)

		err := service.DeleteAccount(t.Context(), uuid.New())

		require.ErrorIs(t, err, txErr)
	})
}

type deleteAccountTxContextKey struct{}

func expectDeleteAccountTransaction(
	txManager *MockTXManager,
	outerCtx context.Context,
	txCtx context.Context,
) {
	txManager.EXPECT().
		WithinTransaction(outerCtx, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			return fn(txCtx)
		})
}
