package messages_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"
	messages_service "github.com/wrzdx/Nero/internal/features/messages/service"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r Repository) GetGroupSenderState(
	ctx context.Context,
	chatID, senderID uuid.UUID,
) (messages_service.AccountState, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT u.id, u.deleted_at IS NOT NULL
	FROM group_participants g
	JOIN users u ON u.id=g.user_id
	WHERE g.chat_id=$1
	  AND g.user_id=$2;
	`
	var state messages_service.AccountState
	err := db.QueryRow(ctx, query, chatID, senderID).Scan(
		&state.UserID,
		&state.Deleted,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return messages_service.AccountState{}, domain.ErrNotFound
		}
		return messages_service.AccountState{}, fmt.Errorf(
			"select group sender state: %w",
			err,
		)
	}

	return state, nil
}
