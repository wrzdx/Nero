package chats_transport_http

import (
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

type ChatResponse struct {
	ID             uuid.UUID  `json:"id"`
	Type           string     `json:"type"`
	LastMessageID  *uuid.UUID `json:"last_message_id"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	CreatedAt      time.Time  `json:"created_at"`
}
type GroupResponse struct {
	ChatResponse
	Title string `json:"title"`
}

func chatResponseFromDomain(chat domain.Chat) ChatResponse {
	return ChatResponse{
		ID:             chat.ID,
		Type:           string(chat.Type),
		LastMessageID:  chat.LastMessageID,
		LastActivityAt: chat.LastActivityAt,
		CreatedAt:      chat.CreatedAt,
	}
}
