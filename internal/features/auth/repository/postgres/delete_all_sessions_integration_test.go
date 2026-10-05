//go:build integration

package auth_postgres_repository

import (
	"testing"

	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDeleteAllSessions(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("deletes every session for user and preserves other users sessions", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		now := sessionTestTime()
		userID := uuid.New()
		otherUserID := uuid.New()
		insertSessionTestUser(t, tx, userID, now)
		insertSessionTestUser(t, tx, otherUserID, now)
		require.NoError(t, repository.CreateSession(t.Context(), newSessionForTest(t, userID, uuid.New(), now)))
		require.NoError(t, repository.CreateSession(t.Context(), newSessionForTest(t, userID, uuid.New(), now)))
		require.NoError(t, repository.CreateSession(t.Context(), newSessionForTest(t, otherUserID, uuid.New(), now)))

		err := repository.DeleteAllSessions(t.Context(), userID)

		require.NoError(t, err)
		require.Equal(t, 0, countSessionsForUser(t, tx, userID))
		require.Equal(t, 1, countSessionsForUser(t, tx, otherUserID))
	})

	t.Run("succeeds when user has no sessions", func(t *testing.T) {
		_, repository := newSessionTestRepository(t, pool, config.Timeout)

		err := repository.DeleteAllSessions(t.Context(), uuid.New())

		require.NoError(t, err)
	})
}

func countSessionsForUser(t *testing.T, db postgres.DBTX, userID uuid.UUID) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(t.Context(), `
		SELECT COUNT(*)
		FROM sessions
		WHERE user_id = $1
	`, userID).Scan(&count))
	return count
}
