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

func (r *ChatsRepository) GetGroup(
	ctx context.Context,
	groupID uuid.UUID,
) (domain.GroupChat, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT c.id, c.type, c.last_message_id, c.last_activity_at, c.created_at, g.title
	FROM groups g
	JOIN chats c 
	  ON g.chat_id=c.id
	WHERE g.chat_id=$1;
	`

	var group domain.GroupChat
	if err := db.QueryRow(ctx, query, groupID).Scan(
		&group.Chat.ID,
		&group.Chat.Type,
		&group.Chat.LastMessageID,
		&group.Chat.LastActivityAt,
		&group.Chat.CreatedAt,
		&group.Title,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.GroupChat{}, domain.ErrNotFound
		}
		return domain.GroupChat{}, fmt.Errorf("select group from db: %w", err)
	}

	if err := group.Validate(); err != nil {
		return domain.GroupChat{}, fmt.Errorf("group from db: %w", err)
	}

	return group, nil
}
