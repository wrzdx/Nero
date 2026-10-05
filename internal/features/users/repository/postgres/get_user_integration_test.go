//go:build integration

package users_postgres_repository

import (
	"context"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestGetUser(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("returns active user with profile", func(t *testing.T) {
		tx, repository := newGetUserTestRepository(t, pool, config.Timeout)
		lastName := "Anderson"
		bio := "Integration test user"
		expected := newGetUserRepositoryTestUser(t, &lastName, &bio, nil)
		insertGetUserTestUser(t, tx, expected)

		actual, err := repository.GetUser(t.Context(), expected.ID)

		require.NoError(t, err)
		requireGetUserTestUserEqual(t, expected, actual)
	})

	t.Run("returns deleted user", func(t *testing.T) {
		tx, repository := newGetUserTestRepository(t, pool, config.Timeout)
		deletedAt := getUserTestTime().Add(time.Hour)
		expected := newGetUserRepositoryTestUser(t, nil, nil, &deletedAt)
		insertGetUserTestUser(t, tx, expected)

		actual, err := repository.GetUser(t.Context(), expected.ID)

		require.NoError(t, err)
		requireGetUserTestUserEqual(t, expected, actual)
	})

	t.Run("returns not found for unknown user", func(t *testing.T) {
		_, repository := newGetUserTestRepository(t, pool, config.Timeout)

		user, err := repository.GetUser(t.Context(), uuid.New())

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, user)
	})
}

func newGetUserTestRepository(
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

func newGetUserRepositoryTestUser(
	t *testing.T,
	lastName *string,
	bio *string,
	deletedAt *time.Time,
) domain.User {
	t.Helper()

	profile, err := domain.NewUserProfile(
		"User_"+uuid.NewString()[:8],
		"Elliot",
		lastName,
		bio,
	)
	require.NoError(t, err)

	user, err := domain.NewUser(
		uuid.New(),
		profile,
		getUserTestTime(),
		deletedAt,
		"password-hash",
	)
	require.NoError(t, err)

	return user
}

func insertGetUserTestUser(t *testing.T, db postgres.DBTX, user domain.User) {
	t.Helper()

	_, err := db.Exec(t.Context(), `
		INSERT INTO users (
			id,
			username,
			first_name,
			last_name,
			created_at,
			deleted_at,
			bio,
			password_hash
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		user.ID,
		user.Profile.Username,
		user.Profile.FirstName,
		user.Profile.LastName,
		user.CreatedAt,
		user.DeletedAt,
		user.Profile.Bio,
		user.PasswordHash,
	)
	require.NoError(t, err)
}

func requireGetUserTestUserEqual(t *testing.T, expected, actual domain.User) {
	t.Helper()

	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.Profile.Username, actual.Profile.Username)
	require.Equal(t, expected.Profile.FirstName, actual.Profile.FirstName)
	require.Equal(t, expected.Profile.LastName, actual.Profile.LastName)
	require.Equal(t, expected.Profile.Bio, actual.Profile.Bio)
	require.True(t, expected.CreatedAt.Equal(actual.CreatedAt))
	require.Equal(t, expected.PasswordHash, actual.PasswordHash)

	if expected.DeletedAt == nil {
		require.Nil(t, actual.DeletedAt)
		return
	}

	require.NotNil(t, actual.DeletedAt)
	require.True(t, expected.DeletedAt.Equal(*actual.DeletedAt))
}

func getUserTestTime() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}
