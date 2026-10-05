package messages_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) MarkAsRead(
	ctx context.Context,
	chatID, userID, messageID uuid.UUID,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	query := `
	WITH target AS (
		SELECT id, created_at
		FROM messages
		WHERE id = $1
		AND chat_id = $2
	),
	participant AS (
		SELECT
			chat_id,
			user_id,
			last_read_message_id
		FROM chat_participants
		WHERE chat_id = $2
		  AND user_id = $3
		FOR UPDATE
	),
	candidate AS (
		SELECT CASE
			WHEN current.id IS NULL
			OR (target.created_at, target.id)
				> (current.created_at, current.id)
			THEN target.id
			ELSE current.id
		END AS id
		FROM participant
		CROSS JOIN target
		LEFT JOIN messages current
			ON current.id = participant.last_read_message_id
		AND current.chat_id = participant.chat_id
	)
	UPDATE chat_participants participant
	SET last_read_message_id = candidate.id
	FROM candidate
	WHERE participant.chat_id = $2
	  AND participant.user_id = $3
	RETURNING participant.last_read_message_id;
	`

	var lastReadMessageID uuid.UUID
	if err := db.QueryRow(
		ctx,
		query,
		messageID,
		chatID,
		userID,
	).Scan(&lastReadMessageID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("mark as read sql: %w", err)
	}

	return nil
}
