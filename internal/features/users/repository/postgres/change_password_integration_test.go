//go:build integration

package users_postgres_repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestChangePassword(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("updates active user when expected hash matches", func(t *testing.T) {
		tx, repository := newPasswordTestRepository(t, pool, config.Timeout)
		userID := insertPasswordTestUser(t, tx, false, "current-hash")

		err := repository.ChangePassword(t.Context(), userID, "new-hash", "current-hash")

		require.NoError(t, err)
		require.Equal(t, "new-hash", passwordHashForUser(t, tx, userID))
	})

	t.Run("rejects stale expected hash without changing password", func(t *testing.T) {
		tx, repository := newPasswordTestRepository(t, pool, config.Timeout)
		userID := insertPasswordTestUser(t, tx, false, "current-hash")

		err := repository.ChangePassword(t.Context(), userID, "new-hash", "stale-hash")

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Equal(t, "current-hash", passwordHashForUser(t, tx, userID))
	})

	t.Run("only first update succeeds with the same expected hash", func(t *testing.T) {
		tx, repository := newPasswordTestRepository(t, pool, config.Timeout)
		userID := insertPasswordTestUser(t, tx, false, "current-hash")

		firstErr := repository.ChangePassword(t.Context(), userID, "first-new-hash", "current-hash")
		secondErr := repository.ChangePassword(t.Context(), userID, "second-new-hash", "current-hash")

		require.NoError(t, firstErr)
		require.ErrorIs(t, secondErr, domain.ErrNotFound)
		require.Equal(t, "first-new-hash", passwordHashForUser(t, tx, userID))
	})

	t.Run("does not update deleted user", func(t *testing.T) {
		tx, repository := newPasswordTestRepository(t, pool, config.Timeout)
		userID := insertPasswordTestUser(t, tx, true, "current-hash")

		err := repository.ChangePassword(t.Context(), userID, "new-hash", "current-hash")

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Equal(t, "current-hash", passwordHashForUser(t, tx, userID))
	})

	t.Run("returns not found for unknown user", func(t *testing.T) {
		_, repository := newPasswordTestRepository(t, pool, config.Timeout)

		err := repository.ChangePassword(t.Context(), uuid.New(), "new-hash", "current-hash")

		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func newPasswordTestRepository(
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

func insertPasswordTestUser(
	t *testing.T,
	db postgres.DBTX,
	deleted bool,
	passwordHash string,
) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	usernameSuffix := strings.ReplaceAll(userID.String(), "-", "")[:16]
	var deletedAt *time.Time
	if deleted {
		value := time.Now().UTC()
		deletedAt = &value
	}
	_, err := db.Exec(t.Context(), `
		INSERT INTO users (id, username, first_name, created_at, deleted_at, password_hash)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, "password_"+usernameSuffix, "Password Test", time.Now().UTC(), deletedAt, passwordHash)
	require.NoError(t, err)
	return userID
}

func passwordHashForUser(t *testing.T, db postgres.DBTX, userID uuid.UUID) string {
	t.Helper()
	var passwordHash string
	require.NoError(t, db.QueryRow(t.Context(), `
		SELECT password_hash
		FROM users
		WHERE id = $1
	`, userID).Scan(&passwordHash))
	return passwordHash
}
