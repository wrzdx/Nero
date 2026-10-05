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

type sendMessageTxContextKey struct{}

func TestSendMessage(t *testing.T) {
	senderID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	peerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	outsiderID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	chatID := uuid.New()
	clientMessageID := uuid.New()
	command := SendMessageCommand{
		SenderID:        senderID,
		ChatID:          chatID,
		ClientMessageID: clientMessageID,
		Content:         "  Hello  ",
	}

	t.Run("rejects invalid message before repository calls", func(t *testing.T) {
		service := NewMessagesService(
			NewMockMessagesRepository(t),
			NewMockChatsRepository(t),
			NewMockTXManager(t), NewMockNotifier(t),
		)

		actual, created, err := service.SendMessage(t.Context(), SendMessageCommand{
			SenderID: senderID,
			ChatID:   chatID,
		})

		require.ErrorIs(t, err, domain.ErrInvalidMessage)
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("returns matching existing message without transaction", func(t *testing.T) {
		existing := newSendMessageTestMessage(t, senderID, chatID, clientMessageID, "Hello")
		messagesRepository := NewMockMessagesRepository(t)
		messagesRepository.EXPECT().
			GetMessageByClientID(t.Context(), senderID, clientMessageID).
			Return(existing, nil)
		service := NewMessagesService(
			messagesRepository,
			NewMockChatsRepository(t),
			NewMockTXManager(t), NewMockNotifier(t),
		)

		actual, created, err := service.SendMessage(t.Context(), command)

		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, existing, actual)
	})

	t.Run("returns conflict when existing message payload differs", func(t *testing.T) {
		existing := newSendMessageTestMessage(t, senderID, chatID, clientMessageID, "Different")
		messagesRepository := NewMockMessagesRepository(t)
		messagesRepository.EXPECT().
			GetMessageByClientID(t.Context(), senderID, clientMessageID).
			Return(existing, nil)
		service := NewMessagesService(
			messagesRepository,
			NewMockChatsRepository(t),
			NewMockTXManager(t), NewMockNotifier(t),
		)

		actual, created, err := service.SendMessage(t.Context(), command)

		require.ErrorIs(t, err, ErrMessageConflict)
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("returns initial message lookup error", func(t *testing.T) {
		lookupErr := errors.New("database unavailable")
		messagesRepository := NewMockMessagesRepository(t)
		messagesRepository.EXPECT().
			GetMessageByClientID(t.Context(), senderID, clientMessageID).
			Return(domain.Message{}, lookupErr)
		service := NewMessagesService(
			messagesRepository,
			NewMockChatsRepository(t),
			NewMockTXManager(t), NewMockNotifier(t),
		)

		actual, created, err := service.SendMessage(t.Context(), command)

		require.ErrorIs(t, err, lookupErr)
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("appends direct message for active participants", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		var appended domain.Message
		appendCall := messagesRepository.EXPECT().
			AppendMessage(txCtx, mock.Anything).
			Run(func(_ context.Context, message domain.Message) {
				appended = message
			}).
			Return(nil)
		markCall := messagesRepository.EXPECT().
			MarkAsRead(txCtx, chatID, senderID, mock.MatchedBy(func(messageID uuid.UUID) bool {
				return appended.ID != uuid.Nil && messageID == appended.ID
			})).
			Return(nil)
		mock.InOrder(appendCall.Call, markCall.Call)
		chatsRepository := NewMockChatsRepository(t)
		var chatLockedAt time.Time
		chatsRepository.EXPECT().
			GetChatForUpdate(txCtx, chatID).
			RunAndReturn(func(context.Context, uuid.UUID) (domain.Chat, error) {
				chatLockedAt = time.Now()
				return direct.Chat, nil
			})
		chatsRepository.EXPECT().
			GetDirectMessageState(txCtx, chatID).
			Return(activeDirectMessageState(direct), nil)
		txManager := NewMockTXManager(t)
		notifier := expectCommittedMessageNotification(t, txManager, outerCtx, txCtx, &appended)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, notifier)

		actual, created, err := service.SendMessage(outerCtx, command)

		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, appended, actual)
		require.Equal(t, senderID, actual.SenderID)
		require.Equal(t, chatID, actual.ChatID)
		require.Equal(t, clientMessageID, actual.ClientMessageID)
		require.Equal(t, "Hello", actual.Content)
		require.False(t, actual.CreatedAt.Before(chatLockedAt))
	})

	t.Run("hides direct chat from non-participant", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, outsiderID, clientMessageID)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, direct.Chat)
		chatsRepository.EXPECT().
			GetDirectMessageState(txCtx, chatID).
			Return(activeDirectMessageState(direct), nil)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))
		outsiderCommand := command
		outsiderCommand.SenderID = outsiderID
		actual, created, err := service.SendMessage(outerCtx, outsiderCommand)

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.False(t, created)
		require.Zero(t, actual)
	})

	for name, deletedIndex := range map[string]int{
		"first participant deleted":  0,
		"second participant deleted": 1,
	} {
		t.Run(name, func(t *testing.T) {
			outerCtx := t.Context()
			txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
			direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
			state := activeDirectMessageState(direct)
			state.Users[deletedIndex].Deleted = true
			messagesRepository := NewMockMessagesRepository(t)
			expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
			chatsRepository := NewMockChatsRepository(t)
			expectLockedChat(chatsRepository, txCtx, direct.Chat)
			chatsRepository.EXPECT().
				GetDirectMessageState(txCtx, chatID).
				Return(state, nil)
			txManager := NewMockTXManager(t)
			expectSendMessageTransaction(txManager, outerCtx, txCtx)
			service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

			actual, created, err := service.SendMessage(outerCtx, command)

			require.ErrorIs(t, err, ErrMessageTargetUnavailable)
			require.False(t, created)
			require.Zero(t, actual)
		})
	}

	t.Run("appends group message for active participant", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		groupChat := newSendMessageTestGroup(t, chatID)
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		var appended domain.Message
		appendCall := messagesRepository.EXPECT().
			AppendMessage(txCtx, mock.Anything).
			Run(func(_ context.Context, message domain.Message) {
				appended = message
			}).
			Return(nil)
		markCall := messagesRepository.EXPECT().
			MarkAsRead(txCtx, chatID, senderID, mock.MatchedBy(func(messageID uuid.UUID) bool {
				return appended.ID != uuid.Nil && messageID == appended.ID
			})).
			Return(nil)
		mock.InOrder(appendCall.Call, markCall.Call)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, groupChat.Chat)
		chatsRepository.EXPECT().
			GetGroupSenderState(txCtx, chatID, senderID).
			Return(AccountState{UserID: senderID}, nil)
		txManager := NewMockTXManager(t)
		notifier := expectCommittedMessageNotification(t, txManager, outerCtx, txCtx, &appended)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, notifier)

		actual, created, err := service.SendMessage(outerCtx, command)

		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, appended, actual)
	})

	t.Run("rejects group message from deleted participant", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		groupChat := newSendMessageTestGroup(t, chatID)
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, groupChat.Chat)
		chatsRepository.EXPECT().
			GetGroupSenderState(txCtx, chatID, senderID).
			Return(AccountState{UserID: senderID, Deleted: true}, nil)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(outerCtx, command)

		require.ErrorIs(t, err, ErrMessageTargetUnavailable)
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("returns chat lookup error", func(t *testing.T) {
		lookupErr := errors.New("chat lookup failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			GetChatForUpdate(txCtx, chatID).
			Return(domain.Chat{}, lookupErr)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(outerCtx, command)

		require.ErrorIs(t, err, lookupErr)
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("reports inconsistency when locked direct has no direct state", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, direct.Chat)
		chatsRepository.EXPECT().
			GetDirectMessageState(txCtx, chatID).
			Return(DirectMessageState{}, domain.ErrNotFound)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(outerCtx, command)

		require.ErrorIs(t, err, ErrInternalInconsistency)
		require.NotErrorIs(t, err, domain.ErrNotFound)
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("returns append error", func(t *testing.T) {
		appendErr := errors.New("insert failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		messagesRepository.EXPECT().AppendMessage(txCtx, mock.Anything).Return(appendErr)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, direct.Chat)
		chatsRepository.EXPECT().
			GetDirectMessageState(txCtx, chatID).
			Return(activeDirectMessageState(direct), nil)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(outerCtx, command)

		require.ErrorIs(t, err, appendErr)
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("returns mark as read error", func(t *testing.T) {
		markErr := errors.New("mark as read failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		var appended domain.Message
		appendCall := messagesRepository.EXPECT().
			AppendMessage(txCtx, mock.Anything).
			Run(func(_ context.Context, message domain.Message) {
				appended = message
			}).
			Return(nil)
		markCall := messagesRepository.EXPECT().
			MarkAsRead(txCtx, chatID, senderID, mock.MatchedBy(func(messageID uuid.UUID) bool {
				return appended.ID != uuid.Nil && messageID == appended.ID
			})).
			Return(markErr)
		mock.InOrder(appendCall.Call, markCall.Call)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, direct.Chat)
		chatsRepository.EXPECT().
			GetDirectMessageState(txCtx, chatID).
			Return(activeDirectMessageState(direct), nil)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(outerCtx, command)

		require.ErrorIs(t, err, markErr)
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("reports inconsistency when append cannot update locked chat", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		messagesRepository.EXPECT().AppendMessage(txCtx, mock.Anything).Return(domain.ErrNotFound)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, direct.Chat)
		chatsRepository.EXPECT().
			GetDirectMessageState(txCtx, chatID).
			Return(activeDirectMessageState(direct), nil)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(outerCtx, command)

		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrNotFound)
		require.Contains(t, err.Error(), "internal inconsistency")
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("returns existing message after concurrent idempotency conflict", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
		existing := newSendMessageTestMessage(t, senderID, chatID, clientMessageID, "Hello")
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		messagesRepository.EXPECT().AppendMessage(txCtx, mock.Anything).Return(domain.ErrAlreadyExists)
		messagesRepository.EXPECT().
			GetMessageByClientID(outerCtx, senderID, clientMessageID).
			Return(existing, nil)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, direct.Chat)
		chatsRepository.EXPECT().
			GetDirectMessageState(txCtx, chatID).
			Return(activeDirectMessageState(direct), nil)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(outerCtx, command)

		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, existing, actual)
	})

	t.Run("returns conflict after concurrent reuse with different payload", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
		existing := newSendMessageTestMessage(t, senderID, chatID, clientMessageID, "Different")
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		messagesRepository.EXPECT().AppendMessage(txCtx, mock.Anything).Return(domain.ErrAlreadyExists)
		messagesRepository.EXPECT().
			GetMessageByClientID(outerCtx, senderID, clientMessageID).
			Return(existing, nil)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, direct.Chat)
		chatsRepository.EXPECT().
			GetDirectMessageState(txCtx, chatID).
			Return(activeDirectMessageState(direct), nil)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(outerCtx, command)

		require.ErrorIs(t, err, ErrMessageConflict)
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("reports inconsistency when conflicting message cannot be found", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, sendMessageTxContextKey{}, "transaction")
		direct := newSendMessageTestDirect(t, chatID, senderID, peerID)
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, outerCtx, senderID, clientMessageID)
		messagesRepository.EXPECT().AppendMessage(txCtx, mock.Anything).Return(domain.ErrAlreadyExists)
		messagesRepository.EXPECT().
			GetMessageByClientID(outerCtx, senderID, clientMessageID).
			Return(domain.Message{}, domain.ErrNotFound)
		chatsRepository := NewMockChatsRepository(t)
		expectLockedChat(chatsRepository, txCtx, direct.Chat)
		chatsRepository.EXPECT().
			GetDirectMessageState(txCtx, chatID).
			Return(activeDirectMessageState(direct), nil)
		txManager := NewMockTXManager(t)
		expectSendMessageTransaction(txManager, outerCtx, txCtx)
		service := NewMessagesService(messagesRepository, chatsRepository, txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(outerCtx, command)

		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrNotFound)
		require.Contains(t, err.Error(), "internal inconsistency")
		require.False(t, created)
		require.Zero(t, actual)
	})

	t.Run("returns transaction manager error", func(t *testing.T) {
		transactionErr := errors.New("cannot begin transaction")
		messagesRepository := NewMockMessagesRepository(t)
		expectMessageNotFound(messagesRepository, t.Context(), senderID, clientMessageID)
		txManager := NewMockTXManager(t)
		txManager.EXPECT().WithinTransaction(t.Context(), mock.Anything).Return(transactionErr)
		service := NewMessagesService(messagesRepository, NewMockChatsRepository(t), txManager, NewMockNotifier(t))

		actual, created, err := service.SendMessage(t.Context(), command)

		require.ErrorIs(t, err, transactionErr)
		require.False(t, created)
		require.Zero(t, actual)
	})
}

func expectCommittedMessageNotification(
	t *testing.T,
	txManager *MockTXManager,
	outerCtx, txCtx context.Context,
	appended *domain.Message,
) *MockNotifier {
	t.Helper()
	committed := false
	txManager.EXPECT().WithinTransaction(outerCtx, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			if err := fn(txCtx); err != nil {
				return err
			}
			committed = true
			return nil
		}).Once()
	notifier := NewMockNotifier(t)
	notifier.EXPECT().MessageCreated(outerCtx, mock.Anything).
		Run(func(_ context.Context, message domain.Message) {
			require.True(t, committed, "notification must happen after commit")
			require.Equal(t, *appended, message)
		}).Once()
	return notifier
}

func TestSendMessageDoesNotNotifyWhenCommitFails(t *testing.T) {
	ctx := t.Context()
	txCtx := context.WithValue(ctx, sendMessageTxContextKey{}, "transaction")
	command := SendMessageCommand{SenderID: uuid.New(), ChatID: uuid.New(), ClientMessageID: uuid.New(), Content: "hello"}
	group := newSendMessageTestGroup(t, command.ChatID)
	messages := NewMockMessagesRepository(t)
	chats := NewMockChatsRepository(t)
	tx := NewMockTXManager(t)
	notifier := NewMockNotifier(t) // No call is permitted, even after successful INSERT.
	expectMessageNotFound(messages, ctx, command.SenderID, command.ClientMessageID)
	expectLockedChat(chats, txCtx, group.Chat)
	chats.EXPECT().GetGroupSenderState(txCtx, command.ChatID, command.SenderID).
		Return(AccountState{UserID: command.SenderID}, nil).Once()
	messages.EXPECT().AppendMessage(txCtx, mock.Anything).Return(nil).Once()
	messages.EXPECT().MarkAsRead(txCtx, command.ChatID, command.SenderID, mock.Anything).Return(nil).Once()
	commitErr := errors.New("commit failed")
	tx.EXPECT().WithinTransaction(ctx, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			require.NoError(t, fn(txCtx))
			return commitErr
		}).Once()
	message, created, err := NewMessagesService(messages, chats, tx, notifier).SendMessage(ctx, command)
	require.ErrorIs(t, err, commitErr)
	require.False(t, created)
	require.Zero(t, message)
}

func expectSendMessageTransaction(
	txManager *MockTXManager,
	outerCtx context.Context,
	txCtx context.Context,
) {
	txManager.EXPECT().
		WithinTransaction(outerCtx, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			return fn(txCtx)
		})
}

func expectMessageNotFound(
	repository *MockMessagesRepository,
	ctx context.Context,
	senderID uuid.UUID,
	clientMessageID uuid.UUID,
) {
	repository.EXPECT().
		GetMessageByClientID(ctx, senderID, clientMessageID).
		Return(domain.Message{}, domain.ErrNotFound).
		Once()
}

func expectLockedChat(
	repository *MockChatsRepository,
	ctx context.Context,
	chat domain.Chat,
) {
	repository.EXPECT().GetChatForUpdate(ctx, chat.ID).Return(chat, nil)
}

func newSendMessageTestDirect(
	t *testing.T,
	chatID, user1ID, user2ID uuid.UUID,
) domain.DirectChat {
	t.Helper()
	direct, err := domain.NewDirectChat(chatID, user1ID, user2ID, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	return direct
}

func activeDirectMessageState(direct domain.DirectChat) DirectMessageState {
	return DirectMessageState{
		Users: [2]AccountState{
			{UserID: direct.User1ID},
			{UserID: direct.User2ID},
		},
	}
}

func newSendMessageTestGroup(
	t *testing.T,
	chatID uuid.UUID,
) domain.GroupChat {
	t.Helper()
	createdAt := time.Now().Add(-time.Hour)
	group, err := domain.NewGroupChat(chatID, "Test group", createdAt)
	require.NoError(t, err)
	require.NoError(t, err)
	require.NoError(t, err)
	return group
}

func newSendMessageTestMessage(
	t *testing.T,
	senderID, chatID, clientMessageID uuid.UUID,
	content string,
) domain.Message {
	t.Helper()
	message, err := domain.NewMessage(
		uuid.New(),
		clientMessageID,
		chatID,
		senderID,
		content,
		time.Now().Add(-time.Minute),
	)
	require.NoError(t, err)
	return message
}
