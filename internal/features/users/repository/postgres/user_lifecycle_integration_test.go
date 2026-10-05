//go:build integration

package users_postgres_repository

import (
	"strings"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateUser(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("persists user", func(t *testing.T) {
		_, repository := newGetUserTestRepository(t, pool, config.Timeout)
		expected := newRepositoryTestUserWithUsername(t, "Created_"+uuid.NewString()[:8])

		err := repository.CreateUser(t.Context(), expected)

		require.NoError(t, err)
		actual, err := repository.GetUser(t.Context(), expected.ID)
		require.NoError(t, err)
		requireGetUserTestUserEqual(t, expected, actual)
	})

	t.Run("maps case-insensitive username conflict", func(t *testing.T) {
		_, repository := newGetUserTestRepository(t, pool, config.Timeout)
		username := "Conflict_" + uuid.NewString()[:8]
		first := newRepositoryTestUserWithUsername(t, username)
		second := newRepositoryTestUserWithUsername(t, strings.ToLower(username))
		require.NoError(t, repository.CreateUser(t.Context(), first))

		err := repository.CreateUser(t.Context(), second)

		require.ErrorIs(t, err, domain.ErrAlreadyExists)
		var detailed domain.DetailedError
		require.ErrorAs(t, err, &detailed)
		require.Equal(t, map[string]string{
			"username": "username already taken",
		}, detailed.Fields())
	})
}

func TestGetUserByUsername(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("finds username case-insensitively", func(t *testing.T) {
		tx, repository := newGetUserTestRepository(t, pool, config.Timeout)
		expected := newRepositoryTestUserWithUsername(t, "MixedCase_"+uuid.NewString()[:8])
		insertGetUserTestUser(t, tx, expected)

		actual, err := repository.GetUserByUsername(
			t.Context(),
			strings.ToLower(expected.Profile.Username),
		)

		require.NoError(t, err)
		requireGetUserTestUserEqual(t, expected, actual)
	})

	t.Run("returns not found for unknown username", func(t *testing.T) {
		_, repository := newGetUserTestRepository(t, pool, config.Timeout)

		user, err := repository.GetUserByUsername(t.Context(), "Unknown_user")

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, user)
	})
}

func TestDeleteUser(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("anonymizes active user and rejects repeated deletion", func(t *testing.T) {
		tx, repository := newGetUserTestRepository(t, pool, config.Timeout)
		lastName := "Anderson"
		bio := "User to delete"
		user := newGetUserRepositoryTestUser(t, &lastName, &bio, nil)
		insertGetUserTestUser(t, tx, user)
		deletedAt := user.CreatedAt.Add(time.Second)
		deletedState, err := user.Delete(deletedAt)
		require.NoError(t, err)

		err = repository.DeleteUser(
			t.Context(),
			user.ID,
			deletedState.Profile,
			deletedAt,
		)

		require.NoError(t, err)
		deleted, err := repository.GetUser(t.Context(), user.ID)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(deleted.Profile.Username, "deleted_"))
		require.Equal(t, "Deleted Account", deleted.Profile.FirstName)
		require.Nil(t, deleted.Profile.LastName)
		require.Nil(t, deleted.Profile.Bio)
		require.NotNil(t, deleted.DeletedAt)
		require.Equal(t, user.PasswordHash, deleted.PasswordHash)

		err = repository.DeleteUser(
			t.Context(),
			user.ID,
			deletedState.Profile,
			deletedAt,
		)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("returns not found for unknown user", func(t *testing.T) {
		_, repository := newGetUserTestRepository(t, pool, config.Timeout)
		user := newGetUserRepositoryTestUser(t, nil, nil, nil)
		deletedAt := user.CreatedAt.Add(time.Second)
		deletedState, err := user.Delete(deletedAt)
		require.NoError(t, err)

		err = repository.DeleteUser(
			t.Context(),
			uuid.New(),
			deletedState.Profile,
			deletedAt,
		)

		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func newRepositoryTestUserWithUsername(t *testing.T, username string) domain.User {
	t.Helper()

	lastName := "Anderson"
	bio := "Repository test user"
	profile, err := domain.NewUserProfile(username, "Elliot", &lastName, &bio)
	require.NoError(t, err)
	user, err := domain.NewUser(
		uuid.New(),
		profile,
		getUserTestTime(),
		nil,
		"password-hash",
	)
	require.NoError(t, err)
	return user
}
