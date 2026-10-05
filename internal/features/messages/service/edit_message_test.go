package messages_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestEditMessage(t *testing.T) {
	senderID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	messageID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	existing := newEditMessageTestMessage(t, messageID, chatID, senderID, "old content")
	command := UpdateMessageCommand{
		SenderID:  senderID,
		ChatID:    chatID,
		MessageID: messageID,
		Content:   "  new content  ",
	}

	t.Run("rejects invalid command before repository calls", func(t *testing.T) {
		service := NewMessagesService(
			NewMockMessagesRepository(t),
			NewMockChatsRepository(t),
			NewMockTXManager(t), NewMockNotifier(t),
		)

		actual, err := service.EditMessage(t.Context(), UpdateMessageCommand{})

		require.ErrorIs(t, err, ErrInvalidInput)
		require.Zero(t, actual)
		var detailed domain.DetailedError
		require.ErrorAs(t, err, &detailed)
		require.Equal(t, map[string]string{
			"sender_id":  "sender id is nil",
			"chat_id":    "chat id is nil",
			"message_id": "message_id is nil",
		}, detailed.Details)
	})

	t.Run("updates own message as active participant", func(t *testing.T) {
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, senderID).Return(nil)
		repository.EXPECT().GetMessage(t.Context(), messageID).Return(existing, nil)
		var persisted domain.Message
		repository.EXPECT().
			UpdateMessage(t.Context(), mock.Anything).
			Run(func(_ context.Context, updated domain.Message) {
				persisted = updated
			}).
			Return(nil)
		notifier := NewMockNotifier(t)
		notifier.EXPECT().MessageEdited(t.Context(), mock.Anything).
			Run(func(_ context.Context, message domain.Message) {
				require.NotZero(t, persisted.ID, "notify only after persistence")
				require.Equal(t, persisted, message)
			}).Once()
		service := NewMessagesService(repository, NewMockChatsRepository(t), NewMockTXManager(t), notifier)
		startedAt := time.Now()

		actual, err := service.EditMessage(t.Context(), command)

		require.NoError(t, err)
		require.Equal(t, persisted, actual)
		require.Equal(t, "new content", actual.Content)
		require.NotNil(t, actual.UpdatedAt)
		require.False(t, actual.UpdatedAt.Before(startedAt))
		require.False(t, actual.UpdatedAt.Before(actual.CreatedAt))
	})

	t.Run("treats the same normalized content as a no-op", func(t *testing.T) {
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, senderID).Return(nil)
		repository.EXPECT().GetMessage(t.Context(), messageID).Return(existing, nil)
		service := NewMessagesService(repository, NewMockChatsRepository(t), NewMockTXManager(t), NewMockNotifier(t))
		noOp := command
		noOp.Content = "  old content  "

		actual, err := service.EditMessage(t.Context(), noOp)

		require.NoError(t, err)
		require.Equal(t, existing, actual)
	})

	t.Run("returns participant check error before reading message", func(t *testing.T) {
		participantErr := errors.New("participant lookup failed")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, senderID).Return(participantErr)
		service := NewMessagesService(repository, NewMockChatsRepository(t), NewMockTXManager(t), NewMockNotifier(t))

		actual, err := service.EditMessage(t.Context(), command)

		require.ErrorIs(t, err, participantErr)
		require.Zero(t, actual)
	})

	t.Run("returns message lookup error", func(t *testing.T) {
		lookupErr := errors.New("message lookup failed")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, senderID).Return(nil)
		repository.EXPECT().GetMessage(t.Context(), messageID).Return(domain.Message{}, lookupErr)
		service := NewMessagesService(repository, NewMockChatsRepository(t), NewMockTXManager(t), NewMockNotifier(t))

		actual, err := service.EditMessage(t.Context(), command)

		require.ErrorIs(t, err, lookupErr)
		require.Zero(t, actual)
	})

	for _, testCase := range []struct {
		name     string
		existing domain.Message
	}{
		{
			name: "hides message from another chat",
			existing: func() domain.Message {
				message := existing
				message.ChatID = uuid.New()
				return message
			}(),
		},
		{
			name: "hides another sender's message",
			existing: func() domain.Message {
				message := existing
				message.SenderID = uuid.New()
				return message
			}(),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := NewMockMessagesRepository(t)
			repository.EXPECT().CheckParticipant(t.Context(), chatID, senderID).Return(nil)
			repository.EXPECT().GetMessage(t.Context(), messageID).Return(testCase.existing, nil)
			service := NewMessagesService(repository, NewMockChatsRepository(t), NewMockTXManager(t), NewMockNotifier(t))

			actual, err := service.EditMessage(t.Context(), command)

			require.ErrorIs(t, err, domain.ErrNotFound)
			require.Zero(t, actual)
		})
	}

	t.Run("returns invalid message for invalid content", func(t *testing.T) {
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, senderID).Return(nil)
		repository.EXPECT().GetMessage(t.Context(), messageID).Return(existing, nil)
		service := NewMessagesService(repository, NewMockChatsRepository(t), NewMockTXManager(t), NewMockNotifier(t))
		invalid := command
		invalid.Content = "   "

		actual, err := service.EditMessage(t.Context(), invalid)

		require.ErrorIs(t, err, domain.ErrInvalidMessage)
		require.Zero(t, actual)
	})

	t.Run("returns update error", func(t *testing.T) {
		updateErr := errors.New("update failed")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, senderID).Return(nil)
		repository.EXPECT().GetMessage(t.Context(), messageID).Return(existing, nil)
		repository.EXPECT().UpdateMessage(t.Context(), mock.Anything).Return(updateErr)
		service := NewMessagesService(repository, NewMockChatsRepository(t), NewMockTXManager(t), NewMockNotifier(t))

		actual, err := service.EditMessage(t.Context(), command)

		require.ErrorIs(t, err, updateErr)
		require.Zero(t, actual)
	})
}

func TestUpdateMessageCommandValidate(t *testing.T) {
	valid := UpdateMessageCommand{
		SenderID:  uuid.New(),
		ChatID:    uuid.New(),
		MessageID: uuid.New(),
		Content:   "content",
	}
	require.NoError(t, valid.Validate())

	invalid := UpdateMessageCommand{}
	err := invalid.Validate()
	require.ErrorIs(t, err, ErrInvalidInput)
	var detailed domain.DetailedError
	require.ErrorAs(t, err, &detailed)
	require.Equal(t, map[string]string{
		"sender_id":  "sender id is nil",
		"chat_id":    "chat id is nil",
		"message_id": "message_id is nil",
	}, detailed.Details)
}

func newEditMessageTestMessage(
	t *testing.T,
	messageID, chatID, senderID uuid.UUID,
	content string,
) domain.Message {
	t.Helper()
	message, err := domain.NewMessage(
		messageID,
		uuid.New(),
		chatID,
		senderID,
		content,
		time.Now().Add(-time.Minute),
	)
	require.NoError(t, err)
	return message
}
