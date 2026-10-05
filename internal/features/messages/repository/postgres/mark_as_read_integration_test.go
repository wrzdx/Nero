//go:build integration

package messages_postgres_repository

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

func TestMarkAsRead(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("moves cursor forward and never backward", func(t *testing.T) {
		repository, chat, participantID := newMessageHistoryFixture(t, pool, config.Timeout)
		base := repositoryTestTime().Add(time.Hour)
		older := newMessageHistoryMessage(t, uuid.New(), chat.ID, participantID, base)
		newer := newMessageHistoryMessage(
			t,
			uuid.New(),
			chat.ID,
			participantID,
			base.Add(time.Minute),
		)
		insertMessageRepositoryMessage(t, pool, older)
		insertMessageRepositoryMessage(t, pool, newer)

		err := repository.MarkAsRead(t.Context(), chat.ID, participantID, older.ID)
		require.NoError(t, err)
		requireMessageRepositoryLastReadMessageID(
			t,
			pool,
			chat.ID,
			participantID,
			&older.ID,
		)

		err = repository.MarkAsRead(t.Context(), chat.ID, participantID, newer.ID)
		require.NoError(t, err)
		requireMessageRepositoryLastReadMessageID(
			t,
			pool,
			chat.ID,
			participantID,
			&newer.ID,
		)

		err = repository.MarkAsRead(t.Context(), chat.ID, participantID, older.ID)
		require.NoError(t, err)
		requireMessageRepositoryLastReadMessageID(
			t,
			pool,
			chat.ID,
			participantID,
			&newer.ID,
		)

		err = repository.MarkAsRead(t.Context(), chat.ID, participantID, newer.ID)
		require.NoError(t, err)
		requireMessageRepositoryLastReadMessageID(
			t,
			pool,
			chat.ID,
			participantID,
			&newer.ID,
		)
	})

	t.Run("uses id as tie breaker", func(t *testing.T) {
		repository, chat, participantID := newMessageHistoryFixture(t, pool, config.Timeout)
		createdAt := repositoryTestTime().Add(time.Hour)
		lower := newMessageHistoryMessage(
			t,
			uuid.MustParse("00000000-0000-0000-0000-000000000010"),
			chat.ID,
			participantID,
			createdAt,
		)
		higher := newMessageHistoryMessage(
			t,
			uuid.MustParse("00000000-0000-0000-0000-000000000020"),
			chat.ID,
			participantID,
			createdAt,
		)
		insertMessageRepositoryMessage(t, pool, lower)
		insertMessageRepositoryMessage(t, pool, higher)

		require.NoError(t, repository.MarkAsRead(
			t.Context(),
			chat.ID,
			participantID,
			higher.ID,
		))
		require.NoError(t, repository.MarkAsRead(
			t.Context(),
			chat.ID,
			participantID,
			lower.ID,
		))

		requireMessageRepositoryLastReadMessageID(
			t,
			pool,
			chat.ID,
			participantID,
			&higher.ID,
		)
	})

	t.Run("hides message from another chat", func(t *testing.T) {
		repository, chat, participantID := newMessageHistoryFixture(t, pool, config.Timeout)
		_, otherChat, otherParticipantID := newMessageHistoryFixture(t, pool, config.Timeout)
		otherMessage := newMessageHistoryMessage(
			t,
			uuid.New(),
			otherChat.ID,
			otherParticipantID,
			repositoryTestTime().Add(time.Hour),
		)
		insertMessageRepositoryMessage(t, pool, otherMessage)

		err := repository.MarkAsRead(
			t.Context(),
			chat.ID,
			participantID,
			otherMessage.ID,
		)

		require.ErrorIs(t, err, domain.ErrNotFound)
		requireMessageRepositoryLastReadMessageID(
			t,
			pool,
			chat.ID,
			participantID,
			nil,
		)
	})

	t.Run("returns not found for missing participant", func(t *testing.T) {
		repository, chat, senderID := newMessageRepositoryFixture(t, pool, config.Timeout)
		message := newRepositoryTestMessage(t, chat.ID, senderID, uuid.New(), "message")
		insertMessageRepositoryMessage(t, pool, message)

		err := repository.MarkAsRead(t.Context(), chat.ID, senderID, message.ID)

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("returns not found for missing message", func(t *testing.T) {
		repository, chat, participantID := newMessageHistoryFixture(t, pool, config.Timeout)

		err := repository.MarkAsRead(t.Context(), chat.ID, participantID, uuid.New())

		require.ErrorIs(t, err, domain.ErrNotFound)
		requireMessageRepositoryLastReadMessageID(
			t,
			pool,
			chat.ID,
			participantID,
			nil,
		)
	})

	t.Run("participates in transaction rollback", func(t *testing.T) {
		repository, chat, participantID := newMessageHistoryFixture(t, pool, config.Timeout)
		message := newRepositoryTestMessage(t, chat.ID, participantID, uuid.New(), "message")
		insertMessageRepositoryMessage(t, pool, message)
		manager := postgres.NewTransactionManager(pool)
		rollbackErr := errors.New("rollback mark as read")

		err := manager.WithinTransaction(t.Context(), func(ctx context.Context) error {
			if err := repository.MarkAsRead(
				ctx,
				chat.ID,
				participantID,
				message.ID,
			); err != nil {
				return err
			}
			return rollbackErr
		})

		require.ErrorIs(t, err, rollbackErr)
		requireMessageRepositoryLastReadMessageID(
			t,
			pool,
			chat.ID,
			participantID,
			nil,
		)
	})
}
