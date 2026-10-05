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

func (r *Repository) UpdateChatLastMsgID(
	ctx context.Context,
	chatID uuid.UUID,
	newLastMessageID *uuid.UUID,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	query := `
	UPDATE chats
	SET last_message_id=$1
	WHERE id=$2
	RETURNING id;
	`
	if err := db.QueryRow(ctx, query, newLastMessageID, chatID).Scan(&chatID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("update chat sql: %w", err)
	}
	return nil
}
