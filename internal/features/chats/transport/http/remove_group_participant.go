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

func (h *ChatsHandler) RemoveGroupParticipant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	claims := core_context.ClaimsRequired(ctx)

	var request RemoveGroupParticipantRequest
	if err := decodeUsernameRequest(r, &request); err != nil {
		sender.Error(err)
		return
	}

	chatIDStr := chi.URLParam(r, "chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		sender.Error(http_request.NewFieldError(
			map[string]string{
				"chat_id": "invalid uuid",
			},
		))
		return
	}
	username, err := requestUsername(request.TargetUsername, "target_username")
	if err != nil {
		sender.Error(err)
		return
	}

	if err := h.chatsService.RemoveGroupParticipantByUsername(
		ctx,
		chats_service.RemoveGroupParticipantByUsernameCommand{
			GroupID:        chatID,
			RequesterID:    claims.UserID,
			TargetUsername: username,
		}); err != nil {
		sender.Error(err)
		return
	}

	sender.OK(http.StatusNoContent, nil)
}

type RemoveGroupParticipantRequest struct {
	TargetUsername string `json:"target_username" validate:"required"`
}
