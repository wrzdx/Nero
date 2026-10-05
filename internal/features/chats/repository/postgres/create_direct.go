package chats_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/jackc/pgx/v5"
)

func (r *ChatsRepository) CreateDirect(
	ctx context.Context,
	direct domain.DirectChat,
	participant1 domain.ChatParticipant,
	participant2 domain.ChatParticipant,
) error {
	if participant2.ChatID != direct.Chat.ID || participant1.ChatID != direct.Chat.ID {
		return errors.New("chat ids do not match")
	}
	if participant1.UserID == participant2.UserID {
		return errors.New("participant ids must be different")
	}
	if (participant1.UserID != direct.User1ID && participant1.UserID != direct.User2ID) ||
		(participant2.UserID != direct.User1ID && participant2.UserID != direct.User2ID) {
		return errors.New("participant ids do not match to direct chat user ids")
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	db := postgres.GetExecutor(ctx, r.db)
	batch := &pgx.Batch{}

	insertChatQuery := `
	INSERT INTO chats (id, type, last_message_id, last_activity_at, created_at)
	VALUES ($1, $2, $3, $4, $5);
	`
	batch.Queue(insertChatQuery,
		direct.Chat.ID,
		direct.Chat.Type,
		direct.Chat.LastMessageID,
		direct.Chat.LastActivityAt,
		direct.Chat.CreatedAt,
	)
	insertDirectQuery := `
	INSERT INTO directs (chat_id, user1_id, user2_id)
	VALUES ($1, $2, $3);
	`

	batch.Queue(
		insertDirectQuery,
		direct.Chat.ID,
		direct.User1ID,
		direct.User2ID,
	)
	insertParticipantsQuery := `
	INSERT INTO chat_participants (chat_id, user_id, last_read_message_id, joined_at)
	VALUES ($1, $2, $3, $4),
		   ($1, $5, $6, $7);
	`
	batch.Queue(
		insertParticipantsQuery,
		direct.Chat.ID,
		participant1.UserID,
		participant1.LastReadMessageID,
		participant1.JoinedAt,
		participant2.UserID,
		participant2.LastReadMessageID,
		participant2.JoinedAt,
	)

	if err := db.SendBatch(ctx, batch).Close(); err != nil {
		if postgres.IsConstraintViolation(err, postgres.UniqueViolation, directsUK) {
			return domain.ErrAlreadyExists
		}
		return fmt.Errorf("create direct aggregate: %w", err)
	}

	return nil
}
