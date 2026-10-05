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

func (h *MessagesHandler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	claims := core_context.ClaimsRequired(ctx)

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

	if err := h.messagesService.DeleteMessage(ctx, messages_service.DeleteMessageCommand{
		ChatID:    chatID,
		SenderID:  claims.UserID,
		MessageID: msgID,
	}); err != nil {
		sender.Error(err)
		return
	}
	sender.OK(http.StatusNoContent, nil)
}
