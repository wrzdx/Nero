package chats_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s *ChatsService) CreateGroup(
	ctx context.Context,
	creatorID uuid.UUID,
	command CreateGroupCommand,
) (domain.GroupChat, error) {
	now := time.Now()
	group, err := domain.NewGroupChat(uuid.New(), command.Title, now)
	if err != nil {
		return domain.GroupChat{}, err
	}

	participants := make([]domain.GroupParticipant, 0, len(command.ParticipantIDs)+1)
	owner, err := domain.NewGroupParticipant(
		group.Chat.ID,
		creatorID,
		nil, now,
		domain.OwnerRole,
	)
	if err != nil {
		return domain.GroupChat{}, err
	}

	participants = append(participants, owner)

	for _, userID := range command.ParticipantIDs {
		member, err := domain.NewGroupParticipant(
			group.Chat.ID,
			userID,
			nil, now,
			domain.MemberRole,
		)
		if err != nil {
			return domain.GroupChat{}, fmt.Errorf(
				"new group participant: %w: %w",
				err,
				domain.ErrInvalidGroupChat,
			)
		}
		participants = append(participants, member)
	}

	if hasDuplicates(participants) {
		return domain.GroupChat{}, fmt.Errorf(
			"duplicate participants: %w",
			domain.ErrInvalidGroupChat,
		)
	}

	statuses, err := s.chatsRepo.GetParticipantsStatus(
		ctx,
		command.ParticipantIDs,
	)
	if err != nil {
		return domain.GroupChat{}, fmt.Errorf("get participant statuses: %w", err)
	}
	notFound := make(map[string]string)
	for _, status := range statuses {
		if !status.Found {
			notFound[status.UserID.String()] = "not found"
		}
	}
	if len(notFound) > 0 {
		return domain.GroupChat{}, domain.DetailedError{
			Err:     domain.ErrNotFound,
			Details: notFound,
		}
	}

	err = s.txmanager.WithinTransaction(ctx, func(ctx context.Context) error {
		userOwner, err := s.usersRepo.GetUserForUpdate(ctx, creatorID)
		if errors.Is(err, domain.ErrNotFound) || (err == nil && userOwner.DeletedAt != nil) {
			return domain.DetailedError{
				Err:     domain.ErrNotFound,
				Details: map[string]string{"creator_id": "creator not found"},
			}
		}
		if err != nil {
			return err
		}
		if err := s.chatsRepo.CreateGroup(ctx, group, participants); err != nil {
			return fmt.Errorf("create group: %w", err)
		}
		return nil
	})

	if err != nil {
		return domain.GroupChat{}, fmt.Errorf("transaction: %w", err)
	}

	return group, nil
}

type CreateGroupCommand struct {
	Title          string
	ParticipantIDs []uuid.UUID
}

type ParticipantStatus struct {
	UserID uuid.UUID
	Found  bool
}

func hasDuplicates(participants []domain.GroupParticipant) bool {
	seen := make(map[uuid.UUID]struct{})

	for _, p := range participants {
		if _, exists := seen[p.UserID]; exists {
			return true
		}
		seen[p.UserID] = struct{}{}
	}
	return false
}
