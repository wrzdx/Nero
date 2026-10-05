//go:build integration

package messages_postgres_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"
	messages_service "github.com/wrzdx/Nero/internal/features/messages/service"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestCheckParticipant(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("accepts active chat participant", func(t *testing.T) {
		repository, chat, participantID := newMessageHistoryFixture(t, pool, config.Timeout)

		err := repository.CheckParticipant(t.Context(), chat.ID, participantID)

		require.NoError(t, err)
	})

	t.Run("hides deleted participant", func(t *testing.T) {
		repository, chat, participantID := newMessageHistoryFixture(t, pool, config.Timeout)
		_, err := pool.Exec(t.Context(), `
			UPDATE users SET deleted_at = $1 WHERE id = $2
		`, repositoryTestTime().Add(time.Hour), participantID)
		require.NoError(t, err)

		err = repository.CheckParticipant(t.Context(), chat.ID, participantID)

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("hides existing non-participant", func(t *testing.T) {
		repository, chat, _ := newMessageHistoryFixture(t, pool, config.Timeout)
		outsiderID := uuid.New()
		insertMessageRepositoryUser(t, pool, outsiderID)
		t.Cleanup(func() {
			deleteMessageRepositoryUser(t, pool, config.Timeout, outsiderID)
		})

		err := repository.CheckParticipant(t.Context(), chat.ID, outsiderID)

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("returns not found for unknown chat", func(t *testing.T) {
		repository := NewRepository(pool, config.Timeout)

		err := repository.CheckParticipant(t.Context(), uuid.New(), uuid.New())

		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestGetMessagesPage(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("returns empty result for chat without messages", func(t *testing.T) {
		repository, chat, _ := newMessageHistoryFixture(t, pool, config.Timeout)

		messages, err := repository.GetMessages(t.Context(), chat.ID, nil, 10, false)

		require.NoError(t, err)
		require.Empty(t, messages)
	})

	t.Run("orders by creation time and id descending", func(t *testing.T) {
		repository, chat, senderID := newMessageHistoryFixture(t, pool, config.Timeout)
		base := repositoryTestTime().Add(time.Hour)
		newest := newMessageHistoryMessage(
			t,
			uuid.MustParse("00000000-0000-0000-0000-000000000010"),
			chat.ID,
			senderID,
			base.Add(2*time.Minute),
		)
		tieLow := newMessageHistoryMessage(
			t,
			uuid.MustParse("00000000-0000-0000-0000-000000000020"),
			chat.ID,
			senderID,
			base.Add(time.Minute),
		)
		tieHigh := newMessageHistoryMessage(
			t,
			uuid.MustParse("00000000-0000-0000-0000-000000000030"),
			chat.ID,
			senderID,
			base.Add(time.Minute),
		)
		oldest := newMessageHistoryMessage(
			t,
			uuid.MustParse("00000000-0000-0000-0000-000000000040"),
			chat.ID,
			senderID,
			base,
		)
		for _, message := range []domain.Message{oldest, tieLow, newest, tieHigh} {
			insertMessageRepositoryMessage(t, pool, message)
		}

		messages, err := repository.GetMessages(t.Context(), chat.ID, nil, 10, false)

		require.NoError(t, err)
		requireMessageHistoryIDs(t, messages, newest.ID, tieHigh.ID, tieLow.ID, oldest.ID)
	})

	t.Run("respects limit", func(t *testing.T) {
		repository, chat, senderID := newMessageHistoryFixture(t, pool, config.Timeout)
		base := repositoryTestTime().Add(time.Hour)
		messages := []domain.Message{
			newMessageHistoryMessage(t, uuid.New(), chat.ID, senderID, base.Add(2*time.Minute)),
			newMessageHistoryMessage(t, uuid.New(), chat.ID, senderID, base.Add(time.Minute)),
			newMessageHistoryMessage(t, uuid.New(), chat.ID, senderID, base),
		}
		for _, message := range messages {
			insertMessageRepositoryMessage(t, pool, message)
		}

		actual, err := repository.GetMessages(t.Context(), chat.ID, nil, 2, false)

		require.NoError(t, err)
		requireMessageHistoryIDs(t, actual, messages[0].ID, messages[1].ID)
	})

	t.Run("returns only messages strictly before cursor", func(t *testing.T) {
		repository, chat, senderID := newMessageHistoryFixture(t, pool, config.Timeout)
		base := repositoryTestTime().Add(time.Hour)
		tieLow := newMessageHistoryMessage(
			t,
			uuid.MustParse("00000000-0000-0000-0000-000000000020"),
			chat.ID,
			senderID,
			base,
		)
		cursorMessage := newMessageHistoryMessage(
			t,
			uuid.MustParse("00000000-0000-0000-0000-000000000030"),
			chat.ID,
			senderID,
			base,
		)
		older := newMessageHistoryMessage(t, uuid.New(), chat.ID, senderID, base.Add(-time.Minute))
		for _, message := range []domain.Message{tieLow, cursorMessage, older} {
			insertMessageRepositoryMessage(t, pool, message)
		}
		cursor := &messages_service.MessageCursor{
			MessageID: cursorMessage.ID,
			CreatedAt: cursorMessage.CreatedAt,
		}

		messages, err := repository.GetMessages(t.Context(), chat.ID, cursor, 10, false)

		require.NoError(t, err)
		requireMessageHistoryIDs(t, messages, tieLow.ID, older.ID)
	})

	t.Run("pages strictly after cursor in ascending order without skipping newer messages", func(t *testing.T) {
		repository, chat, senderID := newMessageHistoryFixture(t, pool, config.Timeout)
		_, otherChat, otherSenderID := newMessageHistoryFixture(t, pool, config.Timeout)
		base := repositoryTestTime().Add(time.Hour)
		older := newMessageHistoryMessage(t,
			uuid.MustParse("00000000-0000-0000-0000-000000000010"), chat.ID, senderID, base)
		boundary := newMessageHistoryMessage(t,
			uuid.MustParse("00000000-0000-0000-0000-000000000020"), chat.ID, senderID, base)
		tieHigher := newMessageHistoryMessage(t,
			uuid.MustParse("00000000-0000-0000-0000-000000000030"), chat.ID, senderID, base)
		later := newMessageHistoryMessage(t,
			uuid.MustParse("00000000-0000-0000-0000-000000000005"), chat.ID, senderID, base.Add(time.Minute))
		latest := newMessageHistoryMessage(t, uuid.New(), chat.ID, senderID, base.Add(2*time.Minute))
		other := newMessageHistoryMessage(t, uuid.New(), otherChat.ID, otherSenderID, base.Add(time.Second))
		for _, message := range []domain.Message{latest, boundary, older, later, tieHigher, other} {
			insertMessageRepositoryMessage(t, pool, message)
		}
		cursor := &messages_service.MessageCursor{MessageID: boundary.ID, CreatedAt: boundary.CreatedAt}

		first, err := repository.GetMessages(t.Context(), chat.ID, cursor, 2, true)

		require.NoError(t, err)
		requireMessageHistoryIDs(t, first, tieHigher.ID, later.ID)
		cursor = &messages_service.MessageCursor{MessageID: later.ID, CreatedAt: later.CreatedAt}
		second, err := repository.GetMessages(t.Context(), chat.ID, cursor, 2, true)
		require.NoError(t, err)
		requireMessageHistoryIDs(t, second, latest.ID)
		cursor = &messages_service.MessageCursor{MessageID: latest.ID, CreatedAt: latest.CreatedAt}
		last, err := repository.GetMessages(t.Context(), chat.ID, cursor, 2, true)
		require.NoError(t, err)
		require.Empty(t, last)
	})

	t.Run("after cursor does not require boundary message to still exist", func(t *testing.T) {
		repository, chat, senderID := newMessageHistoryFixture(t, pool, config.Timeout)
		base := repositoryTestTime().Add(time.Hour)
		boundary := newMessageHistoryMessage(t, uuid.New(), chat.ID, senderID, base)
		newer := newMessageHistoryMessage(t, uuid.New(), chat.ID, senderID, base.Add(time.Minute))
		insertMessageRepositoryMessage(t, pool, boundary)
		insertMessageRepositoryMessage(t, pool, newer)
		_, err := pool.Exec(t.Context(), `DELETE FROM messages WHERE id = $1`, boundary.ID)
		require.NoError(t, err)

		messages, err := repository.GetMessages(t.Context(), chat.ID,
			&messages_service.MessageCursor{MessageID: boundary.ID, CreatedAt: boundary.CreatedAt}, 10, true)

		require.NoError(t, err)
		requireMessageHistoryIDs(t, messages, newer.ID)
	})

	t.Run("does not return messages from another chat", func(t *testing.T) {
		repository, chat, senderID := newMessageHistoryFixture(t, pool, config.Timeout)
		_, otherChat, otherSenderID := newMessageHistoryFixture(t, pool, config.Timeout)
		expected := newMessageHistoryMessage(t, uuid.New(), chat.ID, senderID, repositoryTestTime())
		other := newMessageHistoryMessage(
			t,
			uuid.New(),
			otherChat.ID,
			otherSenderID,
			repositoryTestTime().Add(time.Hour),
		)
		insertMessageRepositoryMessage(t, pool, expected)
		insertMessageRepositoryMessage(t, pool, other)

		messages, err := repository.GetMessages(t.Context(), chat.ID, nil, 10, false)

		require.NoError(t, err)
		requireMessageHistoryIDs(t, messages, expected.ID)
	})

	t.Run("rejects invalid message restored from database", func(t *testing.T) {
		repository, chat, senderID := newMessageHistoryFixture(t, pool, config.Timeout)
		_, err := pool.Exec(t.Context(), `
			INSERT INTO messages (
				id, client_message_id, chat_id, sender_id, content, created_at
			)
			VALUES ($1, $2, $3, $4, '', $5)
		`, uuid.New(), uuid.New(), chat.ID, senderID, repositoryTestTime())
		require.NoError(t, err)

		messages, err := repository.GetMessages(t.Context(), chat.ID, nil, 10, false)

		require.ErrorIs(t, err, domain.ErrInvalidMessage)
		require.Nil(t, messages)
	})

	t.Run("reads uncommitted message from transaction context", func(t *testing.T) {
		repository, chat, senderID := newMessageHistoryFixture(t, pool, config.Timeout)
		message := newMessageHistoryMessage(t, uuid.New(), chat.ID, senderID, repositoryTestTime())
		manager := postgres.NewTransactionManager(pool)
		rollbackErr := errors.New("rollback message history transaction")
		var inside []domain.Message

		err := manager.WithinTransaction(t.Context(), func(ctx context.Context) error {
			db := postgres.GetExecutor(ctx, pool)
			_, err := db.Exec(ctx, `
				INSERT INTO messages (
					id, client_message_id, chat_id, sender_id, content, created_at, updated_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`,
				message.ID,
				message.ClientMessageID,
				message.ChatID,
				message.SenderID,
				message.Content,
				message.CreatedAt,
				message.UpdatedAt,
			)
			if err != nil {
				return err
			}
			inside, err = repository.GetMessages(ctx, chat.ID, nil, 10, false)
			if err != nil {
				return err
			}
			return rollbackErr
		})

		require.ErrorIs(t, err, rollbackErr)
		requireMessageHistoryIDs(t, inside, message.ID)
		after, err := repository.GetMessages(t.Context(), chat.ID, nil, 10, false)
		require.NoError(t, err)
		require.Empty(t, after)
	})
}

func newMessageHistoryFixture(
	t *testing.T,
	pool *pgxpool.Pool,
	timeout time.Duration,
) (*Repository, domain.Chat, uuid.UUID) {
	t.Helper()
	repository, chat, participantID := newMessageRepositoryFixture(t, pool, timeout)
	_, err := pool.Exec(t.Context(), `
		INSERT INTO chat_participants (chat_id, user_id, joined_at)
		VALUES ($1, $2, $3)
	`, chat.ID, participantID, chat.CreatedAt)
	require.NoError(t, err)
	return repository, chat, participantID
}

func newMessageHistoryMessage(
	t *testing.T,
	id, chatID, senderID uuid.UUID,
	createdAt time.Time,
) domain.Message {
	t.Helper()
	message, err := domain.NewMessage(
		id,
		uuid.New(),
		chatID,
		senderID,
		"Message history test",
		createdAt,
	)
	require.NoError(t, err)
	return message
}

func requireMessageHistoryIDs(
	t *testing.T,
	messages []domain.Message,
	expected ...uuid.UUID,
) {
	t.Helper()
	actual := make([]uuid.UUID, 0, len(messages))
	for _, message := range messages {
		actual = append(actual, message.ID)
	}
	require.Equal(t, expected, actual)
}
