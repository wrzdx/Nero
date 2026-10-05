package web_handlers

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *AppHandler) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(http.NewCrossOriginProtection().Handler)
	r.Use(h.sessions.Middleware)
	r.Get("/", h.Index)
	r.Get("/chats/list", h.ChatList)
	r.Get("/chats/{chat_id}", h.Conversation)
	r.Get("/chats/{chat_id}/messages", h.Messages)
	r.Post("/chats/{chat_id}/messages", h.Send)
	r.Post("/chats/{chat_id}/messages/{message_id}/edit", h.Edit)
	r.Post("/chats/{chat_id}/messages/{message_id}/delete", h.DeleteMessage)
	r.Post("/chats/{chat_id}/read", h.MarkRead)
	r.Get("/new", h.NewChat)
	r.Post("/new", h.CreateChat)
	r.Get("/profile", h.Profile)
	r.Post("/profile", h.UpdateProfile)
	r.Get("/users/{user_id}", h.User)
	r.Get("/users/search", h.SearchUsers)
	r.Get("/password", h.Password)
	r.Post("/password", h.ChangePassword)
	r.Get("/account/delete", h.DeleteAccountPage)
	r.Post("/account/delete", h.DeleteAccount)
	r.Get("/groups/{chat_id}", h.Group)
	r.Post("/groups/{chat_id}/title", h.UpdateGroup)
	r.Post("/groups/{chat_id}/add", h.AddMembers)
	r.Post("/groups/{chat_id}/remove", h.RemoveMember)
	r.Post("/session", h.sessions.Token)
	return r
}
