//go:build integration

package chats_postgres_repository

import (
	"testing"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetAndUpdateGroup(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("restores complete group", func(t *testing.T) {
		repository, expected, participants := newCreateGroupTestData(t, pool, config.Timeout)
		require.NoError(t, repository.CreateGroup(t.Context(), expected, participants))

		actual, err := repository.GetGroup(t.Context(), expected.Chat.ID)

		require.NoError(t, err)
		requireGroupEqual(t, expected, actual)
	})

	t.Run("updates title and preserves common chat", func(t *testing.T) {
		repository, original, participants := newCreateGroupTestData(t, pool, config.Timeout)
		require.NoError(t, repository.CreateGroup(t.Context(), original, participants))
		updated, err := original.Update("Updated title")
		require.NoError(t, err)

		err = repository.UpdateGroup(t.Context(), updated)
		require.NoError(t, err)
		actual, err := repository.GetGroup(t.Context(), original.Chat.ID)

		require.NoError(t, err)
		requireGroupEqual(t, updated, actual)
	})

	t.Run("returns not found for missing group", func(t *testing.T) {
		repository := NewChatsRepository(pool, config.Timeout)

		group, err := repository.GetGroup(t.Context(), uuid.New())

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Zero(t, group)
	})

	t.Run("returns not found when updating missing group", func(t *testing.T) {
		repository := NewChatsRepository(pool, config.Timeout)
		group, err := domain.NewGroupChat(uuid.New(), "Missing group", createDirectTestTime())
		require.NoError(t, err)

		err = repository.UpdateGroup(t.Context(), group)

		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func requireGroupEqual(t *testing.T, expected, actual domain.GroupChat) {
	t.Helper()
	require.Equal(t, expected.Chat.ID, actual.Chat.ID)
	require.Equal(t, expected.Chat.Type, actual.Chat.Type)
	require.Equal(t, expected.Chat.LastMessageID, actual.Chat.LastMessageID)
	require.True(t, expected.Chat.LastActivityAt.Equal(actual.Chat.LastActivityAt))
	require.True(t, expected.Chat.CreatedAt.Equal(actual.Chat.CreatedAt))
	require.Equal(t, expected.Title, actual.Title)
}
