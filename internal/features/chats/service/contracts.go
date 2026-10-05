package chats_service

import (
	"context"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
)

type ChatsRepository interface {
	CreateDirect(
		ctx context.Context,
		direct domain.DirectChat,
		participant1, participant2 domain.ChatParticipant,
	) error

	GetDirectByUsers(
		ctx context.Context,
		user1ID, user2ID uuid.UUID,
	) (domain.DirectChat, error)

	ListChats(
		ctx context.Context,
		userID uuid.UUID,
		before *ChatCursor,
		limit int,
	) ([]ChatItem, error)

	GetGroupParticipant(
		ctx context.Context,
		chatID, userID uuid.UUID,
	) (domain.GroupParticipant, error)

	ListGroupParticipants(
		ctx context.Context,
		chatID uuid.UUID,
		before *GroupParticipantCursor,
		limit int,
	) ([]ParticipantInfo, error)

	GetParticipantsStatus(
		ctx context.Context,
		userIDs []uuid.UUID,
	) ([]ParticipantStatus, error)

	CreateGroup(
		ctx context.Context,
		group domain.GroupChat,
		participants []domain.GroupParticipant,
	) error

	AddGroupParticipants(
		ctx context.Context,
		groupID uuid.UUID,
		participants []domain.GroupParticipant,
	) ([]bool, error)

	RemoveGroupParticipant(
		ctx context.Context,
		participant domain.GroupParticipant,
	) error

	GetGroup(
		ctx context.Context,
		groupID uuid.UUID,
	) (domain.GroupChat, error)

	UpdateGroup(
		ctx context.Context,
		updated domain.GroupChat,
	) error
}

type TXManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type UsersRepository interface {
	GetUserByUsername(ctx context.Context, username string) (domain.User, error)

	GetUserForUpdate(
		ctx context.Context,
		userID uuid.UUID,
	) (domain.User, error)
}
