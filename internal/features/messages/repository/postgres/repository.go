package messages_postgres_repository

import (
	"github.com/wrzdx/Nero/internal/core/postgres"
	"time"
)

type Repository struct {
	db      postgres.DBTX
	timeout time.Duration
}

func NewRepository(db postgres.DBTX, timeout time.Duration) *Repository {
	return &Repository{
		db:      db,
		timeout: timeout,
	}
}

const (
	messagesUK = "messages_sender_client_message_unique"
)
