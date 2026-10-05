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

func (r *Repository) GetMessage(
	ctx context.Context,
	id uuid.UUID,
) (domain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	query := `
	SELECT id, client_message_id, chat_id, sender_id, content, created_at, updated_at
	FROM messages
	WHERE id = $1;
	`

	var msg domain.Message
	if err := db.QueryRow(ctx, query, id).Scan(
		&msg.ID,
		&msg.ClientMessageID,
		&msg.ChatID,
		&msg.SenderID,
		&msg.Content,
		&msg.CreatedAt,
		&msg.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Message{}, domain.ErrNotFound
		}
		return domain.Message{}, fmt.Errorf("select msg from db: %w", err)
	}
	if err := msg.Validate(); err != nil {
		return domain.Message{}, fmt.Errorf(
			"validate message restored from database: %v",
			err,
		)
	}

	return msg, nil
}
