package chats_transport_http

import (
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	"net/http"
)

func (h *ChatsHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	claims := core_context.ClaimsRequired(ctx)

	var request CreateGroupRequest
	if err := decodeUsernameRequest(r, &request); err != nil {
		sender.Error(err)
		return
	}
	names, err := requestUsernames(request.ParticipantUsernames, "participant_usernames")
	if err != nil {
		sender.Error(err)
		return
	}
	group, err := h.chatsService.CreateGroupByUsernames(
		ctx,
		claims.UserID,
		chats_service.CreateGroupByUsernamesCommand{
			Title:                request.Title,
			ParticipantUsernames: names,
		},
	)
	if err != nil {
		sender.Error(err)
		return
	}

	response := CreateGroupResponse{
		ChatResponse: chatResponseFromDomain(group.Chat),
		Title:        group.Title,
	}
	sender.OK(http.StatusCreated, response)
}

type CreateGroupResponse GroupResponse

type CreateGroupRequest struct {
	Title                string   `json:"title" validate:"required"`
	ParticipantUsernames []string `json:"participant_usernames"`
}
