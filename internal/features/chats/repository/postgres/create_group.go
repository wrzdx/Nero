package chats_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/jackc/pgx/v5"
)

func (r *ChatsRepository) CreateGroup(
	ctx context.Context,
	group domain.GroupChat,
	participants []domain.GroupParticipant,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	batch := &pgx.Batch{}

	insertChatQuery := `
	INSERT INTO chats (id, type, last_message_id, last_activity_at, created_at)
	VALUES ($1, $2, $3, $4, $5);
	`
	batch.Queue(insertChatQuery,
		group.Chat.ID,
		group.Chat.Type,
		group.Chat.LastMessageID,
		group.Chat.LastActivityAt,
		group.Chat.CreatedAt,
	)
	insertGroupQuery := `
	INSERT INTO groups (chat_id, title)
	VALUES ($1, $2);
	`

	batch.Queue(
		insertGroupQuery,
		group.Chat.ID,
		group.Title,
	)
	insertParticipantsQuery := `
	INSERT INTO chat_participants (chat_id, user_id, last_read_message_id, joined_at)
	VALUES ($1, $2, $3, $4);
	`
	insertGroupParticipantsQuery := `
	INSERT INTO group_participants (chat_id, user_id, role)
	VALUES ($1, $2, $3);
	`
	for _, p := range participants {
		if p.ChatID != group.Chat.ID {
			return errors.New("participant ids and chat ids mismatch")
		}
		batch.Queue(
			insertParticipantsQuery,
			group.Chat.ID,
			p.UserID,
			p.LastReadMessageID,
			p.JoinedAt,
		)
		batch.Queue(
			insertGroupParticipantsQuery,
			group.Chat.ID,
			p.UserID,
			p.Role(),
		)
	}

	if err := db.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("create group aggregate: %w", err)
	}

	return nil
}
