package auth_postgres_repository

import (
	"github.com/wrzdx/Nero/internal/core/postgres"
	"time"
)

type SessionsRepository struct {
	db      postgres.DBTX
	timeout time.Duration
}

func NewSessionsRepository(db postgres.DBTX, timeout time.Duration) *SessionsRepository {
	return &SessionsRepository{
		db:      db,
		timeout: timeout,
	}
}

const (
	sessionsUsersFK          = "sessions_user_id_fkey"
	sessionsPK               = "sessions_pkey"
	sessionsCurrentTokenIDUK = "sessions_current_token_id_key"
)
