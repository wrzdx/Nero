//go:build integration

package auth_postgres_repository

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

func TestCreateSession(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("persists all session fields", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		now := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, now)
		session := newSessionForTest(t, userID, uuid.New(), now)

		err := repository.CreateSession(t.Context(), session)

		require.NoError(t, err)

		var saved domain.Session
		err = tx.QueryRow(t.Context(), `
			SELECT id, user_id, current_token_id, last_used_at, created_at, expires_at
			FROM sessions
			WHERE id = $1
		`, session.ID).Scan(
			&saved.ID,
			&saved.UserID,
			&saved.CurrentTokenID,
			&saved.LastUsedAt,
			&saved.CreatedAt,
			&saved.ExpiresAt,
		)
		require.NoError(t, err)
		require.Equal(t, session.ID, saved.ID)
		require.Equal(t, session.UserID, saved.UserID)
		require.Equal(t, session.CurrentTokenID, saved.CurrentTokenID)
		require.True(t, session.LastUsedAt.Equal(saved.LastUsedAt))
		require.True(t, session.CreatedAt.Equal(saved.CreatedAt))
		require.True(t, session.ExpiresAt.Equal(saved.ExpiresAt))
	})

	t.Run("rejects unknown user", func(t *testing.T) {
		_, repository := newSessionTestRepository(t, pool, config.Timeout)
		session := newSessionForTest(t, uuid.New(), uuid.New(), sessionTestTime())

		err := repository.CreateSession(t.Context(), session)
		require.True(t, postgres.IsConstraintViolation(
			err,
			postgres.ForeignKeyViolation,
			sessionsUsersFK,
		))
	})

	t.Run("rejects duplicate session id", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		now := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, now)
		session := newSessionForTest(t, userID, uuid.New(), now)
		require.NoError(t, repository.CreateSession(t.Context(), session))

		duplicate := newSessionForTest(t, userID, uuid.New(), now)
		duplicate.ID = session.ID
		err := repository.CreateSession(t.Context(), duplicate)

		require.True(t, postgres.IsConstraintViolation(
			err,
			postgres.UniqueViolation,
			sessionsPK,
		))
	})

	t.Run("rejects duplicate current token id", func(t *testing.T) {
		tx, repository := newSessionTestRepository(t, pool, config.Timeout)
		now := sessionTestTime()
		userID := uuid.New()
		insertSessionTestUser(t, tx, userID, now)
		currentTokenID := uuid.New()
		session := newSessionForTest(t, userID, currentTokenID, now)
		require.NoError(t, repository.CreateSession(t.Context(), session))

		duplicate := newSessionForTest(t, userID, currentTokenID, now)
		err := repository.CreateSession(t.Context(), duplicate)

		require.True(t, postgres.IsConstraintViolation(
			err,
			postgres.UniqueViolation,
			sessionsCurrentTokenIDUK,
		))
	})
}

func newSessionTestRepository(
	t *testing.T,
	pool *pgxpool.Pool,
	timeout time.Duration,
) (pgx.Tx, *SessionsRepository) {
	t.Helper()

	tx, err := pool.Begin(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tx.Rollback(context.Background())
	})

	repository := NewSessionsRepository(tx, timeout)
	return tx, repository
}

func insertSessionTestUser(
	t *testing.T,
	db postgres.DBTX,
	userID uuid.UUID,
	createdAt time.Time,
) {
	t.Helper()

	usernameSuffix := strings.ReplaceAll(userID.String(), "-", "")[:16]
	_, err := db.Exec(t.Context(), `
		INSERT INTO users (id, username, first_name, created_at, password_hash)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, "user_"+usernameSuffix, "Session Test", createdAt, "password_hash")
	require.NoError(t, err)
}

func newSessionForTest(
	t *testing.T,
	userID uuid.UUID,
	currentTokenID uuid.UUID,
	createdAt time.Time,
) domain.Session {
	t.Helper()

	session, err := domain.NewSession(
		uuid.New(),
		userID,
		currentTokenID,
		createdAt,
		createdAt.Add(24*time.Hour),
	)
	require.NoError(t, err)
	return session
}

func sessionTestTime() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}
