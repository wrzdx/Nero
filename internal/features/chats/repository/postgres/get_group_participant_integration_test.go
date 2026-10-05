//go:build integration

package chats_postgres_repository

import (
	"testing"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetGroupParticipant(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("restores active group participant", func(t *testing.T) {
		fixture := newGroupParticipantsRepositoryFixture(t, pool, config.Timeout)
		expected := fixture.participants[0]

		actual, err := fixture.repository.GetGroupParticipant(
			t.Context(),
			fixture.group.Chat.ID,
			expected.UserID,
		)

		require.NoError(t, err)
		requireGroupParticipantEqual(t, expected, actual)
	})

	t.Run("returns not found for non participant", func(t *testing.T) {
		fixture := newGroupParticipantsRepositoryFixture(t, pool, config.Timeout)

		participant, err := fixture.repository.GetGroupParticipant(
			t.Context(),
			fixture.group.Chat.ID,
			uuid.New(),
		)

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, participant)
	})

	t.Run("restores deleted group participant", func(t *testing.T) {
		fixture := newGroupParticipantsRepositoryFixture(t, pool, config.Timeout)
		expected := fixture.participants[2]

		actual, err := fixture.repository.GetGroupParticipant(
			t.Context(),
			fixture.group.Chat.ID,
			fixture.deletedUserID,
		)

		require.NoError(t, err)
		requireGroupParticipantEqual(t, expected, actual)
	})

	t.Run("returns not found for direct participant", func(t *testing.T) {
		repository, direct, participant1, participant2 := newCreateDirectTestData(
			t,
			pool,
			config.Timeout,
		)
		require.NoError(t, repository.CreateDirect(
			t.Context(),
			direct,
			participant1,
			participant2,
		))

		participant, err := repository.GetGroupParticipant(
			t.Context(),
			direct.Chat.ID,
			participant1.UserID,
		)

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, participant)
	})
}

func requireGroupParticipantEqual(
	t *testing.T,
	expected domain.GroupParticipant,
	actual domain.GroupParticipant,
) {
	t.Helper()

	require.Equal(t, expected.ChatID, actual.ChatID)
	require.Equal(t, expected.UserID, actual.UserID)
	require.Equal(t, expected.LastReadMessageID, actual.LastReadMessageID)
	require.True(t, expected.JoinedAt.Equal(actual.JoinedAt))
	require.Equal(t, expected.Role(), actual.Role())
}
