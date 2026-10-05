package messages_transport_http

import (
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	messages_service "github.com/wrzdx/Nero/internal/features/messages/service"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *MessagesHandler) EditMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	claims := core_context.ClaimsRequired(ctx)

	var request UpdateMessageRequest
	if err := http_request.DecodeAndValidateRequestBody(r, &request); err != nil {
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
	msgIDStr := chi.URLParam(r, "message_id")
	msgID, err := uuid.Parse(msgIDStr)

	if err != nil {
		sender.Error(http_request.NewFieldError(map[string]string{
			"message_id": "invalid uuid",
		}))
		return
	}

	updated, err := h.messagesService.EditMessage(
		ctx,
		messages_service.UpdateMessageCommand{
			SenderID:  claims.UserID,
			ChatID:    chatID,
			Content:   request.Content,
			MessageID: msgID,
		},
	)

	if err != nil {
		sender.Error(err)
		return
	}

	response := UpdateMessageResponse(messageResponseFromDomain(updated))
	sender.OK(http.StatusOK, response)
}

type UpdateMessageRequest struct {
	Content string `json:"content" validate:"required"`
}

type UpdateMessageResponse MessageResponse
