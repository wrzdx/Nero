//go:build integration

package auth_postgres_repository

import (
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestDeleteSession(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("deletes session with matching current token", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		createdAt := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		session := newSessionForTest(t, userID, uuid.New(), createdAt)
		require.NoError(t, repository.CreateSession(t.Context(), session))

		err := repository.DeleteSession(
			t.Context(),
			session.ID,
			session.CurrentTokenID,
		)

		require.NoError(t, err)
		var id uuid.UUID
		err = tx.QueryRow(t.Context(), `
			SELECT id
			FROM sessions
			WHERE id = $1
		`, session.ID).Scan(&id)
		require.ErrorIs(t, err, pgx.ErrNoRows)
	})

	t.Run("rejects a wrong current token without deleting session", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		createdAt := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		session := newSessionForTest(t, userID, uuid.New(), createdAt)
		require.NoError(t, repository.CreateSession(t.Context(), session))

		err := repository.DeleteSession(t.Context(), session.ID, uuid.New())

		require.ErrorIs(t, err, domain.ErrNotFound)
		requireSessionEqual(t, session, loadSessionForTest(t, tx, session.ID))
	})

	t.Run("old token cannot delete a rotated session", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		createdAt := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		session := newSessionForTest(t, userID, uuid.New(), createdAt)
		require.NoError(t, repository.CreateSession(t.Context(), session))
		newTokenID := uuid.New()
		usedAt := createdAt.Add(time.Hour)
		rotated, err := repository.RotateSession(
			t.Context(),
			session.ID,
			session.CurrentTokenID,
			newTokenID,
			usedAt,
		)
		require.NoError(t, err)

		err = repository.DeleteSession(
			t.Context(),
			session.ID,
			session.CurrentTokenID,
		)

		require.ErrorIs(t, err, domain.ErrNotFound)
		requireSessionEqual(t, rotated, loadSessionForTest(t, tx, session.ID))
	})

	t.Run("returns not found for unknown session", func(t *testing.T) {
		_, repository := newSessionTestRepository(t, pool, config.Timeout)

		err := repository.DeleteSession(t.Context(), uuid.New(), uuid.New())

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("deletes an expired session", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		now := sessionTestTime()
		createdAt := now.Add(-2 * time.Hour)
		expiresAt := now.Add(-time.Hour)
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		session := newSessionWithExpiryForTest(
			t,
			userID,
			uuid.New(),
			createdAt,
			expiresAt,
		)
		require.NoError(t, repository.CreateSession(t.Context(), session))

		err := repository.DeleteSession(
			t.Context(),
			session.ID,
			session.CurrentTokenID,
		)

		require.NoError(t, err)
	})
}
