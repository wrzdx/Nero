package messages_postgres_repository

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
)

func (r *Repository) UpdateLastReadMessages(
	ctx context.Context,
	chatID, oldMessageID uuid.UUID,
	newMessageID *uuid.UUID,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	query := `
	UPDATE chat_participants
	SET last_read_message_id=$1
	WHERE chat_id=$2
	  AND last_read_message_id=$3;
	`
	if _, err := db.Exec(ctx, query, newMessageID, chatID, oldMessageID); err != nil {
		return fmt.Errorf("update chat participants sql: %w", err)
	}

	return nil
}
