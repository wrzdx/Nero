package messages_service

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s *MessagesService) EditMessage(
	ctx context.Context,
	command UpdateMessageCommand,
) (domain.Message, error) {
	if err := command.Validate(); err != nil {
		return domain.Message{}, fmt.Errorf("validate update message command: %w", err)
	}
	if err := s.messagesRepo.CheckParticipant(
		ctx,
		command.ChatID,
		command.SenderID,
	); err != nil {
		return domain.Message{}, fmt.Errorf("check participant: %w", err)
	}
	existing, err := s.messagesRepo.GetMessage(
		ctx,
		command.MessageID,
	)
	if err != nil {
		return domain.Message{}, fmt.Errorf("get message: %w", err)
	}
	if command.ChatID != existing.ChatID || existing.SenderID != command.SenderID {
		return domain.Message{}, domain.ErrNotFound
	}
	updated, err := existing.Update(command.Content, time.Now())
	if err != nil {
		return domain.Message{}, fmt.Errorf("update existing message: %w", err)
	}
	if updated.Content == existing.Content {
		return existing, nil
	}

	if err := s.messagesRepo.UpdateMessage(ctx, updated); err != nil {
		return domain.Message{}, fmt.Errorf("update message in db: %w", err)
	}

	s.notifier.MessageEdited(ctx, updated)
	return updated, nil
}

type UpdateMessageCommand struct {
	SenderID  uuid.UUID
	ChatID    uuid.UUID
	MessageID uuid.UUID
	Content   string
}

func (c UpdateMessageCommand) Validate() error {
	details := make(map[string]string)
	if c.SenderID == uuid.Nil {
		details["sender_id"] = "sender id is nil"
	}
	if c.ChatID == uuid.Nil {
		details["chat_id"] = "chat id is nil"
	}
	if c.MessageID == uuid.Nil {
		details["message_id"] = "message_id is nil"
	}
	if len(details) > 0 {
		return domain.DetailedError{
			Err:     ErrInvalidInput,
			Details: details,
		}
	}
	return nil
}
