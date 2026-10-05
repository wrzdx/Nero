package auth_service

import (
	"context"
	"errors"
	"testing"

	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestChangePassword(t *testing.T) {
	const (
		currentPassword = "current password value"
		newPassword     = "new password value"
		newPasswordHash = "new-password-hash"
	)

	t.Run("changes password with CAS and deletes all sessions", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		hasher := NewMockHasher(t)
		txManager := NewMockTXManager(t)
		user := newLoginTestUser(t, false)

		usersRepository.EXPECT().GetUser(mock.Anything, user.ID).Return(user, nil)
		hasher.EXPECT().Compare(user.PasswordHash, currentPassword).Return(nil)
		hasher.EXPECT().Hash(newPassword).Return(newPasswordHash, nil)
		usersRepository.EXPECT().
			ChangePassword(mock.Anything, user.ID, newPasswordHash, user.PasswordHash).
			Return(nil)
		sessionsRepository.EXPECT().DeleteAllSessions(mock.Anything, user.ID).Return(nil)
		txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			RunAndReturn(runChangePasswordTransaction)
		service := &AuthService{
			usersRepository:    usersRepository,
			sessionsRepository: sessionsRepository,
			hasher:             hasher,
			txManager:          txManager,
		}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, newPassword)

		require.NoError(t, err)
	})

	t.Run("rejects unchanged password before using dependencies", func(t *testing.T) {
		service := &AuthService{}

		err := service.ChangePassword(t.Context(), newLoginTestUser(t, false).ID, currentPassword, currentPassword)

		require.ErrorIs(t, err, auth.ErrInvalidPassword)
		require.Equal(t, map[string]string{
			"new_password": "new password should differ from current",
		}, detailedErrorFields(t, err))
	})

	t.Run("returns invalid token for missing user and performs dummy compare", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		hasher := NewMockHasher(t)
		userID := newLoginTestUser(t, false).ID
		usersRepository.EXPECT().GetUser(mock.Anything, userID).Return(domain.User{}, domain.ErrNotFound)
		hasher.EXPECT().DummyCompare().Return()
		service := &AuthService{usersRepository: usersRepository, hasher: hasher}

		err := service.ChangePassword(t.Context(), userID, currentPassword, newPassword)

		require.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("returns invalid token for deleted user and performs dummy compare", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		hasher := NewMockHasher(t)
		user := newLoginTestUser(t, true)
		usersRepository.EXPECT().GetUser(mock.Anything, user.ID).Return(user, nil)
		hasher.EXPECT().DummyCompare().Return()
		service := &AuthService{usersRepository: usersRepository, hasher: hasher}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, newPassword)

		require.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("returns unexpected user repository error", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		getErr := errors.New("database unavailable")
		userID := newLoginTestUser(t, false).ID
		usersRepository.EXPECT().GetUser(mock.Anything, userID).Return(domain.User{}, getErr)
		service := &AuthService{usersRepository: usersRepository}

		err := service.ChangePassword(t.Context(), userID, currentPassword, newPassword)

		require.ErrorIs(t, err, getErr)
	})

	t.Run("returns current password mismatch with field details", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		hasher := NewMockHasher(t)
		user := newLoginTestUser(t, false)
		usersRepository.EXPECT().GetUser(mock.Anything, user.ID).Return(user, nil)
		hasher.EXPECT().Compare(user.PasswordHash, currentPassword).Return(auth.ErrPasswordMismatch)
		service := &AuthService{usersRepository: usersRepository, hasher: hasher}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, newPassword)

		require.ErrorIs(t, err, auth.ErrPasswordMismatch)
		require.Equal(t, map[string]string{
			"current_password": auth.ErrPasswordMismatch.Error(),
		}, detailedErrorFields(t, err))
	})

	t.Run("returns unexpected password comparison error", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		hasher := NewMockHasher(t)
		compareErr := errors.New("hasher unavailable")
		user := newLoginTestUser(t, false)
		usersRepository.EXPECT().GetUser(mock.Anything, user.ID).Return(user, nil)
		hasher.EXPECT().Compare(user.PasswordHash, currentPassword).Return(compareErr)
		service := &AuthService{usersRepository: usersRepository, hasher: hasher}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, newPassword)

		require.ErrorIs(t, err, compareErr)
	})

	t.Run("returns new password validation details before hashing", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		hasher := NewMockHasher(t)
		user := newLoginTestUser(t, false)
		usersRepository.EXPECT().GetUser(mock.Anything, user.ID).Return(user, nil)
		hasher.EXPECT().Compare(user.PasswordHash, currentPassword).Return(nil)
		service := &AuthService{usersRepository: usersRepository, hasher: hasher}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, "too short")

		require.ErrorIs(t, err, auth.ErrInvalidPassword)
		require.Equal(t, map[string]string{
			"new_password": "password must be at least 15 characters",
		}, detailedErrorFields(t, err))
	})

	t.Run("returns password hashing error before transaction", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		hasher := NewMockHasher(t)
		hashErr := errors.New("hash failure")
		user := newLoginTestUser(t, false)
		usersRepository.EXPECT().GetUser(mock.Anything, user.ID).Return(user, nil)
		hasher.EXPECT().Compare(user.PasswordHash, currentPassword).Return(nil)
		hasher.EXPECT().Hash(newPassword).Return("", hashErr)
		service := &AuthService{usersRepository: usersRepository, hasher: hasher}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, newPassword)

		require.ErrorIs(t, err, hashErr)
	})

	t.Run("returns invalid token when CAS update misses", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		hasher := NewMockHasher(t)
		txManager := NewMockTXManager(t)
		user := newLoginTestUser(t, false)
		expectChangePasswordPrerequisites(usersRepository, hasher, user, currentPassword, newPassword, newPasswordHash)
		usersRepository.EXPECT().
			ChangePassword(mock.Anything, user.ID, newPasswordHash, user.PasswordHash).
			Return(domain.ErrNotFound)
		txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			RunAndReturn(runChangePasswordTransaction)
		service := &AuthService{
			usersRepository:    usersRepository,
			sessionsRepository: sessionsRepository,
			hasher:             hasher,
			txManager:          txManager,
		}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, newPassword)

		require.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("returns unexpected password update error", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		hasher := NewMockHasher(t)
		txManager := NewMockTXManager(t)
		updateErr := errors.New("update failure")
		user := newLoginTestUser(t, false)
		expectChangePasswordPrerequisites(usersRepository, hasher, user, currentPassword, newPassword, newPasswordHash)
		usersRepository.EXPECT().
			ChangePassword(mock.Anything, user.ID, newPasswordHash, user.PasswordHash).
			Return(updateErr)
		txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			RunAndReturn(runChangePasswordTransaction)
		service := &AuthService{usersRepository: usersRepository, hasher: hasher, txManager: txManager}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, newPassword)

		require.ErrorIs(t, err, updateErr)
	})

	t.Run("returns session deletion error so transaction can roll back", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		hasher := NewMockHasher(t)
		txManager := NewMockTXManager(t)
		deleteErr := errors.New("delete sessions failure")
		user := newLoginTestUser(t, false)
		expectChangePasswordPrerequisites(usersRepository, hasher, user, currentPassword, newPassword, newPasswordHash)
		usersRepository.EXPECT().
			ChangePassword(mock.Anything, user.ID, newPasswordHash, user.PasswordHash).
			Return(nil)
		sessionsRepository.EXPECT().DeleteAllSessions(mock.Anything, user.ID).Return(deleteErr)
		txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			RunAndReturn(runChangePasswordTransaction)
		service := &AuthService{
			usersRepository:    usersRepository,
			sessionsRepository: sessionsRepository,
			hasher:             hasher,
			txManager:          txManager,
		}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, newPassword)

		require.ErrorIs(t, err, deleteErr)
	})

	t.Run("returns transaction manager error", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		hasher := NewMockHasher(t)
		txManager := NewMockTXManager(t)
		txErr := errors.New("begin transaction failure")
		user := newLoginTestUser(t, false)
		expectChangePasswordPrerequisites(usersRepository, hasher, user, currentPassword, newPassword, newPasswordHash)
		txManager.EXPECT().WithinTransaction(mock.Anything, mock.Anything).Return(txErr)
		service := &AuthService{usersRepository: usersRepository, hasher: hasher, txManager: txManager}

		err := service.ChangePassword(t.Context(), user.ID, currentPassword, newPassword)

		require.ErrorIs(t, err, txErr)
	})
}

func expectChangePasswordPrerequisites(
	usersRepository *MockUsersRepository,
	hasher *MockHasher,
	user domain.User,
	currentPassword string,
	newPassword string,
	newPasswordHash string,
) {
	usersRepository.EXPECT().GetUser(mock.Anything, user.ID).Return(user, nil)
	hasher.EXPECT().Compare(user.PasswordHash, currentPassword).Return(nil)
	hasher.EXPECT().Hash(newPassword).Return(newPasswordHash, nil)
}

func runChangePasswordTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func detailedErrorFields(t *testing.T, err error) map[string]string {
	t.Helper()
	var detailed domain.DetailedError
	require.ErrorAs(t, err, &detailed)
	return detailed.Fields()
}
