package chats_postgres_repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *ChatsRepository) GetDirectByUsers(
	ctx context.Context,
	user1ID uuid.UUID,
	user2ID uuid.UUID,
) (domain.DirectChat, error) {
	if bytes.Compare(user1ID[:], user2ID[:]) > 0 {
		user1ID, user2ID = user2ID, user1ID
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT c.id, c.type, c.last_message_id, c.last_activity_at, c.created_at, d.user1_id, d.user2_id
	FROM chats c
	JOIN directs d ON d.chat_id=c.id
	WHERE d.user1_id=$1
	  AND d.user2_id=$2;
	`

	var direct domain.DirectChat
	err := db.QueryRow(ctx, query, user1ID, user2ID).Scan(
		&direct.Chat.ID,
		&direct.Chat.Type,
		&direct.Chat.LastMessageID,
		&direct.Chat.LastActivityAt,
		&direct.Chat.CreatedAt,
		&direct.User1ID,
		&direct.User2ID,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DirectChat{}, domain.ErrNotFound
		}
		return domain.DirectChat{}, fmt.Errorf("select chat from db: %w", err)
	}
	if err := direct.Validate(); err != nil {
		return domain.DirectChat{}, fmt.Errorf("validate direct from db: %w", err)
	}

	return direct, nil
}
