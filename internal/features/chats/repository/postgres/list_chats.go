package chats_postgres_repository

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *ChatsRepository) ListChats(
	ctx context.Context,
	userID uuid.UUID,
	before *chats_service.ChatCursor,
	limit int,
) (chatItems []chats_service.ChatItem, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	chats, lastReadMessageIDs, err := r.getUserChats(ctx, userID, before, limit)
	if err != nil {
		return nil, err
	}

	db := postgres.GetExecutor(ctx, r.db)

	batch := &pgx.Batch{}

	for i, chat := range chats {
		switch chat.Type {
		case domain.ChatTypeDirect:
			batch.Queue(`
			SELECT id, username, first_name, last_name, created_at, deleted_at, bio, password_hash
			FROM users
			JOIN directs ON directs.chat_id=$1
			WHERE id=(
				CASE
					WHEN directs.user1_id=$2 THEN directs.user2_id
					ELSE directs.user1_id
				END
			);
			`, chat.ID, userID)
		case domain.ChatTypeGroup:
			batch.Queue(`
			SELECT title
			FROM groups
			WHERE chat_id=$1
			`, chat.ID)
		default:
			return nil, fmt.Errorf("invalid chat type: %w", domain.ErrInvalidChat)
		}

		if chat.LastMessageID != nil {
			batch.Queue(`
			SELECT m.id, m.client_message_id, m.chat_id, m.sender_id, m.content, m.created_at, m.updated_at,
				   s.username, s.first_name, s.last_name, s.bio
			FROM messages m
			JOIN users s ON m.sender_id = s.id
			WHERE m.id=$1
			`, chat.LastMessageID)
			batch.Queue(`
			SELECT count(*) 
			FROM messages m
			WHERE m.chat_id = $2
			AND (
				$1::uuid IS NULL
				OR (m.created_at, m.id) > (
					SELECT created_at, id
					FROM messages
					WHERE id = $1
				)
			)
			`, lastReadMessageIDs[i], chat.ID)
		}
	}
	chatItems = make([]chats_service.ChatItem, 0, len(chats))

	result := db.SendBatch(ctx, batch)
	defer func() {
		if err := result.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("list chats aggregate: %w", err)
		}
	}()

	for i, chat := range chats {
		chatItem := chats_service.ChatItem{Chat: chat}
		switch chat.Type {
		case domain.ChatTypeDirect:
			var user domain.User
			if err := result.QueryRow().Scan(
				&user.ID,
				&user.Profile.Username,
				&user.Profile.FirstName,
				&user.Profile.LastName,
				&user.CreatedAt,
				&user.DeletedAt,
				&user.Profile.Bio,
				&user.PasswordHash,
			); err != nil {
				return nil, fmt.Errorf("scan user: %w", err)
			}
			peer, err := chats_service.PeerFromUser(user)
			if err != nil {
				return nil, err
			}
			chatItem.DirectPeer = &peer
		case domain.ChatTypeGroup:
			group := domain.GroupChat{
				Chat: chat,
			}
			if err := result.QueryRow().Scan(&group.Title); err != nil {
				return nil, fmt.Errorf("scan group: %w", err)
			}
			groupInfo, err := chats_service.GroupInfoFromGroup(group)
			if err != nil {
				return nil, err
			}
			chatItem.GroupInfo = &groupInfo
		default:
			return nil, fmt.Errorf("invalid chat type: %w", domain.ErrInvalidChat)
		}

		if chat.LastMessageID != nil {
			var (
				lastMessage   domain.Message
				senderProfile domain.UserProfile
			)

			if err := result.QueryRow().Scan(
				&lastMessage.ID,
				&lastMessage.ClientMessageID,
				&lastMessage.ChatID,
				&lastMessage.SenderID,
				&lastMessage.Content,
				&lastMessage.CreatedAt,
				&lastMessage.UpdatedAt,
				&senderProfile.Username,
				&senderProfile.FirstName,
				&senderProfile.LastName,
				&senderProfile.Bio,
			); err != nil {
				return nil, fmt.Errorf("last message and sender: %w", err)
			}

			lm, err := chats_service.NewLastMessageItem(lastMessage, senderProfile)
			if err != nil {
				return nil, err
			}
			chatItem.LastMessage = &lm

			chatItem.LastReadMessageID = lastReadMessageIDs[i]

			if err := result.QueryRow().Scan(
				&chatItem.UnreadCount,
			); err != nil {
				return nil, fmt.Errorf("unread count: %w", err)
			}
		}
		if err := chatItem.Validate(); err != nil {
			return nil, err
		}
		chatItems = append(chatItems, chatItem)
	}

	return chatItems, nil
}

func (r *ChatsRepository) getUserChats(
	ctx context.Context,
	userID uuid.UUID,
	before *chats_service.ChatCursor,
	limit int,
) ([]domain.Chat, []*uuid.UUID, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT c.id, c.type, c.last_message_id, c.last_activity_at, c.created_at, cp.last_read_message_id
	FROM chat_participants cp
	JOIN chats c ON c.id = cp.chat_id
	WHERE cp.user_id = $1
		AND (
		$2::timestamptz IS NULL
		OR (c.last_activity_at, c.id) < ($2, $3)
		)
	ORDER BY c.last_activity_at DESC, c.id DESC
	LIMIT $4;
	`

	var beforeActivity *time.Time
	var beforeChatID *uuid.UUID
	if before != nil {
		activity := before.LastActivityAt
		chatID := before.ChatID
		beforeActivity = &activity
		beforeChatID = &chatID
	}

	rows, err := db.Query(ctx, query, userID, beforeActivity, beforeChatID, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()

	var chats []domain.Chat
	var lastReadMessageIDs []*uuid.UUID
	for rows.Next() {
		var chat domain.Chat
		var lastReadMessageID *uuid.UUID
		if err := rows.Scan(
			&chat.ID,
			&chat.Type,
			&chat.LastMessageID,
			&chat.LastActivityAt,
			&chat.CreatedAt,
			&lastReadMessageID,
		); err != nil {
			return nil, nil, fmt.Errorf("scan chat: %w", err)
		}

		chats = append(chats, chat)
		lastReadMessageIDs = append(lastReadMessageIDs, lastReadMessageID)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate chat list: %w", err)
	}

	return chats, lastReadMessageIDs, nil
}
