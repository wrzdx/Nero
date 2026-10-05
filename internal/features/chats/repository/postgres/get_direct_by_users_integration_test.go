//go:build integration

package chats_postgres_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetDirectByUsers(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("restores complete direct chat state for either user order", func(t *testing.T) {
		repository, expected, participant1, participant2 := newCreateDirectTestData(
			t,
			pool,
			config.Timeout,
		)
		require.NoError(t, repository.CreateDirect(
			t.Context(),
			expected,
			participant1,
			participant2,
		))

		messageID := uuid.New()
		clientMessageID := uuid.New()
		lastActivityAt := expected.Chat.CreatedAt.Add(time.Minute)
		_, err := pool.Exec(t.Context(), `
			INSERT INTO messages (
				id,
				client_message_id,
				chat_id,
				sender_id,
				content,
				created_at
			)
			VALUES ($1, $2, $3, $4, $5, $6)
		`,
			messageID,
			clientMessageID,
			expected.Chat.ID,
			expected.User1ID,
			"test message",
			lastActivityAt,
		)
		require.NoError(t, err)
		_, err = pool.Exec(t.Context(), `
			UPDATE chats
			SET last_message_id = $1, last_activity_at = $2
			WHERE id = $3
		`, messageID, lastActivityAt, expected.Chat.ID)
		require.NoError(t, err)
		expected.Chat.LastMessageID = &messageID
		expected.Chat.LastActivityAt = lastActivityAt

		testCases := []struct {
			name    string
			user1ID uuid.UUID
			user2ID uuid.UUID
		}{
			{
				name:    "normalized order",
				user1ID: expected.User1ID,
				user2ID: expected.User2ID,
			},
			{
				name:    "reverse order",
				user1ID: expected.User2ID,
				user2ID: expected.User1ID,
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				actual, err := repository.GetDirectByUsers(
					t.Context(),
					testCase.user1ID,
					testCase.user2ID,
				)

				require.NoError(t, err)
				requireCreateDirectEqual(t, expected, actual)
			})
		}
	})

	t.Run("returns not found for unknown pair", func(t *testing.T) {
		repository := NewChatsRepository(pool, config.Timeout)

		direct, err := repository.GetDirectByUsers(
			t.Context(),
			uuid.New(),
			uuid.New(),
		)

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, direct)
	})

	t.Run("reads uncommitted direct from transaction context", func(t *testing.T) {
		repository, expected, participant1, participant2 := newCreateDirectTestData(
			t,
			pool,
			config.Timeout,
		)
		manager := postgres.NewTransactionManager(pool)
		rollbackErr := errors.New("rollback test transaction")
		var actual domain.DirectChat

		err := manager.WithinTransaction(t.Context(), func(ctx context.Context) error {
			if err := repository.CreateDirect(
				ctx,
				expected,
				participant1,
				participant2,
			); err != nil {
				return err
			}

			var err error
			actual, err = repository.GetDirectByUsers(
				ctx,
				expected.User2ID,
				expected.User1ID,
			)
			if err != nil {
				return err
			}
			return rollbackErr
		})

		require.ErrorIs(t, err, rollbackErr)
		requireCreateDirectEqual(t, expected, actual)
		require.Equal(t, 0, createDirectRowCount(t, pool, "chats", expected.Chat.ID))
	})
}

func requireCreateDirectEqual(
	t *testing.T,
	expected domain.DirectChat,
	actual domain.DirectChat,
) {
	t.Helper()

	require.Equal(t, expected.Chat.ID, actual.Chat.ID)
	require.Equal(t, expected.Chat.LastMessageID, actual.Chat.LastMessageID)
	require.True(t, expected.Chat.LastActivityAt.Equal(actual.Chat.LastActivityAt))
	require.True(t, expected.Chat.CreatedAt.Equal(actual.Chat.CreatedAt))
	require.Equal(t, expected.User1ID, actual.User1ID)
	require.Equal(t, expected.User2ID, actual.User2ID)
}
