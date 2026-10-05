package chats_postgres_repository

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	"time"

	"github.com/google/uuid"
)

func (r *ChatsRepository) ListGroupParticipants(
	ctx context.Context,
	chatID uuid.UUID,
	before *chats_service.GroupParticipantCursor,
	limit int,
) ([]chats_service.ParticipantInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT cp.chat_id, cp.user_id, cp.last_read_message_id, cp.joined_at, gp.role,
		   p.username, p.first_name, p.last_name, p.bio
	FROM chat_participants cp 
	JOIN group_participants gp ON cp.chat_id=gp.chat_id 
							  AND cp.user_id=gp.user_id
	JOIN users p ON cp.user_id=p.id
	WHERE cp.chat_id=$1
      AND (
		$2::timestamptz IS NULL
		OR (cp.joined_at, cp.user_id) < ($2, $3)
	  )
	ORDER BY cp.joined_at DESC, cp.user_id DESC
	LIMIT $4;
	`

	var beforeJoinedAt *time.Time
	var beforeParticipantID *uuid.UUID
	if before != nil {
		joinedAt := before.JoinedAt
		participantID := before.ParticipantID
		beforeJoinedAt = &joinedAt
		beforeParticipantID = &participantID
	}

	rows, err := db.Query(ctx, query, chatID, beforeJoinedAt, beforeParticipantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list group participants: %w", err)
	}
	defer rows.Close()

	var participants []chats_service.ParticipantInfo
	for rows.Next() {
		var participant domain.ChatParticipant
		var role domain.GroupRole
		var profile domain.UserProfile
		if err := rows.Scan(
			&participant.ChatID,
			&participant.UserID,
			&participant.LastReadMessageID,
			&participant.JoinedAt,
			&role,
			&profile.Username,
			&profile.FirstName,
			&profile.LastName,
			&profile.Bio,
		); err != nil {
			return nil, fmt.Errorf("scan participant info: %w", err)
		}

		participantInfo, err := chats_service.NewParticipantInfo(
			participant,
			role,
			profile,
		)
		if err != nil {
			return nil, err
		}

		participants = append(participants, participantInfo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group participants list: %w", err)
	}

	return participants, nil
}
