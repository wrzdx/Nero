//go:build integration

package users_postgres_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserProfile(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("updates all profile fields", func(t *testing.T) {
		tx, repository := newProfileUpdateTestRepository(t, pool, config.Timeout)
		user := newGetUserRepositoryTestUser(
			t,
			ptrForProfileUpdateTest("Old surname"),
			ptrForProfileUpdateTest("Old bio"),
			nil,
		)
		insertGetUserTestUser(t, tx, user)
		updatedProfile := newProfileUpdateTestProfile(
			t,
			"Updated_user",
			"Updated name",
			nil,
			ptrForProfileUpdateTest("Updated bio"),
		)

		err := repository.UpdateUserProfile(t.Context(), user.ID, updatedProfile)

		require.NoError(t, err)
		actual, err := repository.GetUser(t.Context(), user.ID)
		require.NoError(t, err)
		user.Profile = updatedProfile
		requireGetUserTestUserEqual(t, user, actual)
	})

	t.Run("does not update deleted user", func(t *testing.T) {
		tx, repository := newProfileUpdateTestRepository(t, pool, config.Timeout)
		deletedAt := getUserTestTime().Add(time.Hour)
		user := newGetUserRepositoryTestUser(t, nil, nil, &deletedAt)
		insertGetUserTestUser(t, tx, user)
		updatedProfile := newProfileUpdateTestProfile(
			t,
			"Updated_user",
			"Updated name",
			nil,
			nil,
		)

		err := repository.UpdateUserProfile(t.Context(), user.ID, updatedProfile)

		require.ErrorIs(t, err, domain.ErrNotFound)
		actual, err := repository.GetUser(t.Context(), user.ID)
		require.NoError(t, err)
		requireGetUserTestUserEqual(t, user, actual)
	})

	t.Run("returns not found for unknown user", func(t *testing.T) {
		_, repository := newProfileUpdateTestRepository(t, pool, config.Timeout)
		updatedProfile := newProfileUpdateTestProfile(
			t,
			"Updated_user",
			"Updated name",
			nil,
			nil,
		)

		err := repository.UpdateUserProfile(t.Context(), uuid.New(), updatedProfile)

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("maps duplicate username to already exists", func(t *testing.T) {
		tx, repository := newProfileUpdateTestRepository(t, pool, config.Timeout)
		first := newGetUserRepositoryTestUser(t, nil, nil, nil)
		second := newGetUserRepositoryTestUser(t, nil, nil, nil)
		insertGetUserTestUser(t, tx, first)
		insertGetUserTestUser(t, tx, second)
		updatedProfile := newProfileUpdateTestProfile(
			t,
			second.Profile.Username,
			first.Profile.FirstName,
			first.Profile.LastName,
			first.Profile.Bio,
		)

		err := repository.UpdateUserProfile(t.Context(), first.ID, updatedProfile)

		require.ErrorIs(t, err, domain.ErrAlreadyExists)
		var detailed domain.DetailedError
		require.ErrorAs(t, err, &detailed)
		require.Equal(t, map[string]string{
			"username": "username already taken",
		}, detailed.Fields())
	})
}

func TestGetUserForUpdateHoldsLockUntilTransactionEnds(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	user := newGetUserRepositoryTestUser(t, nil, nil, nil)
	insertGetUserTestUser(t, pool, user)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	lockingTx, err := pool.Begin(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = lockingTx.Rollback(context.Background())
	})
	lockingRepository := NewUsersRepository(lockingTx, config.Timeout)
	lockedUser, err := lockingRepository.GetUserForUpdate(t.Context(), user.ID)
	require.NoError(t, err)
	requireGetUserTestUserEqual(t, user, lockedUser)

	waitingTx, err := pool.Begin(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = waitingTx.Rollback(context.Background())
	})
	_, err = waitingTx.Exec(t.Context(), "SET LOCAL lock_timeout = '100ms'")
	require.NoError(t, err)
	waitingRepository := NewUsersRepository(waitingTx, config.Timeout)
	updatedProfile := newProfileUpdateTestProfile(
		t,
		"Updated_user",
		"Updated name",
		nil,
		nil,
	)

	err = waitingRepository.UpdateUserProfile(t.Context(), user.ID, updatedProfile)

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr))
	require.Equal(t, "55P03", pgErr.Code)
	require.NoError(t, lockingTx.Rollback(t.Context()))

	repository := NewUsersRepository(pool, config.Timeout)
	require.NoError(t, repository.UpdateUserProfile(t.Context(), user.ID, updatedProfile))
	actual, err := repository.GetUser(t.Context(), user.ID)
	require.NoError(t, err)
	user.Profile = updatedProfile
	requireGetUserTestUserEqual(t, user, actual)
}

func newProfileUpdateTestRepository(
	t *testing.T,
	pool *pgxpool.Pool,
	timeout time.Duration,
) (pgx.Tx, *UsersRepository) {
	t.Helper()

	tx, err := pool.Begin(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tx.Rollback(context.Background())
	})
	return tx, NewUsersRepository(tx, timeout)
}

func newProfileUpdateTestProfile(
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

func ptrForProfileUpdateTest(value string) *string {
	return &value
}
