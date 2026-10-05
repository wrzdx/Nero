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

func (h *ChatsHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	claims := core_context.ClaimsRequired(ctx)

	var request UpdateGroupRequest
	if err := http_request.DecodeAndValidateRequestBody(r, &request); err != nil {
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

	updated, err := h.chatsService.UpdateGroup(
		ctx,
		chats_service.UpdateGroupCommand{
			RequesterID: claims.UserID,
			GroupID:     chatID,
			Title:       request.Title,
		},
	)
	if err != nil {
		sender.Error(err)
		return
	}

	response := UpdateGroupResponse{
		ChatResponse: chatResponseFromDomain(updated.Chat),
		Title:        updated.Title,
	}
	sender.OK(http.StatusOK, response)
}

type UpdateGroupRequest struct {
	Title string `json:"title" validate:"required"`
}

type UpdateGroupResponse GroupResponse
