package web_handlers

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/auth"
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/domain"
	http_cursor "github.com/wrzdx/Nero/internal/core/transport/http/cursor"
	http_request "github.com/wrzdx/Nero/internal/core/transport/http/request"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	messages_service "github.com/wrzdx/Nero/internal/features/messages/service"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	web_views "github.com/wrzdx/Nero/internal/web/views"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type WebUsers interface {
	SearchUsers(context.Context, uuid.UUID, users_service.SearchUsersQuery) ([]users_service.UserSearchResult, error)
	GetUser(context.Context, uuid.UUID) (domain.User, error)
	UpdateProfile(context.Context, uuid.UUID, users_service.UpdateProfileCommand) (domain.User, error)
	DeleteAccount(context.Context, uuid.UUID) error
}
type WebChats interface {
	ListChats(context.Context, uuid.UUID, chats_service.ListChatsQuery) (chats_service.ChatPage, error)
	CreateDirectByUsername(context.Context, uuid.UUID, string) (domain.DirectChat, bool, error)
	CreateGroupByUsernames(context.Context, uuid.UUID, chats_service.CreateGroupByUsernamesCommand) (domain.GroupChat, error)
	ListGroupParticipants(context.Context, uuid.UUID, chats_service.ListGroupParticipantsQuery) (chats_service.GroupParticipantPage, error)
	UpdateGroup(context.Context, chats_service.UpdateGroupCommand) (domain.GroupChat, error)
	AddGroupParticipantsByUsernames(context.Context, chats_service.AddGroupParticipantsByUsernamesCommand) ([]chats_service.UsernameParticipantResult, error)
	RemoveGroupParticipant(context.Context, chats_service.RemoveGroupParticipantCommand) error
}
type WebMessages interface {
	GetMessages(context.Context, uuid.UUID, messages_service.GetMessagesQuery) (messages_service.MessagePage, error)
	SendMessage(context.Context, messages_service.SendMessageCommand) (domain.Message, bool, error)
	EditMessage(context.Context, messages_service.UpdateMessageCommand) (domain.Message, error)
	DeleteMessage(context.Context, messages_service.DeleteMessageCommand) error
	MarkAsRead(context.Context, messages_service.MarkAsReadCommand) error
}
type PasswordService interface {
	ChangePassword(context.Context, uuid.UUID, string, string) error
}

type AppHandler struct {
	auth      *AuthHandler
	sessions  *SessionManager
	users     WebUsers
	chats     WebChats
	messages  WebMessages
	passwords PasswordService
}

func NewAppHandler(a *AuthHandler, s *SessionManager, users WebUsers, chats WebChats, messages WebMessages, passwords PasswordService) *AppHandler {
	return &AppHandler{a, s, users, chats, messages, passwords}
}

func (h *AppHandler) Home(w http.ResponseWriter, r *http.Request) {
	signedIn := false
	if cookie, err := r.Cookie(accessCookieName); err == nil {
		_, err = h.auth.tokens.ParseAccessToken(cookie.Value)
		signedIn = err == nil
	}
	h.auth.render(w, r, 200, web_views.Home(signedIn))
}

func requester(r *http.Request) uuid.UUID { return core_context.ClaimsRequired(r.Context()).UserID }
func routeID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, domain.ErrNotFound
	}
	return id, nil
}
func person(user domain.User) web_views.Person {
	p := user.Profile
	name := p.FirstName
	last, bio := "", ""
	if p.LastName != nil {
		last = *p.LastName
		name += " " + last
	}
	if p.Bio != nil {
		bio = *p.Bio
	}
	return web_views.Person{ID: user.ID.String(), Username: p.Username, FirstName: p.FirstName, LastName: last, Bio: bio, Name: name, Initials: web_views.Initials(name)}
}
func row(item chats_service.ChatItem) web_views.ChatRow {
	view := web_views.ChatRow{ID: item.Chat.ID.String(), Title: "Разговор", Group: item.Chat.Type == domain.ChatTypeGroup, Unread: item.UnreadCount, Preview: "Начало разговора"}
	if item.DirectPeer != nil {
		view.Title = item.DirectPeer.FirstName
		if item.DirectPeer.LastName != nil {
			view.Title += " " + *item.DirectPeer.LastName
		}
	}
	if item.GroupInfo != nil {
		view.Title = item.GroupInfo.Title
	}
	view.Initials = web_views.Initials(view.Title)
	if item.LastMessage != nil {
		view.Preview = item.LastMessage.Message.Content
		if view.Group {
			view.Preview = item.LastMessage.SenderFirstName + ": " + view.Preview
		}
		view.Time = shortTime(item.LastMessage.Message.CreatedAt)
	}
	return view
}
func shortTime(t time.Time) string {
	if t.In(time.Local).Format("2006-01-02") == time.Now().In(time.Local).Format("2006-01-02") {
		return t.In(time.Local).Format("15:04")
	}
	return t.In(time.Local).Format("02.01")
}
func cursorString[T any](cursor *T) string {
	encoded, _ := http_cursor.Encode(cursor)
	if encoded == nil {
		return ""
	}
	return *encoded
}

func (h *AppHandler) base(r *http.Request, screen string) (web_views.AppData, error) {
	me, err := h.users.GetUser(r.Context(), requester(r))
	if err != nil {
		return web_views.AppData{}, err
	}
	page, err := h.chats.ListChats(r.Context(), me.ID, chats_service.ListChatsQuery{Limit: 50})
	if err != nil {
		return web_views.AppData{}, err
	}
	data := web_views.AppData{Me: person(me), Screen: screen, Title: "Разговоры"}
	for _, item := range page.Chats {
		data.Chats = append(data.Chats, row(item))
	}
	if page.NextCursor != nil {
		data.MoreChatsURL = "/app/chats/list?cursor=" + url.QueryEscape(cursorString(page.NextCursor))
	}
	return data, nil
}

func (h *AppHandler) findChat(r *http.Request, id uuid.UUID) (chats_service.ChatItem, error) {
	// A single-chat service query does not exist yet: walk the user's paginated list.
	var before *chats_service.ChatCursor
	for {
		page, err := h.chats.ListChats(r.Context(), requester(r), chats_service.ListChatsQuery{Before: before, Limit: 100})
		if err != nil {
			return chats_service.ChatItem{}, err
		}
		for _, item := range page.Chats {
			if item.Chat.ID == id {
				return item, nil
			}
		}
		if page.NextCursor == nil {
			return chats_service.ChatItem{}, domain.ErrNotFound
		}
		if before != nil && *page.NextCursor == *before {
			return chats_service.ChatItem{}, errors.New("chat cursor did not advance")
		}
		before = page.NextCursor
	}
}

func (h *AppHandler) Index(w http.ResponseWriter, r *http.Request) {
	data, err := h.base(r, "list")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	h.page(w, r, 200, web_views.AppPage(data), web_views.AppView(data))
}
func (h *AppHandler) Conversation(w http.ResponseWriter, r *http.Request) {
	id, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	data, err := h.conversation(r, id)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	h.page(w, r, 200, web_views.AppPage(data), web_views.AppView(data))
}
func (h *AppHandler) conversation(r *http.Request, id uuid.UUID) (web_views.AppData, error) {
	data, err := h.base(r, "chat")
	if err != nil {
		return data, err
	}
	item, err := h.findChat(r, id)
	if err != nil {
		return data, err
	}
	data.Active = id.String()
	data.Title = row(item).Title
	data.Group = item.Chat.Type == domain.ChatTypeGroup
	data.Subtitle = "Личный разговор"
	if item.DirectPeer != nil {
		data.PeerID = item.DirectPeer.ID.String()
		data.Subtitle = "@" + item.DirectPeer.Username
	}
	if data.Group {
		data.Subtitle = "Групповой разговор"
	}
	data.Feed, err = h.feed(r, id, nil, false)
	data.ClientMessageID = uuid.NewString()
	return data, err
}

func (h *AppHandler) ChatList(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") != "true" {
		redirectPage(w, r, "/app")
		return
	}
	cursor, err := http_cursor.DecodeAndValidate[chats_service.ChatCursor](r.URL.Query().Get("cursor"))
	if err != nil {
		h.failure(w, r, err)
		return
	}
	page, err := h.chats.ListChats(r.Context(), requester(r), chats_service.ListChatsQuery{Before: cursor, Limit: 50})
	if err != nil {
		h.failure(w, r, err)
		return
	}
	rows := make([]web_views.ChatRow, 0, len(page.Chats))
	for _, item := range page.Chats {
		rows = append(rows, row(item))
	}
	more := ""
	if page.NextCursor != nil {
		more = "/app/chats/list?cursor=" + url.QueryEscape(cursorString(page.NextCursor))
	}
	h.auth.render(w, r, 200, web_views.MoreChatRows(rows, more, r.URL.Query().Get("active")))
}

func (h *AppHandler) feed(r *http.Request, id uuid.UUID, cursor *messages_service.MessageCursor, after bool) (web_views.FeedData, error) {
	page, err := h.messages.GetMessages(r.Context(), requester(r), messages_service.GetMessagesQuery{ChatID: id, Cursor: cursor, Limit: 50, After: after})
	if err != nil {
		return web_views.FeedData{}, err
	}
	feed := web_views.FeedData{ChatID: id.String()}
	ordered := slices.Clone(page.Messages)
	if !after {
		slices.Reverse(ordered)
	}
	names := make(map[uuid.UUID]string)
	lastDay := ""
	for _, message := range ordered {
		name, ok := names[message.SenderID]
		if !ok {
			u, err := h.users.GetUser(r.Context(), message.SenderID)
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				return feed, err
			}
			name = "Удалённый аккаунт"
			if err == nil {
				name = person(u).Name
			}
			names[message.SenderID] = name
		}
		day := message.CreatedAt.In(time.Local).Format("02.01.2006")
		feed.Messages = append(feed.Messages, web_views.MessageRow{ID: message.ID.String(), Sender: name, Initials: web_views.Initials(name), Content: message.Content, Time: message.CreatedAt.In(time.Local).Format("15:04"), Day: day, Mine: message.SenderID == requester(r), Edited: message.UpdatedAt != nil, NewDay: !after && day != lastDay})
		lastDay = day
	}
	if len(ordered) > 0 {
		latest := ordered[len(ordered)-1]
		feed.LastID = latest.ID.String()
		feed.Cursor = cursorString(&messages_service.MessageCursor{MessageID: latest.ID, CreatedAt: latest.CreatedAt})
	} else if cursor != nil {
		feed.LastID = cursor.MessageID.String()
		feed.Cursor = cursorString(cursor)
	}
	if !after && page.NextCursor != nil {
		feed.OlderURL = "/app/chats/" + id.String() + "/messages?cursor=" + url.QueryEscape(cursorString(page.NextCursor))
	}
	return feed, nil
}

func (h *AppHandler) Messages(w http.ResponseWriter, r *http.Request) {
	id, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	cursor, err := http_cursor.DecodeAndValidate[messages_service.MessageCursor](r.URL.Query().Get("cursor"))
	if err != nil {
		h.failure(w, r, err)
		return
	}
	after := r.URL.Query().Get("after") == "1"
	feed, err := h.feed(r, id, cursor, after)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		redirectPage(w, r, "/app/chats/"+id.String())
		return
	}
	if after {
		h.auth.render(w, r, 200, web_views.NewFeed(feed))
	} else if cursor != nil {
		h.auth.render(w, r, 200, web_views.OlderFeed(feed))
	} else {
		h.auth.render(w, r, 200, web_views.Feed(feed))
	}
}

func parseAppForm(w http.ResponseWriter, r *http.Request) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" {
		return fmt.Errorf("form media: %w", messages_service.ErrInvalidInput)
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("form body: %w", messages_service.ErrInvalidInput)
	}
	return nil
}
func (h *AppHandler) Send(w http.ResponseWriter, r *http.Request) {
	id, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	clientID, err := uuid.Parse(r.PostForm.Get("client_message_id"))
	content := r.PostForm.Get("content")
	if err != nil || clientID == uuid.Nil {
		h.composerError(w, r, id, uuid.NewString(), content, "Не удалось отправить сообщение. Попробуй ещё раз.", 422)
		return
	}
	_, _, err = h.messages.SendMessage(r.Context(), messages_service.SendMessageCommand{ChatID: id, SenderID: requester(r), ClientMessageID: clientID, Content: content})
	if err != nil {
		status, message := publicError(err)
		if status >= 500 {
			h.auth.log.Error("web send message", zap.Error(err))
		}
		h.composerError(w, r, id, clientID.String(), content, message, status)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Trigger", `{"nero:message-sent":true}`)
		h.auth.render(w, r, 200, web_views.Composer(id.String(), uuid.NewString(), "", ""))
		return
	}
	redirectPage(w, r, "/app/chats/"+id.String())
}
func (h *AppHandler) composerError(w http.ResponseWriter, r *http.Request, id uuid.UUID, clientID, content, message string, status int) {
	if r.Header.Get("HX-Request") == "true" {
		h.auth.render(w, r, status, web_views.Composer(id.String(), clientID, content, message))
		return
	}
	data, err := h.conversation(r, id)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	data.Draft = content
	data.Notice = message
	data.ClientMessageID = clientID
	h.auth.render(w, r, status, web_views.AppPage(data))
}
func (h *AppHandler) Edit(w http.ResponseWriter, r *http.Request) {
	chatID, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	messageID, err := routeID(r, "message_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	content := r.PostForm.Get("content")
	m, err := h.messages.EditMessage(r.Context(), messages_service.UpdateMessageCommand{ChatID: chatID, MessageID: messageID, SenderID: requester(r), Content: content})
	if err != nil {
		status, message := publicError(err)
		if r.Header.Get("HX-Request") == "true" {
			h.auth.render(w, r, status, web_views.EditMessage(chatID.String(), messageID.String(), content, message))
			return
		}
		h.failure(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		redirectPage(w, r, "/app/chats/"+chatID.String())
		return
	}
	u, err := h.users.GetUser(r.Context(), requester(r))
	if err != nil {
		h.failure(w, r, err)
		return
	}
	name := person(u).Name
	h.auth.render(w, r, 200, web_views.Messages([]web_views.MessageRow{{ID: m.ID.String(), Content: m.Content, Sender: name, Initials: web_views.Initials(name), Time: m.CreatedAt.In(time.Local).Format("15:04"), Mine: true, Edited: m.UpdatedAt != nil}}, chatID.String()))
}
func (h *AppHandler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	chatID, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	messageID, err := routeID(r, "message_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	err = h.messages.DeleteMessage(r.Context(), messages_service.DeleteMessageCommand{ChatID: chatID, MessageID: messageID, SenderID: requester(r)})
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(200)
		return
	}
	redirectPage(w, r, "/app/chats/"+chatID.String())
}
func (h *AppHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	chatID, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	messageID, err := uuid.Parse(r.PostForm.Get("message_id"))
	if err != nil {
		h.failure(w, r, messages_service.ErrInvalidInput)
		return
	}
	if err := h.messages.MarkAsRead(r.Context(), messages_service.MarkAsReadCommand{ChatID: chatID, UserID: requester(r), MessageID: messageID}); err != nil {
		h.failure(w, r, err)
		return
	}
	w.WriteHeader(204)
}

func (h *AppHandler) page(w http.ResponseWriter, r *http.Request, status int, full, partial templ.Component) {
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true" {
		h.auth.render(w, r, status, partial)
	} else {
		h.auth.render(w, r, status, full)
	}
}
func publicError(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return 404, "Этот разговор или пользователь недоступен."
	case errors.Is(err, domain.ErrAlreadyExists):
		return 409, "Это имя пользователя уже занято."
	case errors.Is(err, chats_service.ErrNotEnoughRights):
		return 403, "Для этого действия нужны права администратора."
	case errors.Is(err, chats_service.ErrOwnerCannotQuitGroup):
		return 422, "Владелец не может выйти из группы."
	case errors.Is(err, messages_service.ErrMessageTargetUnavailable):
		return 409, "Сейчас нельзя отправить сообщение этому собеседнику."
	case errors.Is(err, messages_service.ErrMessageConflict):
		return 409, "Это сообщение уже отправлено. Обнови разговор перед повторной отправкой."
	case errors.Is(err, domain.ErrInvalidMessage):
		return 422, "Сообщение должно содержать от 1 до 4096 символов."
	case errors.Is(err, domain.ErrInvalidGroupChat):
		return 422, "Проверь название группы и список участников."
	case errors.Is(err, domain.ErrInvalidUserProfile):
		return 422, "Проверь поля профиля."
	case errors.Is(err, auth.ErrPasswordMismatch):
		return 422, "Текущий пароль указан неверно."
	case errors.Is(err, auth.ErrInvalidPassword):
		return 422, "Новый пароль должен отличаться от текущего: от 15 символов и до 72 байт."
	case errors.Is(err, messages_service.ErrInvalidInput), errors.Is(err, chats_service.ErrInvalidInput), errors.Is(err, users_service.ErrInvalidSearchQuery), errors.Is(err, http_request.ErrInvalidRequest):
		return 422, "Проверь введённые данные."
	default:
		return 500, "Не удалось выполнить запрос. Попробуй ещё раз."
	}
}
func (h *AppHandler) failure(w http.ResponseWriter, r *http.Request, err error) {
	status, message := publicError(err)
	if status >= 500 {
		h.auth.log.Error("web application request", zap.Error(err))
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Retarget", "#app-toast")
		w.Header().Set("HX-Reswap", "innerHTML")
		h.auth.render(w, r, status, web_views.ErrorNotice(message))
		return
	}
	h.auth.render(w, r, status, web_views.AppFailure(message))
}
