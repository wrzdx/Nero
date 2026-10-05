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

type deleteMessageTxContextKey struct{}

func TestDeleteMessage(t *testing.T) {
	senderID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	messageID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	existing := newEditMessageTestMessage(t, messageID, chatID, senderID, "message")
	command := DeleteMessageCommand{
		ChatID:    chatID,
		SenderID:  senderID,
		MessageID: messageID,
	}

	t.Run("rejects invalid command before repository calls", func(t *testing.T) {
		service := NewMessagesService(
			NewMockMessagesRepository(t),
			NewMockChatsRepository(t),
			NewMockTXManager(t), NewMockNotifier(t),
		)

		err := service.DeleteMessage(t.Context(), DeleteMessageCommand{})

		require.ErrorIs(t, err, ErrInvalidInput)
		var detailed domain.DetailedError
		require.ErrorAs(t, err, &detailed)
		require.Equal(t, map[string]string{
			"chat_id":    "chat id is nil",
			"sender_id":  "sender id is nil",
			"message_id": "message id is nil",
		}, detailed.Details)
	})

	t.Run("returns participant check error before reading message", func(t *testing.T) {
		participantErr := errors.New("participant lookup failed")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, senderID).Return(participantErr)
		service := NewMessagesService(repository, NewMockChatsRepository(t), NewMockTXManager(t), NewMockNotifier(t))

		err := service.DeleteMessage(t.Context(), command)

		require.ErrorIs(t, err, participantErr)
	})

	t.Run("returns message lookup error before transaction", func(t *testing.T) {
		lookupErr := errors.New("message lookup failed")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, senderID).Return(nil)
		repository.EXPECT().GetMessage(t.Context(), messageID).Return(domain.Message{}, lookupErr)
		service := NewMessagesService(repository, NewMockChatsRepository(t), NewMockTXManager(t), NewMockNotifier(t))

		err := service.DeleteMessage(t.Context(), command)

		require.ErrorIs(t, err, lookupErr)
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
			name: "does not allow deleting another author's message",
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

			err := service.DeleteMessage(t.Context(), command)

			require.ErrorIs(t, err, domain.ErrNotFound)
		})
	}

	t.Run("deletes a non-last message without changing chat state", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		lastMessageID := uuid.New()
		chat := newDeleteMessageTestChat(t, chatID, &lastMessageID)
		previous := newEditMessageTestMessage(t, uuid.New(), chatID, senderID, "previous")
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		repository.EXPECT().GetMessages(txCtx, chatID, deleteMessageCursor(existing), 1, false).
			Return([]domain.Message{previous}, nil)
		repository.EXPECT().DeleteMessage(txCtx, messageID).Return(nil)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(chat, nil)
		expectLastReadMessageUpdate(chatsRepository, txCtx, chatID, messageID, &previous.ID, nil)
		txManager := NewMockTXManager(t)
		notifier := expectCommittedDeletionNotification(t, txManager, outerCtx, txCtx, chatID, messageID)
		service := NewMessagesService(repository, chatsRepository, txManager, notifier)

		err := service.DeleteMessage(outerCtx, command)

		require.NoError(t, err)
	})

	t.Run("clears last message when deleting the only message", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		chat := newDeleteMessageTestChat(t, chatID, &messageID)
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		repository.EXPECT().GetMessages(txCtx, chatID, deleteMessageCursor(existing), 1, false).
			Return(nil, nil)
		repository.EXPECT().DeleteMessage(txCtx, messageID).Return(nil)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(chat, nil)
		expectLastReadMessageUpdate(chatsRepository, txCtx, chatID, messageID, nil, nil)
		chatsRepository.EXPECT().UpdateChatLastMsgID(txCtx, chatID, (*uuid.UUID)(nil)).Return(nil)
		txManager := NewMockTXManager(t)
		notifier := expectCommittedDeletionNotification(t, txManager, outerCtx, txCtx, chatID, messageID)
		service := NewMessagesService(repository, chatsRepository, txManager, notifier)

		err := service.DeleteMessage(outerCtx, command)

		require.NoError(t, err)
	})

	t.Run("moves last message pointer to the previous message", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		chat := newDeleteMessageTestChat(t, chatID, &messageID)
		previous := newEditMessageTestMessage(t, uuid.New(), chatID, senderID, "previous")
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		repository.EXPECT().GetMessages(txCtx, chatID, deleteMessageCursor(existing), 1, false).
			Return([]domain.Message{previous}, nil)
		repository.EXPECT().DeleteMessage(txCtx, messageID).Return(nil)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(chat, nil)
		expectLastReadMessageUpdate(chatsRepository, txCtx, chatID, messageID, &previous.ID, nil)
		chatsRepository.EXPECT().UpdateChatLastMsgID(
			txCtx,
			chatID,
			mock.MatchedBy(func(id *uuid.UUID) bool {
				return id != nil && *id == previous.ID
			}),
		).Return(nil)
		txManager := NewMockTXManager(t)
		notifier := expectCommittedDeletionNotification(t, txManager, outerCtx, txCtx, chatID, messageID)
		service := NewMessagesService(repository, chatsRepository, txManager, notifier)

		err := service.DeleteMessage(outerCtx, command)

		require.NoError(t, err)
	})

	t.Run("returns locked chat lookup error", func(t *testing.T) {
		lookupErr := errors.New("chat lookup failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(domain.Chat{}, lookupErr)
		txManager := NewMockTXManager(t)
		expectDeleteMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.DeleteMessage(outerCtx, command)

		require.ErrorIs(t, err, lookupErr)
	})

	t.Run("rejects inconsistent chat without last message pointer", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		chat := newDeleteMessageTestChat(t, chatID, nil)
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(chat, nil)
		txManager := NewMockTXManager(t)
		expectDeleteMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.DeleteMessage(outerCtx, command)

		require.Error(t, err)
		require.Contains(t, err.Error(), "chat has messages but no last message id")
	})

	t.Run("returns last messages lookup error", func(t *testing.T) {
		lookupErr := errors.New("messages lookup failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		chat := newDeleteMessageTestChat(t, chatID, &messageID)
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		repository.EXPECT().GetMessages(txCtx, chatID, deleteMessageCursor(existing), 1, false).
			Return(nil, lookupErr)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(chat, nil)
		txManager := NewMockTXManager(t)
		expectDeleteMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.DeleteMessage(outerCtx, command)

		require.ErrorIs(t, err, lookupErr)
	})

	t.Run("rejects invalid previous message page length", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		chat := newDeleteMessageTestChat(t, chatID, &messageID)
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		repository.EXPECT().GetMessages(txCtx, chatID, deleteMessageCursor(existing), 1, false).
			Return(make([]domain.Message, 2), nil)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(chat, nil)
		txManager := NewMockTXManager(t)
		expectDeleteMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.DeleteMessage(outerCtx, command)

		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid number of message")
	})

	t.Run("returns last read message update error", func(t *testing.T) {
		updateErr := errors.New("last read update failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		chat := newDeleteMessageTestChat(t, chatID, &messageID)
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		repository.EXPECT().GetMessages(txCtx, chatID, deleteMessageCursor(existing), 1, false).
			Return(nil, nil)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(chat, nil)
		expectLastReadMessageUpdate(
			chatsRepository,
			txCtx,
			chatID,
			messageID,
			nil,
			updateErr,
		)
		txManager := NewMockTXManager(t)
		expectDeleteMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.DeleteMessage(outerCtx, command)

		require.ErrorIs(t, err, updateErr)
	})

	t.Run("returns chat state update error", func(t *testing.T) {
		updateErr := errors.New("chat update failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		chat := newDeleteMessageTestChat(t, chatID, &messageID)
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		repository.EXPECT().GetMessages(txCtx, chatID, deleteMessageCursor(existing), 1, false).
			Return(nil, nil)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(chat, nil)
		expectLastReadMessageUpdate(chatsRepository, txCtx, chatID, messageID, nil, nil)
		chatsRepository.EXPECT().UpdateChatLastMsgID(txCtx, chatID, (*uuid.UUID)(nil)).
			Return(updateErr)
		txManager := NewMockTXManager(t)
		expectDeleteMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.DeleteMessage(outerCtx, command)

		require.ErrorIs(t, err, updateErr)
	})

	t.Run("returns message delete error", func(t *testing.T) {
		deleteErr := errors.New("delete failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, deleteMessageTxContextKey{}, "transaction")
		lastMessageID := uuid.New()
		chat := newDeleteMessageTestChat(t, chatID, &lastMessageID)
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, outerCtx, command, existing)
		repository.EXPECT().GetMessages(txCtx, chatID, deleteMessageCursor(existing), 1, false).
			Return(nil, nil)
		repository.EXPECT().DeleteMessage(txCtx, messageID).Return(deleteErr)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).Return(chat, nil)
		expectLastReadMessageUpdate(chatsRepository, txCtx, chatID, messageID, nil, nil)
		txManager := NewMockTXManager(t)
		expectDeleteMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.DeleteMessage(outerCtx, command)

		require.ErrorIs(t, err, deleteErr)
	})

	t.Run("returns transaction manager error", func(t *testing.T) {
		transactionErr := errors.New("cannot begin transaction")
		repository := NewMockMessagesRepository(t)
		expectDeleteMessagePrelude(repository, t.Context(), command, existing)
		txManager := NewMockTXManager(t)
		txManager.EXPECT().WithinTransaction(t.Context(), mock.Anything).Return(transactionErr)
		service := NewMessagesService(repository, NewMockChatsRepository(t), txManager, NewMockNotifier(t))

		err := service.DeleteMessage(t.Context(), command)

		require.ErrorIs(t, err, transactionErr)
	})
}

func expectCommittedDeletionNotification(t *testing.T, tx *MockTXManager, outerCtx, txCtx context.Context, chatID, messageID uuid.UUID) *MockNotifier {
	t.Helper()
	committed := false
	tx.EXPECT().WithinTransaction(outerCtx, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			if err := fn(txCtx); err != nil {
				return err
			}
			committed = true
			return nil
		}).Once()
	notifier := NewMockNotifier(t)
	notifier.EXPECT().MessageDeleted(outerCtx, chatID, messageID).
		Run(func(context.Context, uuid.UUID, uuid.UUID) {
			require.True(t, committed, "notification must follow commit")
		}).Once()
	return notifier
}

func TestDeleteMessageDoesNotNotifyWhenCommitFails(t *testing.T) {
	ctx := t.Context()
	txCtx := context.WithValue(ctx, deleteMessageTxContextKey{}, "transaction")
	command := DeleteMessageCommand{ChatID: uuid.New(), SenderID: uuid.New(), MessageID: uuid.New()}
	existing := newEditMessageTestMessage(t, command.MessageID, command.ChatID, command.SenderID, "message")
	lastID := uuid.New()
	chat := newDeleteMessageTestChat(t, command.ChatID, &lastID)
	repo := NewMockMessagesRepository(t)
	chats := NewMockChatsRepository(t)
	tx := NewMockTXManager(t)
	notifier := NewMockNotifier(t)
	expectDeleteMessagePrelude(repo, ctx, command, existing)
	chats.EXPECT().GetChatForUpdate(txCtx, command.ChatID).Return(chat, nil).Once()
	repo.EXPECT().GetMessages(txCtx, command.ChatID, deleteMessageCursor(existing), 1, false).Return(nil, nil).Once()
	expectLastReadMessageUpdate(chats, txCtx, command.ChatID, command.MessageID, nil, nil)
	repo.EXPECT().DeleteMessage(txCtx, command.MessageID).Return(nil).Once()
	commitErr := errors.New("commit failed")
	tx.EXPECT().WithinTransaction(ctx, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			require.NoError(t, fn(txCtx))
			return commitErr
		}).Once()
	err := NewMessagesService(repo, chats, tx, notifier).DeleteMessage(ctx, command)
	require.ErrorIs(t, err, commitErr)
}

func expectDeleteMessagePrelude(
	repository *MockMessagesRepository,
	ctx context.Context,
	command DeleteMessageCommand,
	existing domain.Message,
) {
	repository.EXPECT().CheckParticipant(ctx, command.ChatID, command.SenderID).Return(nil)
	repository.EXPECT().GetMessage(ctx, command.MessageID).Return(existing, nil)
}

func expectDeleteMessageTransaction(
	manager *MockTXManager,
	outerCtx context.Context,
	txCtx context.Context,
) {
	manager.EXPECT().WithinTransaction(outerCtx, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			return fn(txCtx)
		})
}

func deleteMessageCursor(message domain.Message) *MessageCursor {
	return &MessageCursor{
		MessageID: message.ID,
		CreatedAt: message.CreatedAt,
	}
}

func expectLastReadMessageUpdate(
	repository *MockChatsRepository,
	ctx context.Context,
	chatID, oldMessageID uuid.UUID,
	newMessageID *uuid.UUID,
	err error,
) {
	repository.EXPECT().UpdateLastReadMessages(
		ctx,
		chatID,
		oldMessageID,
		mock.MatchedBy(func(actual *uuid.UUID) bool {
			if newMessageID == nil {
				return actual == nil
			}
			return actual != nil && *actual == *newMessageID
		}),
	).Return(err)
}

func newDeleteMessageTestChat(
	t *testing.T,
	chatID uuid.UUID,
	lastMessageID *uuid.UUID,
) domain.Chat {
	t.Helper()
	createdAt := time.Now().Add(-time.Hour)
	chat := domain.Chat{
		ID:             chatID,
		Type:           domain.ChatTypeDirect,
		LastMessageID:  lastMessageID,
		LastActivityAt: createdAt,
		CreatedAt:      createdAt,
	}
	require.NoError(t, chat.Validate())
	return chat
}
