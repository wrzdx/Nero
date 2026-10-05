package auth_postgres_repository

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
)

func (r *SessionsRepository) DeleteAllSessions(
	ctx context.Context,
	userID uuid.UUID,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	db := postgres.GetExecutor(ctx, r.db)

	_, err := db.Exec(ctx, `--sql
	DELETE FROM sessions
	WHERE user_id=$1;
	`, userID)

	if err != nil {
		return fmt.Errorf("delete all sessions: %w", err)
	}

	return nil
}
