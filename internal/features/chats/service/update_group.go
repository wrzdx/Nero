package chats_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
)

func (s *ChatsService) UpdateGroup(
	ctx context.Context,
	command UpdateGroupCommand,
) (domain.GroupChat, error) {
	if err := command.validate(); err != nil {
		return domain.GroupChat{}, fmt.Errorf("validate update group command: %w", err)
	}
	requesterStatus, err := s.chatsRepo.GetParticipantsStatus(
		ctx,
		[]uuid.UUID{command.RequesterID},
	)
	if err != nil {
		return domain.GroupChat{}, fmt.Errorf(
			"get requester status: %w",
			err,
		)
	}
	if len(requesterStatus) != 1 {
		return domain.GroupChat{}, errors.New("invalid get participant status len")
	}
	if !requesterStatus[0].Found {
		return domain.GroupChat{}, domain.DetailedError{
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
		return domain.GroupChat{}, fmt.Errorf(
			"get requester group participant: %w",
			err,
		)
	}

	if requester.Role() != domain.OwnerRole &&
		requester.Role() != domain.AdminRole {
		return domain.GroupChat{}, ErrNotEnoughRights
	}

	group, err := s.chatsRepo.GetGroup(ctx, command.GroupID)
	if err != nil {
		return domain.GroupChat{}, fmt.Errorf("get group from repo: %w", err)
	}
	updated, err := group.Update(command.Title)

	if err != nil {
		return domain.GroupChat{}, fmt.Errorf("update group: %w", err)
	}
	if err := s.chatsRepo.UpdateGroup(ctx, updated); err != nil {
		return domain.GroupChat{}, fmt.Errorf("update group in repo: %w", err)
	}

	return updated, nil
}

type UpdateGroupCommand struct {
	RequesterID uuid.UUID
	GroupID     uuid.UUID
	Title       string
}

func (c UpdateGroupCommand) validate() error {
	if c.RequesterID == uuid.Nil {
		return fmt.Errorf("requester id is nil: %w", ErrInvalidInput)
	}
	if c.GroupID == uuid.Nil {
		return fmt.Errorf("group id is nil: %w", ErrInvalidInput)
	}

	return nil
}
