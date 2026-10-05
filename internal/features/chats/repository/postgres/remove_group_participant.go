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

func (r *ChatsRepository) RemoveGroupParticipant(
	ctx context.Context,
	participant domain.GroupParticipant,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	deleteGroupParticipantQuery := `--sql
	DELETE FROM group_participants
	WHERE chat_id=$1
	  AND user_id=$2
	RETURNING user_id;
	`

	deleteChatParticipantQuery := `--sql
	DELETE FROM chat_participants
	WHERE chat_id=$1
	  AND user_id=$2;
	`

	var userID uuid.UUID
	if err := db.QueryRow(
		ctx,
		deleteGroupParticipantQuery,
		participant.ChatID,
		participant.UserID,
	).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("delete group participant db query: %w", err)
	}
	if _, err := db.Exec(
		ctx,
		deleteChatParticipantQuery,
		participant.ChatID,
		participant.UserID,
	); err != nil {
		return fmt.Errorf("delete chat participant db query: %w", err)
	}

	return nil
}
