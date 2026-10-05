package chats_service

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s ChatsService) ListChats(
	ctx context.Context,
	requesterID uuid.UUID,
	query ListChatsQuery,
) (ChatPage, error) {
	if requesterID == uuid.Nil {
		return ChatPage{}, fmt.Errorf("nil requester id: %w", domain.ErrNotFound)
	}

	query = query.normalize()
	if err := query.validate(); err != nil {
		return ChatPage{}, fmt.Errorf("validate query: %w", err)
	}

	allChats, err := s.chatsRepo.ListChats(
		ctx,
		requesterID,
		query.Before,
		query.Limit+1,
	)

	if err != nil {
		return ChatPage{}, fmt.Errorf("list chats: %w", err)
	}
	if allChats == nil {
		allChats = make([]ChatItem, 0)
	}

	var page ChatPage
	hasMore := len(allChats) > query.Limit
	if hasMore {
		allChats = allChats[:query.Limit]
	}

	page.Chats = allChats

	if hasMore {
		last := page.Chats[len(page.Chats)-1]
		page.NextCursor = &ChatCursor{
			ChatID:         last.Chat.ID,
			LastActivityAt: last.Chat.LastActivityAt,
		}
	}
	return page, nil
}

type ListChatsQuery struct {
	Before *ChatCursor
	Limit  int
}

func (q ListChatsQuery) normalize() ListChatsQuery {
	if q.Limit == 0 {
		q.Limit = 50
	}
	return q
}

func (q ListChatsQuery) validate() error {
	fields := make(map[string]string)
	if q.Limit < 0 || q.Limit > 100 {
		fields["limit"] = "limit must be between 1 and 100"
	}

	if q.Before != nil {
		if q.Before.LastActivityAt.IsZero() {
			fields["last_activity_at"] = "last_activity_at of chat cursor cannot be zero value"
		}
		if q.Before.ChatID == uuid.Nil {
			fields["chat_id"] = "chat id of chat cursor cannot be nil"
		}
	}
	if len(fields) > 0 {
		return domain.DetailedError{
			Err:     fmt.Errorf("list chats: %w", ErrInvalidInput),
			Details: fields,
		}
	}

	return nil
}

type ChatItem struct {
	Chat              domain.Chat
	DirectPeer        *directPeer
	GroupInfo         *groupInfo
	LastMessage       *lastMessageItem
	LastReadMessageID *uuid.UUID
	UnreadCount       int
}

func (i ChatItem) Validate() error {
	if err := i.Chat.Validate(); err != nil {
		return fmt.Errorf("invalid chat: %w", ErrInvalidChatItem)
	}
	if i.LastMessage != nil && i.LastMessage.Message.ChatID != i.Chat.ID {
		return fmt.Errorf(
			"last message chat id not equal to chat id: %w",
			ErrInvalidChatItem,
		)
	}
	if i.DirectPeer == nil && i.GroupInfo == nil {
		return fmt.Errorf("mutex of direct peer and group info: %w", ErrInvalidChatItem)
	}
	if i.DirectPeer != nil && i.GroupInfo != nil {
		return fmt.Errorf("presense of both direct peer and group info: %w", ErrInvalidChatItem)
	}
	if i.Chat.LastMessageID != nil && i.LastMessage == nil ||
		i.Chat.LastMessageID == nil && i.LastMessage != nil {
		return fmt.Errorf(
			"mistmatch last message info and last message: %w",
			ErrInvalidChatItem,
		)
	}
	if i.Chat.LastMessageID != nil &&
		*i.Chat.LastMessageID != (i.LastMessage.Message.ID) {
		return fmt.Errorf("last message id mismatch: %w", ErrInvalidChatItem)
	}
	return nil
}

type groupInfo struct {
	Title string
}

func GroupInfoFromGroup(group domain.GroupChat) (groupInfo, error) {
	if err := group.Validate(); err != nil {
		return groupInfo{}, fmt.Errorf("validate group: %w", ErrInvalidChatItem)
	}
	return groupInfo{
		Title: group.Title,
	}, nil
}

type directPeer struct {
	ID        uuid.UUID
	DeletedAt *time.Time
	Username  string
	FirstName string
	LastName  *string
}

func PeerFromUser(user domain.User) (directPeer, error) {
	if err := user.Validate(); err != nil {
		return directPeer{}, fmt.Errorf("invalid user: %w", ErrInvalidChatItem)
	}
	return directPeer{
		ID:        user.ID,
		DeletedAt: user.DeletedAt,
		Username:  user.Profile.Username,
		FirstName: user.Profile.FirstName,
		LastName:  user.Profile.LastName,
	}, nil
}

type lastMessageItem struct {
	Message         domain.Message
	SenderFirstName string
}

func NewLastMessageItem(m domain.Message, profile domain.UserProfile) (lastMessageItem, error) {
	if err := m.Validate(); err != nil {
		return lastMessageItem{}, fmt.Errorf("invalid last message: %w", ErrInvalidChatItem)
	}
	if err := profile.Validate(); err != nil {
		return lastMessageItem{}, fmt.Errorf(
			"invalid last message sender profile: %w",
			ErrInvalidChatItem,
		)
	}

	return lastMessageItem{
		Message:         m,
		SenderFirstName: profile.FirstName,
	}, nil
}

type ChatPage struct {
	Chats      []ChatItem
	NextCursor *ChatCursor
}

type ChatCursor struct {
	ChatID         uuid.UUID
	LastActivityAt time.Time
}
