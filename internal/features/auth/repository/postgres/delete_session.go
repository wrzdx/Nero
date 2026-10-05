package auth_postgres_repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *SessionsRepository) DeleteSession(
	ctx context.Context,
	sessionID, currentTokenID uuid.UUID,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	db := postgres.GetExecutor(ctx, r.db)
	var id uuid.UUID
	err := db.QueryRow(ctx, `--sql
		DELETE FROM sessions
		WHERE id=$1
			AND current_token_id=$2
		RETURNING id; 
	`, sessionID, currentTokenID,
	).Scan(&id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf(
				"matching session: %w",
				domain.ErrNotFound,
			)
		}
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
