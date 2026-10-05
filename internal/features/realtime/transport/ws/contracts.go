package realtime_transport_ws

import (
	"context"
	"github.com/google/uuid"
	"github.com/wrzdx/Nero/internal/core/auth"
)

type ParticipantsRepository interface {
	GetParticipants(ctx context.Context, chatID uuid.UUID) ([]uuid.UUID, error)
}

type TokenProvider interface {
	ParseAccessToken(tokenStr string) (auth.ParsedAccessToken, error)
}
