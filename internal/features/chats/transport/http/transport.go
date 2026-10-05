package chats_transport_http

import (
	http_middleware "github.com/wrzdx/Nero/internal/core/transport/http/middleware"

	"github.com/go-chi/chi/v5"
)

type ChatsHandler struct {
	chatsService ChatsService
}

func NewChatsHandler(chatsService ChatsService) *ChatsHandler {
	return &ChatsHandler{
		chatsService: chatsService,
	}
}

func (h *ChatsHandler) Router(authMW http_middleware.Middleware) chi.Router {
	router := chi.NewRouter()
	router.Use(authMW)
	router.Get("/", h.ListChats)
	router.Post("/directs", h.CreateDirect)
	router.Post("/groups", h.CreateGroup)
	router.Put("/groups/{chat_id}", h.UpdateGroup)
	router.Get("/groups/{chat_id}/participants", h.ListGroupParticipants)
	router.Post("/groups/{chat_id}/participants", h.AddGroupParticipants)
	router.Delete("/groups/{chat_id}/participants", h.RemoveGroupParticipant)
	return router
}
