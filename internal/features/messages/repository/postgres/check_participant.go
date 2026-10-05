package messages_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CheckParticipant(
	ctx context.Context,
	chatID, participantID uuid.UUID,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT id
	FROM chat_participants p
	JOIN users u ON u.id=p.user_id
	WHERE p.chat_id=$1
	  AND p.user_id=$2
	  AND u.deleted_at IS NULL;
	`
	var id uuid.UUID

	err := db.QueryRow(ctx, query, chatID, participantID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("check participant: %w", err)
	}

	return nil
}
