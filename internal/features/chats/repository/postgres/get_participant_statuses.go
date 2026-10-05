package chats_postgres_repository

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/postgres"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"

	"github.com/google/uuid"
)

func (r *ChatsRepository) GetParticipantsStatus(
	ctx context.Context,
	userIDs []uuid.UUID,
) ([]chats_service.ParticipantStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT requested.user_id,
       users.id IS NOT NULL AS found
	FROM UNNEST($1::uuid[]) AS requested(user_id)
	LEFT JOIN users
		ON users.id = requested.user_id
	AND users.deleted_at IS NULL;
	`

	rows, err := db.Query(ctx, query, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get participant statuses from db: %w", err)
	}
	defer rows.Close()

	var statuses []chats_service.ParticipantStatus
	for rows.Next() {
		var status chats_service.ParticipantStatus
		if err := rows.Scan(&status.UserID, &status.Found); err != nil {
			return nil, fmt.Errorf("scan participant status: %w", err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get participant statuses: %w", err)
	}

	return statuses, nil
}
