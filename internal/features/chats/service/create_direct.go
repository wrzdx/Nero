package chats_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s *ChatsService) CreateDirect(
	ctx context.Context,
	currentUserID uuid.UUID,
	peerID uuid.UUID,
) (domain.DirectChat, bool, error) {
	now := time.Now()
	direct, err := domain.NewDirectChat(uuid.New(), currentUserID, peerID, now)
	if err != nil {
		return domain.DirectChat{}, false, fmt.Errorf("new direct chat: %w", err)
	}

	participant1, err := domain.NewChatParticipant(direct.Chat.ID, direct.User1ID, nil, now)
	if err != nil {
		return domain.DirectChat{}, false, fmt.Errorf(
			"new chat participant: %w: %w",
			err,
			domain.ErrInvalidDirectChat,
		)
	}

	participant2, err := domain.NewChatParticipant(direct.Chat.ID, direct.User2ID, nil, now)
	if err != nil {
		return domain.DirectChat{}, false, fmt.Errorf(
			"new chat participant: %w: %w",
			err,
			domain.ErrInvalidDirectChat,
		)
	}

	err = s.txmanager.WithinTransaction(ctx, func(ctx context.Context) error {
		user1, err := s.usersRepo.GetUserForUpdate(ctx, direct.User1ID)
		if err != nil {
			return fmt.Errorf("get user for update: %w", err)
		}
		if user1.DeletedAt != nil {
			return domain.ErrNotFound
		}
		user2, err := s.usersRepo.GetUserForUpdate(ctx, direct.User2ID)
		if err != nil {
			return fmt.Errorf("get user for update: %w", err)
		}

		if user2.DeletedAt != nil {
			return domain.ErrNotFound
		}

		if err := s.chatsRepo.CreateDirect(
			ctx, direct, participant1, participant2); err != nil {
			return fmt.Errorf("create direct in repo: %w", err)
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			direct, err := s.chatsRepo.GetDirectByUsers(ctx, direct.User1ID, direct.User2ID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return domain.DirectChat{}, false, errors.New("internal inconsistency")
				}
				return domain.DirectChat{}, false, fmt.Errorf("get direct chat from repo: %w", err)
			}
			return direct, false, nil
		}
		return domain.DirectChat{}, false, fmt.Errorf("transaction: %w", err)
	}

	return direct, true, nil
}
