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

func (r *ChatsRepository) GetGroupParticipant(
	ctx context.Context,
	chatID, userID uuid.UUID,
) (domain.GroupParticipant, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT cp.chat_id, cp.user_id, cp.last_read_message_id, cp.joined_at, gp.role
	FROM chat_participants cp 
	JOIN group_participants gp ON cp.chat_id=gp.chat_id 
							  AND cp.user_id=gp.user_id
	WHERE cp.chat_id=$1
	  AND cp.user_id=$2;
	`
	var participant domain.ChatParticipant
	var role string
	err := db.QueryRow(ctx, query, chatID, userID).Scan(
		&participant.ChatID,
		&participant.UserID,
		&participant.LastReadMessageID,
		&participant.JoinedAt,
		&role,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.GroupParticipant{}, domain.ErrNotFound
		}
		return domain.GroupParticipant{}, fmt.Errorf(
			"get group participant from db: %w",
			err,
		)
	}
	groupParticipant, err := domain.NewGroupParticipant(
		participant.ChatID,
		participant.UserID,
		participant.LastReadMessageID,
		participant.JoinedAt,
		domain.GroupRole(role),
	)
	if err != nil {
		return domain.GroupParticipant{}, fmt.Errorf("init group participant from db: %w", err)
	}

	return groupParticipant, nil
}
