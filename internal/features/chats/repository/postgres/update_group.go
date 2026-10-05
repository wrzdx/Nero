package chats_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *ChatsRepository) UpdateGroup(
	ctx context.Context,
	updated domain.GroupChat,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	UPDATE groups 
	SET title=$1
	WHERE chat_id=$2
	RETURNING chat_id;
	`
	var chatID uuid.UUID
	if err := db.QueryRow(ctx, query, updated.Title, updated.Chat.ID).Scan(&chatID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("update group sql: %w", err)
	}

	return nil
}
