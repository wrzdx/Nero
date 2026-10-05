package chats_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
)

func (s *ChatsService) RemoveGroupParticipant(
	ctx context.Context,
	command RemoveGroupParticipantCommand,
) error {
	if err := command.validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	requesterStatus, err := s.chatsRepo.GetParticipantsStatus(
		ctx,
		[]uuid.UUID{command.RequesterID},
	)
	if err != nil {
		return fmt.Errorf(
			"get requester status: %w",
			err,
		)
	}
	if len(requesterStatus) != 1 {
		return errors.New("invalid get participant status len")
	}
	if !requesterStatus[0].Found {
		return domain.DetailedError{
			Err:     domain.ErrNotFound,
			Details: map[string]string{"requester_id": "not found"},
		}
	}
	requester, err := s.chatsRepo.GetGroupParticipant(
		ctx,
		command.GroupID,
		command.RequesterID,
	)
	if err != nil {
		return fmt.Errorf("get requester group participant: %w", err)
	}
	target, err := s.chatsRepo.GetGroupParticipant(
		ctx,
		command.GroupID,
		command.TargetID,
	)
	if err != nil {
		return fmt.Errorf("get target group participant: %w", err)
	}
	if requester.UserID == target.UserID &&
		requester.Role() == domain.OwnerRole {
		return ErrOwnerCannotQuitGroup
	}
	if !(requester.UserID == target.UserID ||
		requester.Role() == domain.OwnerRole ||
		(requester.Role() == domain.AdminRole && target.Role() == domain.MemberRole)) {
		return ErrNotEnoughRights
	}
	if err := s.txmanager.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.chatsRepo.RemoveGroupParticipant(ctx, target); err != nil {
			return fmt.Errorf("chats repo: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("transaction: %w", err)
	}
	return nil
}

type RemoveGroupParticipantCommand struct {
	GroupID     uuid.UUID
	RequesterID uuid.UUID
	TargetID    uuid.UUID
}

func (c RemoveGroupParticipantCommand) validate() error {
	if c.GroupID == uuid.Nil {
		return fmt.Errorf(
			"group id is nil: %w",
			ErrInvalidInput,
		)
	}

	if c.RequesterID == uuid.Nil {
		return fmt.Errorf(
			"requester id is nil: %w",
			ErrInvalidInput,
		)
	}

	if c.TargetID == uuid.Nil {
		return fmt.Errorf(
			"target id is nil: %w",
			ErrInvalidInput,
		)
	}

	return nil
}
