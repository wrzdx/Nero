package messages_postgres_repository

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
)

func (r *Repository) GetParticipants(
	ctx context.Context,
	chatID uuid.UUID,
) ([]uuid.UUID, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	rows, err := db.Query(ctx, `
	SELECT user_id
	FROM chat_participants
	WHERE chat_id=$1;
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("get chat participant ids: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(
			&id,
		); err != nil {
			return nil, fmt.Errorf("scan participant id: %w", err)
		}

		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat participant ids: %w", err)
	}

	return ids, nil
}
