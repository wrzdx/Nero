package auth_postgres_repository

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"
)

func (r *SessionsRepository) CreateSession(
	ctx context.Context,
	session domain.Session,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	db := postgres.GetExecutor(ctx, r.db)

	_, err := db.Exec(ctx, `
		INSERT INTO sessions (id, user_id, current_token_id, last_used_at, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6);
	`, session.ID, session.UserID, session.CurrentTokenID, session.LastUsedAt, session.CreatedAt, session.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}
