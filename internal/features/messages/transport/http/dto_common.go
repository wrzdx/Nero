package messages_transport_http

import (
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

type MessageResponse struct {
	ID              uuid.UUID  `json:"id"`
	ClientMessageID uuid.UUID  `json:"client_message_id"`
	ChatID          uuid.UUID  `json:"chat_id"`
	SenderID        uuid.UUID  `json:"sender_id"`
	Content         string     `json:"content"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

func messageResponseFromDomain(message domain.Message) MessageResponse {
	return MessageResponse{
		ID:              message.ID,
		ClientMessageID: message.ClientMessageID,
		ChatID:          message.ChatID,
		SenderID:        message.SenderID,
		Content:         message.Content,
		CreatedAt:       message.CreatedAt,
		UpdatedAt:       message.UpdatedAt,
	}
}

func messageResponsesFromDomain(message []domain.Message) []MessageResponse {
	result := make([]MessageResponse, len(message))
	for i, m := range message {
		result[i] = messageResponseFromDomain(m)
	}
	return result
}
