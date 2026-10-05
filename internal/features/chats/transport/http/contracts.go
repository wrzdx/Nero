package chats_transport_http

import (
	"context"
	"github.com/wrzdx/Nero/internal/core/domain"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"

	"github.com/google/uuid"
)

type ChatsService interface {
	CreateDirectByUsername(
		ctx context.Context,
		currentUserID uuid.UUID,
		peerUsername string,
	) (domain.DirectChat, bool, error)

	ListChats(
		ctx context.Context,
		requesterID uuid.UUID,
		query chats_service.ListChatsQuery,
	) (chats_service.ChatPage, error)

	CreateGroupByUsernames(
		ctx context.Context,
		creatorID uuid.UUID,
		command chats_service.CreateGroupByUsernamesCommand,
	) (domain.GroupChat, error)

	ListGroupParticipants(
		ctx context.Context,
		requesterID uuid.UUID,
		query chats_service.ListGroupParticipantsQuery,
	) (chats_service.GroupParticipantPage, error)

	AddGroupParticipantsByUsernames(
		ctx context.Context,
		command chats_service.AddGroupParticipantsByUsernamesCommand,
	) ([]chats_service.UsernameParticipantResult, error)

	RemoveGroupParticipantByUsername(
		ctx context.Context,
		command chats_service.RemoveGroupParticipantByUsernameCommand,
	) error

	UpdateGroup(
		ctx context.Context,
		command chats_service.UpdateGroupCommand,
	) (domain.GroupChat, error)
}
