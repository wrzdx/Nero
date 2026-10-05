package messages_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s *MessagesService) SendMessage(
	ctx context.Context,
	command SendMessageCommand,
) (domain.Message, bool, error) {
	if err := command.validate(); err != nil {
		return domain.Message{}, false, fmt.Errorf(
			"validate send message command: %w",
			err,
		)
	}
	newMessage, err := domain.NewMessage(
		uuid.New(),
		command.ClientMessageID,
		command.ChatID,
		command.SenderID,
		command.Content,
		time.Now(),
	)
	if err != nil {
		return domain.Message{}, false, fmt.Errorf("new message: %w", err)
	}

	existing, err := s.messagesRepo.GetMessageByClientID(
		ctx,
		newMessage.SenderID,
		newMessage.ClientMessageID,
	)
	if err == nil {
		if newMessage.Content != existing.Content ||
			newMessage.SenderID != existing.SenderID ||
			newMessage.ChatID != existing.ChatID {
			return domain.Message{}, false, fmt.Errorf(
				"sent messages mismatch: %w",
				ErrMessageConflict,
			)
		}
		return existing, false, nil
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Message{}, false, fmt.Errorf("get message by client id: %w", err)
	}

	err = s.txmanager.WithinTransaction(ctx, func(ctx context.Context) error {
		chat, err := s.chatsRepo.GetChatForUpdate(ctx, newMessage.ChatID)
		if err != nil {
			return fmt.Errorf("get chat for update: %w", err)
		}
		newMessage, err = domain.NewMessage(
			newMessage.ID,
			newMessage.ClientMessageID,
			newMessage.ChatID,
			newMessage.SenderID,
			newMessage.Content,
			time.Now(),
		)
		if err != nil {
			return fmt.Errorf("new message: %w", err)
		}
		switch chat.Type {
		case domain.ChatTypeDirect:
			directMsgState, err := s.chatsRepo.GetDirectMessageState(ctx, newMessage.ChatID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return ErrInternalInconsistency
				}
				return fmt.Errorf("get direct message state: %w", err)
			}
			if newMessage.SenderID != directMsgState.Users[0].UserID &&
				newMessage.SenderID != directMsgState.Users[1].UserID {
				return domain.ErrNotFound
			}
			if directMsgState.Users[0].Deleted || directMsgState.Users[1].Deleted {
				return ErrMessageTargetUnavailable
			}

		case domain.ChatTypeGroup:
			groupSenderState, err := s.chatsRepo.GetGroupSenderState(
				ctx,
				newMessage.ChatID,
				newMessage.SenderID,
			)

			if err != nil {
				return fmt.Errorf("get group sender state: %w", err)
			}

			if groupSenderState.Deleted {
				return ErrMessageTargetUnavailable
			}
		default:
			return errors.New("unknown chat type")
		}

		if err = s.messagesRepo.AppendMessage(ctx, newMessage); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return ErrInternalInconsistency
			}
			return fmt.Errorf("append message: %w", err)
		}

		if err := s.messagesRepo.MarkAsRead(
			ctx,
			newMessage.ChatID,
			newMessage.SenderID,
			newMessage.ID,
		); err != nil {
			return fmt.Errorf("repo: mark as read: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			existing, err := s.messagesRepo.GetMessageByClientID(
				ctx,
				newMessage.SenderID,
				newMessage.ClientMessageID,
			)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return domain.Message{}, false, ErrInternalInconsistency
				}
				return domain.Message{}, false, fmt.Errorf("get message by client id: %w", err)
			}
			if newMessage.Content != existing.Content ||
				newMessage.SenderID != existing.SenderID ||
				newMessage.ChatID != existing.ChatID {
				return domain.Message{}, false, fmt.Errorf(
					"sent messages mismatch: %w", ErrMessageConflict,
				)
			}
			return existing, false, nil
		}
		return domain.Message{}, false, fmt.Errorf("transaction: %w", err)
	}
	s.notifier.MessageCreated(ctx, newMessage)
	return newMessage, true, nil
}

type SendMessageCommand struct {
	SenderID        uuid.UUID
	ChatID          uuid.UUID
	ClientMessageID uuid.UUID
	Content         string
}

func (c SendMessageCommand) validate() error {
	details := make(map[string]string)
	if c.SenderID == uuid.Nil {
		details["sender_id"] = "sender id is nil"
	}
	if c.ChatID == uuid.Nil {
		details["chat_id"] = "chat id is nil"
	}
	if len(details) > 0 {
		return domain.DetailedError{
			Err:     ErrInvalidInput,
			Details: details,
		}
	}
	return nil
}

type AccountState struct {
	UserID  uuid.UUID
	Deleted bool
}

type DirectMessageState struct {
	Users [2]AccountState
}
