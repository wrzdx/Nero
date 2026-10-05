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

func (r *Repository) GetDirectMessageState(
	ctx context.Context,
	chatID uuid.UUID,
) (messages_service.DirectMessageState, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT u1.id, u1.deleted_at IS NOT NULL, u2.id, u2.deleted_at IS NOT NULL
	FROM directs d
	JOIN users u1 ON u1.id=d.user1_id
	JOIN users u2 ON u2.id=d.user2_id 
	WHERE d.chat_id=$1;
	`
	var state messages_service.DirectMessageState
	err := db.QueryRow(ctx, query, chatID).Scan(
		&state.Users[0].UserID,
		&state.Users[0].Deleted,
		&state.Users[1].UserID,
		&state.Users[1].Deleted,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return messages_service.DirectMessageState{}, domain.ErrNotFound
		}
		return messages_service.DirectMessageState{}, fmt.Errorf(
			"select direct message state: %w",
			err,
		)
	}

	return state, nil
}
