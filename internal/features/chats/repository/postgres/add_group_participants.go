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

func (r *ChatsRepository) AddGroupParticipants(
	ctx context.Context,
	groupID uuid.UUID,
	participants []domain.GroupParticipant,
) (result []bool, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	batch := &pgx.Batch{}

	insertParticipantsQuery := `
	INSERT INTO chat_participants (chat_id, user_id, last_read_message_id, joined_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT DO NOTHING;
	`
	insertGroupParticipantsQuery := `
	INSERT INTO group_participants (chat_id, user_id, role)
	VALUES ($1, $2, $3)
	ON CONFLICT DO NOTHING
	RETURNING user_id;
	`
	for _, p := range participants {
		if p.ChatID != groupID {
			return nil, errors.New("participant ids and chat ids mismatch")
		}
		batch.Queue(
			insertParticipantsQuery,
			groupID,
			p.UserID,
			p.LastReadMessageID,
			p.JoinedAt,
		)
		batch.Queue(
			insertGroupParticipantsQuery,
			groupID,
			p.UserID,
			p.Role(),
		)
	}
	results := db.SendBatch(ctx, batch)
	defer func() {
		if err := results.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("send batch to add group participants: %w", err)
		}
	}()

	for range participants {
		var userID uuid.UUID

		if _, err := results.Exec(); err != nil {
			return nil, fmt.Errorf("insert chat participant in db: %w", err)
		}
		errGroupParticipant := results.QueryRow().Scan(&userID)
		if errGroupParticipant != nil {
			result = append(result, false)
			if errors.Is(errGroupParticipant, pgx.ErrNoRows) {
				continue
			}
			return nil, fmt.Errorf("insert group participant in db: %w", errGroupParticipant)
		}

		result = append(result, true)
	}

	return result, nil
}
