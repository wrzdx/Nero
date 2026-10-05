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

func (h *MessagesHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	claims := core_context.ClaimsRequired(ctx)

	var request SendMessageRequest
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

	message, isCreated, err := h.messagesService.SendMessage(
		ctx,
		messages_service.SendMessageCommand{
			SenderID:        claims.UserID,
			ChatID:          chatID,
			ClientMessageID: uuid.MustParse(request.ClientMessageID),
			Content:         request.Content,
		},
	)
	if err != nil {
		sender.Error(err)
		return
	}
	response := SendMessageResponse(messageResponseFromDomain(message))
	status := http.StatusOK
	if isCreated {
		status = http.StatusCreated
	}

	sender.OK(status, response)
}

type SendMessageRequest struct {
	ClientMessageID string `json:"client_message_id" validate:"required,uuid"`
	Content         string `json:"content" validate:"required"`
}

type SendMessageResponse MessageResponse
