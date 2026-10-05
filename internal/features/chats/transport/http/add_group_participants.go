package chats_transport_http

import (
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *ChatsHandler) AddGroupParticipants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	claims := core_context.ClaimsRequired(ctx)

	var request AddGroupParticipantsRequest
	if err := decodeUsernameRequest(r, &request); err != nil {
		sender.Error(err)
		return
	}
	names, err := requestUsernames(request.ParticipantUsernames, "participant_usernames")
	if err != nil {
		sender.Error(err)
		return
	}

	chatIDStr := chi.URLParam(r, "chat_id")
	chatID, err := uuid.Parse(chatIDStr)

	if err != nil {
		sender.Error(http_request.NewFieldError(map[string]string{
			"chat_id": "invalid uuid",
		}))
		return
	}
	result, err := h.chatsService.AddGroupParticipantsByUsernames(
		ctx,
		chats_service.AddGroupParticipantsByUsernamesCommand{
			GroupID:              chatID,
			RequesterID:          claims.UserID,
			ParticipantUsernames: names,
		},
	)
	if err != nil {
		sender.Error(err)
		return
	}

	response := make([]AddGroupParticipantItem, 0, len(result))
	for _, status := range result {
		response = append(response, AddGroupParticipantItem{
			Username: status.Username,
			Status:   string(status.Status),
		})
	}
	sender.OK(http.StatusOK, response)
}

type AddGroupParticipantItem struct {
	Username string `json:"username"`
	Status   string `json:"status"`
}

type AddGroupParticipantsResponse []AddGroupParticipantItem

type AddGroupParticipantsRequest struct {
	ParticipantUsernames []string `json:"participant_usernames"`
}
