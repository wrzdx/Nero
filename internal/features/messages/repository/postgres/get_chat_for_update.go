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

func (r *Repository) GetChatForUpdate(
	ctx context.Context,
	id uuid.UUID,
) (domain.Chat, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT id, type, last_message_id, last_activity_at, created_at
	FROM chats
	WHERE id=$1
	FOR UPDATE;
	`

	var chat domain.Chat
	err := db.QueryRow(ctx, query, id).Scan(
		&chat.ID,
		&chat.Type,
		&chat.LastMessageID,
		&chat.LastActivityAt,
		&chat.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Chat{}, domain.ErrNotFound
		}
		return domain.Chat{}, fmt.Errorf("select chat for update: %w", err)
	}

	if err := chat.Validate(); err != nil {
		return domain.Chat{}, fmt.Errorf("validate chat from db: %w", err)
	}

	return chat, nil
}
