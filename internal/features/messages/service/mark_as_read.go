package messages_service

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
)

func (s *MessagesService) MarkAsRead(
	ctx context.Context,
	command MarkAsReadCommand,
) error {
	if err := command.validate(); err != nil {
		return fmt.Errorf("validate mark as read command: %w", err)
	}
	if err := s.messagesRepo.CheckParticipant(
		ctx,
		command.ChatID,
		command.UserID,
	); err != nil {
		return fmt.Errorf("check participant: %w", err)
	}
	if err := s.txmanager.WithinTransaction(ctx, func(ctx context.Context) error {
		if _, err := s.chatsRepo.GetChatForUpdate(ctx, command.ChatID); err != nil {
			return fmt.Errorf("repo: get chat for update: %w", err)
		}
		if err := s.messagesRepo.MarkAsRead(
			ctx,
			command.ChatID,
			command.UserID,
			command.MessageID,
		); err != nil {
			return fmt.Errorf("repo: mark as read: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("transaction: %w", err)
	}

	return nil
}

type MarkAsReadCommand struct {
	ChatID    uuid.UUID
	UserID    uuid.UUID
	MessageID uuid.UUID
}

func (c MarkAsReadCommand) validate() error {
	details := make(map[string]string)
	if c.ChatID == uuid.Nil {
		details["chat_id"] = "chat id is nil"
	}
	if c.UserID == uuid.Nil {
		details["user_id"] = "user id is nil"
	}
	if c.MessageID == uuid.Nil {
		details["message_id"] = "message id is nil"
	}

	if len(details) > 0 {
		return domain.DetailedError{
			Err:     ErrInvalidInput,
			Details: details,
		}
	}

	return nil
}
