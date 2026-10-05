package chats_transport_http

import (
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	"net/http"
)

func (h *ChatsHandler) CreateDirect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	sender := http_response.NewHTTPSender(log, w, errorMapper)
	claims := core_context.ClaimsRequired(ctx)

	var request CreateDirectRequest
	if err := decodeUsernameRequest(r, &request); err != nil {
		sender.Error(err)
		return
	}

	username, err := requestUsername(request.PeerUsername, "peer_username")
	if err != nil {
		sender.Error(err)
		return
	}
	direct, isCreated, err := h.chatsService.CreateDirectByUsername(
		ctx,
		claims.UserID,
		username,
	)
	if err != nil {
		sender.Error(err)
		return
	}
	response := CreateDirectResponse(chatResponseFromDomain(direct.Chat))
	status := http.StatusOK
	if isCreated {
		status = http.StatusCreated
	}
	sender.OK(status, response)
}

type CreateDirectRequest struct {
	PeerUsername string `json:"peer_username" validate:"required"`
}

type CreateDirectResponse ChatResponse
