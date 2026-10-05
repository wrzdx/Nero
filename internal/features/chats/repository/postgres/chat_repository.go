package chats_postgres_repository

import (
	"github.com/wrzdx/Nero/internal/core/postgres"
	"time"
)

type ChatsRepository struct {
	db      postgres.DBTX
	timeout time.Duration
}

func NewChatsRepository(db postgres.DBTX, timeout time.Duration) *ChatsRepository {
	return &ChatsRepository{
		db:      db,
		timeout: timeout,
	}
}

const (
	directsUK          = "directs_unique_pair"
	chatParticipantsPK = "chat_participants_pkey"
)
