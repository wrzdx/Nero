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

type markAsReadTxContextKey struct{}

func TestMarkAsRead(t *testing.T) {
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	messageID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	command := MarkAsReadCommand{
		ChatID:    chatID,
		UserID:    userID,
		MessageID: messageID,
	}

	t.Run("rejects invalid command before repository calls", func(t *testing.T) {
		service := NewMessagesService(
			NewMockMessagesRepository(t),
			NewMockChatsRepository(t),
			NewMockTXManager(t), NewMockNotifier(t),
		)

		err := service.MarkAsRead(t.Context(), MarkAsReadCommand{})

		require.ErrorIs(t, err, ErrInvalidInput)
		var detailed domain.DetailedError
		require.ErrorAs(t, err, &detailed)
		require.Equal(t, map[string]string{
			"chat_id":    "chat id is nil",
			"user_id":    "user id is nil",
			"message_id": "message id is nil",
		}, detailed.Details)
	})

	t.Run("returns participant check error before transaction", func(t *testing.T) {
		participantErr := errors.New("participant lookup failed")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, userID).
			Return(participantErr)
		service := NewMessagesService(
			repository,
			NewMockChatsRepository(t),
			NewMockTXManager(t), NewMockNotifier(t),
		)

		err := service.MarkAsRead(t.Context(), command)

		require.ErrorIs(t, err, participantErr)
	})

	t.Run("locks chat and marks message inside the same transaction", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, markAsReadTxContextKey{}, "transaction")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(outerCtx, chatID, userID).Return(nil)
		chatsRepository := NewMockChatsRepository(t)
		lockCall := chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).
			Return(newMarkAsReadTestChat(chatID), nil)
		markCall := repository.EXPECT().MarkAsRead(txCtx, chatID, userID, messageID).
			Return(nil)
		mock.InOrder(lockCall.Call, markCall.Call)
		txManager := NewMockTXManager(t)
		expectMarkAsReadTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.MarkAsRead(outerCtx, command)

		require.NoError(t, err)
	})

	t.Run("returns chat lock error without updating read pointer", func(t *testing.T) {
		lockErr := errors.New("chat lock failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, markAsReadTxContextKey{}, "transaction")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(outerCtx, chatID, userID).Return(nil)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).
			Return(domain.Chat{}, lockErr)
		txManager := NewMockTXManager(t)
		expectMarkAsReadTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.MarkAsRead(outerCtx, command)

		require.ErrorIs(t, err, lockErr)
	})

	t.Run("returns repository update error", func(t *testing.T) {
		updateErr := errors.New("read pointer update failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, markAsReadTxContextKey{}, "transaction")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(outerCtx, chatID, userID).Return(nil)
		repository.EXPECT().MarkAsRead(txCtx, chatID, userID, messageID).
			Return(updateErr)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().GetChatForUpdate(txCtx, chatID).
			Return(newMarkAsReadTestChat(chatID), nil)
		txManager := NewMockTXManager(t)
		expectMarkAsReadTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(repository, chatsRepository, txManager, NewMockNotifier(t))

		err := service.MarkAsRead(outerCtx, command)

		require.ErrorIs(t, err, updateErr)
	})

	t.Run("returns transaction manager error", func(t *testing.T) {
		transactionErr := errors.New("cannot begin transaction")
		repository := NewMockMessagesRepository(t)
		repository.EXPECT().CheckParticipant(t.Context(), chatID, userID).Return(nil)
		txManager := NewMockTXManager(t)
		txManager.EXPECT().WithinTransaction(t.Context(), mock.Anything).
			Return(transactionErr)
		service := NewMessagesService(repository, NewMockChatsRepository(t), txManager, NewMockNotifier(t))

		err := service.MarkAsRead(t.Context(), command)

		require.ErrorIs(t, err, transactionErr)
	})
}

func expectMarkAsReadTransaction(
	manager *MockTXManager,
	outerCtx, txCtx context.Context,
) {
	manager.EXPECT().WithinTransaction(outerCtx, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			return fn(txCtx)
		})
}

func newMarkAsReadTestChat(chatID uuid.UUID) domain.Chat {
	createdAt := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	return domain.Chat{
		ID:             chatID,
		Type:           domain.ChatTypeDirect,
		LastActivityAt: createdAt,
		CreatedAt:      createdAt,
	}
}
