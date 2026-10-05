package messages_transport_http

import (
	"context"
	"github.com/wrzdx/Nero/internal/core/domain"
	messages_service "github.com/wrzdx/Nero/internal/features/messages/service"

	"github.com/google/uuid"
)

type MessagesService interface {
	GetMessages(
		ctx context.Context,
		requesterID uuid.UUID,
		query messages_service.GetMessagesQuery,
	) (messages_service.MessagePage, error)

	SendMessage(
		ctx context.Context,
		command messages_service.SendMessageCommand,
	) (domain.Message, bool, error)

	EditMessage(
		ctx context.Context,
		command messages_service.UpdateMessageCommand,
	) (domain.Message, error)

	DeleteMessage(
		ctx context.Context,
		command messages_service.DeleteMessageCommand,
	) error

	MarkAsRead(
		ctx context.Context,
		command messages_service.MarkAsReadCommand,
	) error
}
