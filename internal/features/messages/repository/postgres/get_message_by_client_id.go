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

func (r *Repository) GetMessageByClientID(
	ctx context.Context,
	senderID, clientMessageID uuid.UUID,
) (domain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	var message domain.Message
	err := db.QueryRow(ctx, `
		SELECT id, client_message_id, chat_id, sender_id, content, created_at, updated_at
		FROM messages
		WHERE sender_id=$1 
		  AND client_message_id=$2;
	`, senderID, clientMessageID).Scan(
		&message.ID,
		&message.ClientMessageID,
		&message.ChatID,
		&message.SenderID,
		&message.Content,
		&message.CreatedAt,
		&message.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Message{}, domain.ErrNotFound
		}
		return domain.Message{}, fmt.Errorf("select message from db: %w", err)
	}

	if err := message.Validate(); err != nil {
		return domain.Message{}, fmt.Errorf("validate message from db: %w", err)
	}

	return message, nil
}
