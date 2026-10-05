package users_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	core_types "github.com/wrzdx/Nero/internal/core/types"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestUpdateProfile(t *testing.T) {
	t.Run("returns current user for empty update without transaction", func(t *testing.T) {
		user := newUpdateProfileTestUser(t, nil)
		repository := NewMockUsersRepository(t)
		repository.EXPECT().GetUser(t.Context(), user.ID).Return(user, nil)
		txManager := NewMockTXManager(t)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		actual, err := service.UpdateProfile(t.Context(), user.ID, UpdateProfileCommand{})

		require.NoError(t, err)
		require.Equal(t, user, actual)
	})

	t.Run("updates and normalizes profile in transaction", func(t *testing.T) {
		user := newUpdateProfileTestUser(t, nil)
		username := "  Updated_user  "
		firstName := "  Updated name  "
		lastName := "  Updated surname  "
		bio := "  Updated bio  "
		command := UpdateProfileCommand{
			Username:  &username,
			FirstName: &firstName,
			LastName:  core_types.Nullable[string]{Set: true, Value: &lastName},
			Bio:       core_types.Nullable[string]{Set: true, Value: &bio},
		}
		expectedProfile := newUpdateProfileTestProfile(
			t,
			"Updated_user",
			"Updated name",
			ptrForUpdateProfileTest("Updated surname"),
			ptrForUpdateProfileTest("Updated bio"),
		)
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, updateProfileTxContextKey{}, "transaction")
		repository := NewMockUsersRepository(t)
		repository.EXPECT().GetUserForUpdate(txCtx, user.ID).Return(user, nil)
		repository.EXPECT().UpdateUserProfile(txCtx, user.ID, expectedProfile).Return(nil)
		txManager := NewMockTXManager(t)
		expectUpdateProfileTransaction(txManager, outerCtx, txCtx)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		actual, err := service.UpdateProfile(outerCtx, user.ID, command)

		require.NoError(t, err)
		user.Profile = expectedProfile
		require.Equal(t, user, actual)
	})

	t.Run("clears nullable field and preserves omitted fields", func(t *testing.T) {
		user := newUpdateProfileTestUser(t, nil)
		command := UpdateProfileCommand{
			Bio: core_types.Nullable[string]{Set: true},
		}
		expectedProfile := newUpdateProfileTestProfile(
			t,
			user.Profile.Username,
			user.Profile.FirstName,
			user.Profile.LastName,
			nil,
		)
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, updateProfileTxContextKey{}, "transaction")
		repository := NewMockUsersRepository(t)
		repository.EXPECT().GetUserForUpdate(txCtx, user.ID).Return(user, nil)
		repository.EXPECT().UpdateUserProfile(txCtx, user.ID, expectedProfile).Return(nil)
		txManager := NewMockTXManager(t)
		expectUpdateProfileTransaction(txManager, outerCtx, txCtx)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		actual, err := service.UpdateProfile(outerCtx, user.ID, command)

		require.NoError(t, err)
		user.Profile = expectedProfile
		require.Equal(t, user, actual)
	})

	t.Run("returns not found for deleted user without updating", func(t *testing.T) {
		deletedAt := time.Now().UTC()
		user := newUpdateProfileTestUser(t, &deletedAt)
		username := "Updated_user"
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, updateProfileTxContextKey{}, "transaction")
		repository := NewMockUsersRepository(t)
		repository.EXPECT().GetUserForUpdate(txCtx, user.ID).Return(user, nil)
		txManager := NewMockTXManager(t)
		expectUpdateProfileTransaction(txManager, outerCtx, txCtx)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		actual, err := service.UpdateProfile(outerCtx, user.ID, UpdateProfileCommand{
			Username: &username,
		})

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, actual)
	})

	t.Run("wraps locked user lookup error", func(t *testing.T) {
		userID := uuid.New()
		repositoryErr := errors.New("database unavailable")
		username := "Updated_user"
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, updateProfileTxContextKey{}, "transaction")
		repository := NewMockUsersRepository(t)
		repository.EXPECT().
			GetUserForUpdate(txCtx, userID).
			Return(domain.User{}, repositoryErr)
		txManager := NewMockTXManager(t)
		expectUpdateProfileTransaction(txManager, outerCtx, txCtx)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		actual, err := service.UpdateProfile(outerCtx, userID, UpdateProfileCommand{
			Username: &username,
		})

		require.ErrorIs(t, err, repositoryErr)
		require.ErrorContains(t, err, "get user for update")
		require.Empty(t, actual)
	})

	t.Run("rejects invalid resulting profile without updating", func(t *testing.T) {
		user := newUpdateProfileTestUser(t, nil)
		invalidUsername := "!"
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, updateProfileTxContextKey{}, "transaction")
		repository := NewMockUsersRepository(t)
		repository.EXPECT().GetUserForUpdate(txCtx, user.ID).Return(user, nil)
		txManager := NewMockTXManager(t)
		expectUpdateProfileTransaction(txManager, outerCtx, txCtx)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		actual, err := service.UpdateProfile(outerCtx, user.ID, UpdateProfileCommand{
			Username: &invalidUsername,
		})

		require.ErrorIs(t, err, domain.ErrInvalidUserProfile)
		require.Empty(t, actual)
	})

	t.Run("wraps profile update repository error", func(t *testing.T) {
		user := newUpdateProfileTestUser(t, nil)
		username := "Updated_user"
		expectedProfile := newUpdateProfileTestProfile(
			t,
			username,
			user.Profile.FirstName,
			user.Profile.LastName,
			user.Profile.Bio,
		)
		repositoryErr := errors.New("update failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, updateProfileTxContextKey{}, "transaction")
		repository := NewMockUsersRepository(t)
		repository.EXPECT().GetUserForUpdate(txCtx, user.ID).Return(user, nil)
		repository.EXPECT().
			UpdateUserProfile(txCtx, user.ID, expectedProfile).
			Return(repositoryErr)
		txManager := NewMockTXManager(t)
		expectUpdateProfileTransaction(txManager, outerCtx, txCtx)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		actual, err := service.UpdateProfile(outerCtx, user.ID, UpdateProfileCommand{
			Username: &username,
		})

		require.ErrorIs(t, err, repositoryErr)
		require.ErrorContains(t, err, "update profile repo")
		require.Empty(t, actual)
	})

	t.Run("wraps transaction manager error", func(t *testing.T) {
		userID := uuid.New()
		username := "Updated_user"
		transactionErr := errors.New("cannot begin transaction")
		repository := NewMockUsersRepository(t)
		txManager := NewMockTXManager(t)
		txManager.EXPECT().
			WithinTransaction(t.Context(), mock.Anything).
			Return(transactionErr)
		sessionsRepo := NewMockSessionsRepository(t)
		service := NewUsersService(repository, sessionsRepo, txManager)

		actual, err := service.UpdateProfile(t.Context(), userID, UpdateProfileCommand{
			Username: &username,
		})

		require.ErrorIs(t, err, transactionErr)
		require.ErrorContains(t, err, "transaction")
		require.Empty(t, actual)
	})
}

type updateProfileTxContextKey struct{}

func expectUpdateProfileTransaction(
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

func newUpdateProfileTestUser(t *testing.T, deletedAt *time.Time) domain.User {
	t.Helper()

	profile := newUpdateProfileTestProfile(
		t,
		"Username_1",
		"First name",
		ptrForUpdateProfileTest("Last name"),
		ptrForUpdateProfileTest("Current bio"),
	)
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

func newUpdateProfileTestProfile(
	t *testing.T,
	username string,
	firstName string,
	lastName *string,
	bio *string,
) domain.UserProfile {
	t.Helper()

	profile, err := domain.NewUserProfile(username, firstName, lastName, bio)
	require.NoError(t, err)
	return profile
}

func ptrForUpdateProfileTest(value string) *string {
	return &value
}
