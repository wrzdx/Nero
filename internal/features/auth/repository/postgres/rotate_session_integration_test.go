//go:build integration

package auth_postgres_repository

import (
	"context"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRotateSession(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("rotates current token and returns updated session", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		createdAt := sessionTestTime()
		usedAt := createdAt.Add(time.Hour)
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		session := newSessionForTest(t, userID, uuid.New(), createdAt)
		require.NoError(t, repository.CreateSession(t.Context(), session))
		newTokenID := uuid.New()

		rotated, err := repository.RotateSession(
			t.Context(),
			session.ID,
			session.CurrentTokenID,
			newTokenID,
			usedAt,
		)

		require.NoError(t, err)
		require.Equal(t, session.ID, rotated.ID)
		require.Equal(t, session.UserID, rotated.UserID)
		require.Equal(t, newTokenID, rotated.CurrentTokenID)
		require.True(t, usedAt.Equal(rotated.LastUsedAt))
		require.True(t, session.CreatedAt.Equal(rotated.CreatedAt))
		require.True(t, session.ExpiresAt.Equal(rotated.ExpiresAt))

		saved := loadSessionForTest(t, tx, session.ID)
		require.Equal(t, newTokenID, saved.CurrentTokenID)
		require.True(t, usedAt.Equal(saved.LastUsedAt))
	})

	t.Run("rejects a wrong current token without changing session", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		createdAt := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		session := newSessionForTest(t, userID, uuid.New(), createdAt)
		require.NoError(t, repository.CreateSession(t.Context(), session))

		_, err := repository.RotateSession(
			t.Context(),
			session.ID,
			uuid.New(),
			uuid.New(),
			createdAt.Add(time.Hour),
		)

		require.ErrorIs(t, err, domain.ErrNotFound)
		requireSessionEqual(t, session, loadSessionForTest(t, tx, session.ID))
	})

	t.Run("rejects reuse of the previous token", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		createdAt := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		session := newSessionForTest(t, userID, uuid.New(), createdAt)
		require.NoError(t, repository.CreateSession(t.Context(), session))
		firstNewTokenID := uuid.New()
		firstUsedAt := createdAt.Add(time.Hour)
		_, err := repository.RotateSession(
			t.Context(),
			session.ID,
			session.CurrentTokenID,
			firstNewTokenID,
			firstUsedAt,
		)
		require.NoError(t, err)

		_, err = repository.RotateSession(
			t.Context(),
			session.ID,
			session.CurrentTokenID,
			uuid.New(),
			createdAt.Add(2*time.Hour),
		)

		require.ErrorIs(t, err, domain.ErrNotFound)
		saved := loadSessionForTest(t, tx, session.ID)
		require.Equal(t, firstNewTokenID, saved.CurrentTokenID)
		require.True(t, firstUsedAt.Equal(saved.LastUsedAt))
	})

	t.Run("rejects rotation at expiration time", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		createdAt := sessionTestTime()
		expiresAt := createdAt.Add(time.Hour)
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		session := newSessionWithExpiryForTest(t, userID, uuid.New(), createdAt, expiresAt)
		require.NoError(t, repository.CreateSession(t.Context(), session))

		_, err := repository.RotateSession(
			t.Context(),
			session.ID,
			session.CurrentTokenID,
			uuid.New(),
			expiresAt,
		)

		require.ErrorIs(t, err, domain.ErrNotFound)
		requireSessionEqual(t, session, loadSessionForTest(t, tx, session.ID))
	})

	t.Run("does not move last used time backwards", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		createdAt := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		session := newSessionForTest(t, userID, uuid.New(), createdAt)
		require.NoError(t, repository.CreateSession(t.Context(), session))
		secondTokenID := uuid.New()
		latestUsedAt := createdAt.Add(2 * time.Hour)
		_, err := repository.RotateSession(
			t.Context(),
			session.ID,
			session.CurrentTokenID,
			secondTokenID,
			latestUsedAt,
		)
		require.NoError(t, err)

		rotated, err := repository.RotateSession(
			t.Context(),
			session.ID,
			secondTokenID,
			uuid.New(),
			createdAt.Add(time.Hour),
		)

		require.NoError(t, err)
		require.True(t, latestUsedAt.Equal(rotated.LastUsedAt))
	})

	t.Run("rejects a duplicate new token id", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		createdAt := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, createdAt)
		first := newSessionForTest(t, userID, uuid.New(), createdAt)
		second := newSessionForTest(t, userID, uuid.New(), createdAt)
		require.NoError(t, repository.CreateSession(t.Context(), first))
		require.NoError(t, repository.CreateSession(t.Context(), second))

		_, err := repository.RotateSession(
			t.Context(),
			first.ID,
			first.CurrentTokenID,
			second.CurrentTokenID,
			createdAt.Add(time.Hour),
		)

		require.True(t, postgres.IsConstraintViolation(
			err,
			postgres.UniqueViolation,
			sessionsCurrentTokenIDUK,
		))
	})

	t.Run("allows only one concurrent rotation of the same token", func(t *testing.T) {
		repository := NewSessionsRepository(pool, config.Timeout)
		createdAt := sessionTestTime()
		usedAt := createdAt.Add(time.Hour)
		userID := uuid.New()
		insertSessionTestUser(t, pool, userID, createdAt)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
			defer cancel()
			_, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
			require.NoError(t, err)
		})
		session := newSessionForTest(t, userID, uuid.New(), createdAt)
		require.NoError(t, repository.CreateSession(t.Context(), session))

		type rotationResult struct {
			session domain.Session
			err     error
		}

		start := make(chan struct{})
		results := make(chan rotationResult, 2)
		newTokenIDs := [2]uuid.UUID{uuid.New(), uuid.New()}
		for _, newTokenID := range newTokenIDs {
			go func() {
				<-start
				rotated, err := repository.RotateSession(
					t.Context(),
					session.ID,
					session.CurrentTokenID,
					newTokenID,
					usedAt,
				)
				results <- rotationResult{session: rotated, err: err}
			}()
		}
		close(start)

		var successful domain.Session
		successCount := 0
		notFoundCount := 0
		for range newTokenIDs {
			result := <-results
			if result.err == nil {
				successCount++
				successful = result.session
				continue
			}
			require.ErrorIs(t, result.err, domain.ErrNotFound)
			notFoundCount++
		}

		require.Equal(t, 1, successCount)
		require.Equal(t, 1, notFoundCount)
		require.NotEqual(t, session.CurrentTokenID, successful.CurrentTokenID)
		saved := loadSessionForTest(t, pool, session.ID)
		require.Equal(t, successful.CurrentTokenID, saved.CurrentTokenID)
		require.True(t, usedAt.Equal(saved.LastUsedAt))
	})
}

func loadSessionForTest(
	t *testing.T,
	db postgres.DBTX,
	sessionID uuid.UUID,
) domain.Session {
	t.Helper()

	var session domain.Session
	err := db.QueryRow(t.Context(), `
		SELECT id, user_id, current_token_id, last_used_at, created_at, expires_at
		FROM sessions
		WHERE id = $1
	`, sessionID).Scan(
		&session.ID,
		&session.UserID,
		&session.CurrentTokenID,
		&session.LastUsedAt,
		&session.CreatedAt,
		&session.ExpiresAt,
	)
	require.NoError(t, err)
	return session
}

func newSessionWithExpiryForTest(
	t *testing.T,
	userID uuid.UUID,
	currentTokenID uuid.UUID,
	createdAt time.Time,
	expiresAt time.Time,
) domain.Session {
	t.Helper()

	session, err := domain.NewSession(
		uuid.New(),
		userID,
		currentTokenID,
		createdAt,
		expiresAt,
	)
	require.NoError(t, err)
	return session
}

func requireSessionEqual(t *testing.T, expected, actual domain.Session) {
	t.Helper()

	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.UserID, actual.UserID)
	require.Equal(t, expected.CurrentTokenID, actual.CurrentTokenID)
	require.True(t, expected.LastUsedAt.Equal(actual.LastUsedAt))
	require.True(t, expected.CreatedAt.Equal(actual.CreatedAt))
	require.True(t, expected.ExpiresAt.Equal(actual.ExpiresAt))
}
