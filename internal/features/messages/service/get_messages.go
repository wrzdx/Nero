package messages_service

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s *MessagesService) GetMessages(
	ctx context.Context,
	requesterID uuid.UUID,
	query GetMessagesQuery,
) (MessagePage, error) {
	if requesterID == uuid.Nil {
		return MessagePage{}, fmt.Errorf("nil requester id: %w", domain.ErrNotFound)
	}

	query = query.normalize()
	if err := query.validate(); err != nil {
		return MessagePage{}, fmt.Errorf("validate query: %w", err)
	}

	if err := s.messagesRepo.CheckParticipant(ctx, query.ChatID, requesterID); err != nil {
		return MessagePage{}, fmt.Errorf("check participant: %w", err)
	}

	allMessages, err := s.messagesRepo.GetMessages(
		ctx,
		query.ChatID,
		query.Cursor,
		query.Limit+1,
		query.After,
	)

	if err != nil {
		return MessagePage{}, fmt.Errorf("get messages: %w", err)
	}
	if allMessages == nil {
		allMessages = make([]domain.Message, 0)
	}

	var page MessagePage
	hasMore := len(allMessages) > query.Limit
	if hasMore {
		allMessages = allMessages[:query.Limit]
	}

	page.Messages = allMessages

	if hasMore || (query.After && len(page.Messages) != 0) {
		last := page.Messages[len(page.Messages)-1]
		page.NextCursor = &MessageCursor{
			MessageID: last.ID,
			CreatedAt: last.CreatedAt,
		}
	} else if query.After {
		page.NextCursor = query.Cursor
	}
	return page, nil
}

type GetMessagesQuery struct {
	ChatID uuid.UUID
	Cursor *MessageCursor
	Limit  int
	After  bool
}

func (q GetMessagesQuery) normalize() GetMessagesQuery {
	if q.Limit == 0 {
		q.Limit = 50
	}
	return q
}

func (q GetMessagesQuery) validate() error {
	fields := make(map[string]string)
	if q.ChatID == uuid.Nil {
		fields["chat_id"] = "chat_id is nil"
	}
	if q.Limit < 0 || q.Limit > 100 {
		fields["limit"] = "limit must be between 1 and 100"
	}

	if q.Cursor != nil {
		if q.Cursor.CreatedAt.IsZero() {
			fields["created_at"] = "created_at of message cursor cannot be zero value"
		}
		if q.Cursor.MessageID == uuid.Nil {
			fields["message_id"] = "message id of message cursor cannot be nil"
		}
	} else if q.After {
		fields["after"] = "missing cursor for after"
	}
	if len(fields) > 0 {
		return domain.DetailedError{
			Err:     ErrInvalidInput,
			Details: fields,
		}
	}

	return nil
}

type MessagePage struct {
	Messages   []domain.Message
	NextCursor *MessageCursor
}

type MessageCursor struct {
	MessageID uuid.UUID
	CreatedAt time.Time
}
