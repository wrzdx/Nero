package messages_service

import (
	"context"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
)

type MessagesRepository interface {
	GetMessageByClientID(
		ctx context.Context,
		senderID, clientMessageID uuid.UUID,
	) (domain.Message, error)

	AppendMessage(
		ctx context.Context,
		message domain.Message,
	) error

	CheckParticipant(
		ctx context.Context,
		chatID, participantID uuid.UUID,
	) error

	GetMessages(
		ctx context.Context,
		chatID uuid.UUID,
		before *MessageCursor,
		limit int,
		after bool,
	) ([]domain.Message, error)

	GetMessage(
		ctx context.Context,
		id uuid.UUID,
	) (domain.Message, error)

	UpdateMessage(
		ctx context.Context,
		updated domain.Message,
	) error

	DeleteMessage(
		ctx context.Context,
		id uuid.UUID,
	) error

	MarkAsRead(
		ctx context.Context,
		chatID, userID, messageID uuid.UUID,
	) error
}

type ChatsRepository interface {
	GetChatForUpdate(
		ctx context.Context,
		chatID uuid.UUID,
	) (domain.Chat, error)

	GetDirectMessageState(
		ctx context.Context,
		chatID uuid.UUID,
	) (DirectMessageState, error)

	GetGroupSenderState(
		ctx context.Context,
		chatID, senderID uuid.UUID,
	) (AccountState, error)

	UpdateChatLastMsgID(
		ctx context.Context,
		chatID uuid.UUID,
		newLastMessageID *uuid.UUID,
	) error

	UpdateLastReadMessages(
		ctx context.Context,
		chatID, oldMessageID uuid.UUID,
		newMessageID *uuid.UUID,
	) error
}

type TXManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Notifier handles notification failures internally. Call only after commit.
type Notifier interface {
	MessageCreated(
		ctx context.Context,
		message domain.Message,
	)
	MessageEdited(ctx context.Context, message domain.Message)
	MessageDeleted(ctx context.Context, chatID, messageID uuid.UUID)
}
