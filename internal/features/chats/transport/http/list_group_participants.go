package chats_transport_http

import (
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_cursor "github.com/wrzdx/Nero/internal/core/transport/http/cursor"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *ChatsHandler) ListGroupParticipants(w http.ResponseWriter, r *http.Request) {
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

	queryParams := r.URL.Query()
	cursorPayload, err := http_cursor.DecodeAndValidate[groupParticipantCursorPayload](
		queryParams.Get("cursor"),
	)
	if err != nil {
		sender.Error(err)
		return
	}

	var limit int
	if limitString := queryParams.Get("limit"); limitString != "" {
		limit, err = strconv.Atoi(limitString)
		if err != nil {
			sender.Error(http_request.NewFieldError(
				map[string]string{"limit": "invalid limit query param"},
			))
			return
		}
	}
	var cursor *chats_service.GroupParticipantCursor
	if cursorPayload != nil {
		cursor = &chats_service.GroupParticipantCursor{
			ParticipantID: uuid.MustParse(cursorPayload.ParticipantID),
			JoinedAt:      cursorPayload.JoinedAt,
		}
	}

	page, err := h.chatsService.ListGroupParticipants(
		ctx,
		claims.UserID,
		chats_service.ListGroupParticipantsQuery{
			ChatID: chatID,
			Before: cursor,
			Limit:  limit,
		},
	)
	if err != nil {
		sender.Error(err)
		return
	}

	var responseCursorPayload *groupParticipantCursorPayload
	if page.NextCursor != nil {
		responseCursorPayload = &groupParticipantCursorPayload{
			ParticipantID: page.NextCursor.ParticipantID.String(),
			JoinedAt:      page.NextCursor.JoinedAt,
		}
	}

	nextCursor, err := http_cursor.Encode(responseCursorPayload)
	if err != nil {
		sender.Error(err)
		return
	}

	response := ListGroupParticipantsResponse{
		Participants: make([]GroupParticipantResponse, len(page.Participants)),
		NextCursor:   nextCursor,
	}
	for index, item := range page.Participants {
		response.Participants[index] = GroupParticipantResponse{
			UserID:    item.ID,
			Username:  item.Username,
			FirstName: item.FirstName,
			LastName:  item.LastName,
			Role:      item.Role,
			JoinedAt:  item.JoinedAt,
		}
	}

	sender.OK(http.StatusOK, response)
}

type groupParticipantCursorPayload struct {
	ParticipantID string    `json:"participant_id" validate:"required,uuid"`
	JoinedAt      time.Time `json:"joined_at" validate:"required"`
}

type ListGroupParticipantsResponse struct {
	Participants []GroupParticipantResponse `json:"participants"`
	NextCursor   *string                    `json:"next_cursor"`
}

type GroupParticipantResponse struct {
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	FirstName string    `json:"first_name"`
	LastName  *string   `json:"last_name"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}
