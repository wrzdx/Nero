package realtime_transport_ws

import (
	"context"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Notifier struct {
	hub          *Hub
	participants ParticipantsRepository
	log          *logger.Logger
}

func NewNotifier(hub *Hub, participants ParticipantsRepository, log *logger.Logger) *Notifier {
	return &Notifier{
		hub:          hub,
		participants: participants,
		log:          log,
	}
}

// MessageCreated makes one best-effort notification attempt after commit.
// Failures are logged here and do not change the result of saving the message.
func (p *Notifier) MessageCreated(
	ctx context.Context,
	message domain.Message,
) {
	p.notify(ctx, message.ChatID, message.ID, Event{Type: msgCreated, Data: messageDTO(message)})
}

func (p *Notifier) MessageEdited(ctx context.Context, message domain.Message) {
	p.notify(ctx, message.ChatID, message.ID, Event{Type: msgEdited, Data: messageDTO(message)})
}

func (p *Notifier) MessageDeleted(ctx context.Context, chatID, messageID uuid.UUID) {
	p.notify(ctx, chatID, messageID, Event{Type: msgDeleted, Data: DeletedMessage{ID: messageID, ChatID: chatID}})
}

func (p *Notifier) notify(ctx context.Context, chatID, messageID uuid.UUID, event Event) {
	log := p.log.With(zap.String("chat_id", chatID.String()), zap.String("message_id", messageID.String()), zap.String("event_type", event.Type))
	recipientIDs, err := p.participants.GetParticipants(ctx, chatID)
	if err != nil {
		log.Error("get message notification recipients", zap.Error(err))
		return
	}
	for _, id := range recipientIDs {
		if err := p.hub.Publish(id, event); err != nil {
			log.Error("publish message event", zap.Error(err), zap.String("recipient_id", id.String()))
		}
	}
}

func messageDTO(message domain.Message) Message {
	return Message{
		ID: message.ID, ChatID: message.ChatID, SenderID: message.SenderID,
		Content: message.Content, CreatedAt: message.CreatedAt, UpdatedAt: message.UpdatedAt,
	}
}
