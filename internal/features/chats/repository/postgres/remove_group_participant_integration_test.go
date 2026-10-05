//go:build integration

package chats_postgres_repository

import (
	"context"
	"errors"
	"testing"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestRemoveGroupParticipant(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("removes deleted group participant from both tables", func(t *testing.T) {
		fixture := newGroupParticipantsRepositoryFixture(t, pool, config.Timeout)
		target := fixture.participants[2]
		manager := postgres.NewTransactionManager(pool)

		err := manager.WithinTransaction(t.Context(), func(ctx context.Context) error {
			return fixture.repository.RemoveGroupParticipant(ctx, target)
		})

		require.NoError(t, err)
		require.Equal(t, 0, removeGroupParticipantCount(t, pool, "group_participants", target))
		require.Equal(t, 0, removeGroupParticipantCount(t, pool, "chat_participants", target))
		require.Equal(t, len(fixture.participants)-1, createGroupParticipantCount(
			t,
			pool,
			fixture.group.Chat.ID,
		))
	})

	t.Run("returns not found when participant is already absent", func(t *testing.T) {
		fixture := newGroupParticipantsRepositoryFixture(t, pool, config.Timeout)
		target := fixture.participants[1]
		manager := postgres.NewTransactionManager(pool)
		require.NoError(t, manager.WithinTransaction(t.Context(), func(ctx context.Context) error {
			return fixture.repository.RemoveGroupParticipant(ctx, target)
		}))

		err := manager.WithinTransaction(t.Context(), func(ctx context.Context) error {
			return fixture.repository.RemoveGroupParticipant(ctx, target)
		})

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("uses transaction executor and rolls back both deletions", func(t *testing.T) {
		fixture := newGroupParticipantsRepositoryFixture(t, pool, config.Timeout)
		target := fixture.participants[1]
		manager := postgres.NewTransactionManager(pool)
		rollbackErr := errors.New("rollback participant removal")

		err := manager.WithinTransaction(t.Context(), func(ctx context.Context) error {
			if err := fixture.repository.RemoveGroupParticipant(ctx, target); err != nil {
				return err
			}
			executor := postgres.GetExecutor(ctx, pool)
			require.Equal(t, 0, removeGroupParticipantCountWithExecutor(
				t,
				executor,
				"group_participants",
				target,
			))
			require.Equal(t, 0, removeGroupParticipantCountWithExecutor(
				t,
				executor,
				"chat_participants",
				target,
			))
			return rollbackErr
		})

		require.ErrorIs(t, err, rollbackErr)
		require.Equal(t, 1, removeGroupParticipantCount(t, pool, "group_participants", target))
		require.Equal(t, 1, removeGroupParticipantCount(t, pool, "chat_participants", target))
	})
}

func removeGroupParticipantCount(
	t *testing.T,
	pool *pgxpool.Pool,
	table string,
	participant domain.GroupParticipant,
) int {
	t.Helper()
	return removeGroupParticipantCountWithExecutor(t, pool, table, participant)
}

func removeGroupParticipantCountWithExecutor(
	t *testing.T,
	executor postgres.DBTX,
	table string,
	participant domain.GroupParticipant,
) int {
	t.Helper()
	require.Contains(t, []string{"group_participants", "chat_participants"}, table)
	var count int
	err := executor.QueryRow(
		t.Context(),
		"SELECT count(*) FROM "+table+" WHERE chat_id = $1 AND user_id = $2",
		participant.ChatID,
		participant.UserID,
	).Scan(&count)
	require.NoError(t, err)
	return count
}
