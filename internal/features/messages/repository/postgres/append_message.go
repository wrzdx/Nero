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

func (r *Repository) AppendMessage(
	ctx context.Context,
	message domain.Message,
) (result error) {
	if err := message.Validate(); err != nil {
		return fmt.Errorf("validate passed message: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	batch := &pgx.Batch{}
	batch.Queue(`
	    INSERT INTO messages 
		(id, client_message_id, chat_id, sender_id, content, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7);
	`,
		message.ID,
		message.ClientMessageID,
		message.ChatID,
		message.SenderID,
		message.Content,
		message.CreatedAt,
		message.UpdatedAt,
	)
	var chatID uuid.UUID
	batch.Queue(`
	    UPDATE chats 
		SET last_message_id=$1,
			last_activity_at=$2
		WHERE id=$3
		RETURNING id;
	`, message.ID, message.CreatedAt, message.ChatID)
	results := db.SendBatch(ctx, batch)
	defer func() {
		if err := results.Close(); err != nil && result == nil {
			result = fmt.Errorf("batch close: %w", err)
		}
	}()
	if _, err := results.Exec(); err != nil {
		if postgres.IsConstraintViolation(err, postgres.UniqueViolation, messagesUK) {
			return domain.ErrAlreadyExists
		}

		return fmt.Errorf("insert message in db: %w", err)
	}

	if err := results.QueryRow().Scan(&chatID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("update chats in db: %w", err)
	}

	return nil
}
