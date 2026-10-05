package users_postgres_repository

import (
	"github.com/wrzdx/Nero/internal/core/postgres"
	"time"
)

type UsersRepository struct {
	db      postgres.DBTX
	timeout time.Duration
}

func NewUsersRepository(db postgres.DBTX, timeout time.Duration) *UsersRepository {
	return &UsersRepository{
		db:      db,
		timeout: timeout,
	}
}

var (
	usernameUK = "users_username_lower_uidx"
)
