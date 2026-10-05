package web_handlers

import (
	"context"
	"errors"
	"github.com/wrzdx/Nero/internal/core/auth"
	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/domain"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	messages_service "github.com/wrzdx/Nero/internal/features/messages/service"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type sessionStub struct {
	refresh func(string) (auth.TokenPair, error)
	logout  func(string) error
}

func TestHTMLLoginSetsAppSessionAndRedirect(t *testing.T) {
	a := testHandler(authStub{login: func(string, string) (auth.TokenPair, error) {
		return auth.TokenPair{Access: "access", Refresh: "refresh"}, nil
	}})
	s := NewSessionManager(sessionStub{}, a)
	s.SetRefreshTTL(time.Hour)
	w := httptest.NewRecorder()
	a.Login(w, formRequest("/login", url.Values{"username": {"daniel_k"}, "password": {"long-password-123"}}, true))
	require.Equal(t, 204, w.Code)
	require.Equal(t, "/app", w.Header().Get("HX-Redirect"))
	require.Len(t, w.Result().Cookies(), 3)
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == refreshCookieName {
			require.Equal(t, "/", cookie.Path)
			require.Equal(t, 3600, cookie.MaxAge)
			require.True(t, cookie.HttpOnly)
		}
	}
}

func (s sessionStub) Refresh(_ context.Context, token string) (auth.TokenPair, error) {
	return s.refresh(token)
}
func (s sessionStub) Logout(_ context.Context, token string) error { return s.logout(token) }

func TestHTMLSessionSharesRefreshAndExposesOnlyAccessToken(t *testing.T) {
	userID := uuid.New()
	var calls atomic.Int32
	a := testHandler(authStub{parse: func(token string) (auth.ParsedAccessToken, error) {
		if token != "new-access" {
			return auth.ParsedAccessToken{}, auth.ErrInvalidToken
		}
		return auth.ParsedAccessToken{AccessTokenClaims: auth.AccessTokenClaims{UserID: userID}}, nil
	}})
	s := NewSessionManager(sessionStub{refresh: func(token string) (auth.TokenPair, error) {
		calls.Add(1)
		if token != "old-refresh" {
			return auth.TokenPair{}, auth.ErrInvalidToken
		}
		return auth.TokenPair{Access: "new-access", Refresh: "new-refresh"}, nil
	}}, a)
	var wg sync.WaitGroup
	responses := make([]*httptest.ResponseRecorder, 12)
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/app/session", nil)
			r.AddCookie(&http.Cookie{Name: accessCookieName, Value: "expired"})
			r.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "old-refresh"})
			w := httptest.NewRecorder()
			s.Middleware(http.HandlerFunc(s.Token)).ServeHTTP(w, r)
			responses[i] = w
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 1, calls.Load())
	for _, w := range responses {
		require.Equal(t, 200, w.Code)
		require.JSONEq(t, `{"access_token":"new-access"}`, w.Body.String())
		require.NotContains(t, w.Body.String(), "refresh")
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		for _, cookie := range w.Result().Cookies() {
			require.True(t, cookie.HttpOnly)
			require.True(t, cookie.Secure)
		}
	}
	// Logout with an older browser request must revoke the already rotated session.
	s.service = sessionStub{logout: func(token string) error { require.Equal(t, "new-refresh", token); return nil }}
	r := httptest.NewRequest("POST", "/logout", nil)
	r.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "old-refresh"})
	w := httptest.NewRecorder()
	s.Logout(w, r)
	require.Equal(t, 303, w.Code)
	require.Empty(t, s.recent)
	for _, cookie := range w.Result().Cookies() {
		require.Equal(t, -1, cookie.MaxAge)
	}
}

func TestHTMLSessionRefreshFailuresAndCrossOriginProtection(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{{auth.ErrInvalidToken, 204}, {errors.New("database unavailable"), 503}} {
		a := testHandler(authStub{parse: func(string) (auth.ParsedAccessToken, error) { return auth.ParsedAccessToken{}, auth.ErrInvalidToken }})
		s := NewSessionManager(sessionStub{refresh: func(string) (auth.TokenPair, error) { return auth.TokenPair{}, tc.err }}, a)
		r := httptest.NewRequest("POST", "/app/session", nil)
		r.Header.Set("HX-Request", "true")
		r.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "bad"})
		w := httptest.NewRecorder()
		s.Middleware(http.HandlerFunc(s.Token)).ServeHTTP(w, r)
		require.Equal(t, tc.status, w.Code)
		require.NotContains(t, w.Body.String(), "database unavailable")
		if tc.status == 204 {
			require.Equal(t, "/login", w.Header().Get("HX-Redirect"))
		} else {
			require.Equal(t, "#app-toast", w.Header().Get("HX-Retarget"))
		}
	}
	a := testHandler(authStub{})
	h := NewAppHandler(a, NewSessionManager(sessionStub{}, a), nil, nil, nil, nil)
	r := httptest.NewRequest("POST", "/session", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	h.Router().ServeHTTP(w, r)
	require.Equal(t, 403, w.Code)
}

type messageStub struct {
	WebMessages
	send func(messages_service.SendMessageCommand) (domain.Message, bool, error)
	get  func(messages_service.GetMessagesQuery) (messages_service.MessagePage, error)
}

func (s messageStub) SendMessage(_ context.Context, c messages_service.SendMessageCommand) (domain.Message, bool, error) {
	return s.send(c)
}
func (s messageStub) GetMessages(_ context.Context, _ uuid.UUID, q messages_service.GetMessagesQuery) (messages_service.MessagePage, error) {
	return s.get(q)
}

type userStub struct {
	WebUsers
	get    func(uuid.UUID) (domain.User, error)
	update func(uuid.UUID, users_service.UpdateProfileCommand) (domain.User, error)
}

func (s userStub) GetUser(_ context.Context, id uuid.UUID) (domain.User, error) { return s.get(id) }
func (s userStub) UpdateProfile(_ context.Context, id uuid.UUID, c users_service.UpdateProfileCommand) (domain.User, error) {
	return s.update(id, c)
}

type chatStub struct{ WebChats }

func (s chatStub) ListChats(context.Context, uuid.UUID, chats_service.ListChatsQuery) (chats_service.ChatPage, error) {
	return chats_service.ChatPage{}, nil
}

func appRequest(r *http.Request, userID uuid.UUID, params map[string]string) *http.Request {
	ctx := core_context.WithClaims(r.Context(), core_context.ContextClaims{UserID: userID})
	routes := chi.NewRouteContext()
	for name, value := range params {
		routes.URLParams.Add(name, value)
	}
	return r.WithContext(context.WithValue(ctx, chi.RouteCtxKey, routes))
}

func TestMessageSendUsesAuthenticatedSenderAndKeepsRetryID(t *testing.T) {
	userID, chatID, clientID := uuid.New(), uuid.New(), uuid.New()
	for _, failed := range []bool{false, true} {
		a := testHandler(authStub{})
		h := NewAppHandler(a, nil, nil, nil, messageStub{send: func(c messages_service.SendMessageCommand) (domain.Message, bool, error) {
			require.Equal(t, userID, c.SenderID)
			require.Equal(t, chatID, c.ChatID)
			require.Equal(t, clientID, c.ClientMessageID)
			if failed {
				return domain.Message{}, false, errors.New("private DB error")
			}
			return domain.Message{}, true, nil
		}}, nil)
		r := appRequest(formRequest("/", url.Values{"client_message_id": {clientID.String()}, "content": {"<script>alert(1)</script>"}, "sender_id": {uuid.NewString()}}, true), userID, map[string]string{"chat_id": chatID.String()})
		w := httptest.NewRecorder()
		h.Send(w, r)
		require.NotContains(t, w.Body.String(), "<script>")
		if failed {
			require.Equal(t, 500, w.Code)
			require.Contains(t, w.Body.String(), clientID.String())
			require.Contains(t, w.Body.String(), "&lt;script&gt;")
			require.NotContains(t, w.Body.String(), "private DB error")
		} else {
			require.Equal(t, 200, w.Code)
			require.NotContains(t, w.Body.String(), clientID.String())
			require.NotEmpty(t, w.Header().Get("HX-Trigger"))
		}
	}
}

func TestProfileEmptyOptionalFieldsAreCleared(t *testing.T) {
	userID := uuid.New()
	me := domain.User{ID: userID, Profile: domain.UserProfile{Username: "daniel_k", FirstName: "Даниил"}}
	h := NewAppHandler(testHandler(authStub{}), nil, userStub{
		get: func(uuid.UUID) (domain.User, error) { return me, nil },
		update: func(id uuid.UUID, c users_service.UpdateProfileCommand) (domain.User, error) {
			require.Equal(t, userID, id)
			require.True(t, c.LastName.Set)
			require.Nil(t, c.LastName.Value)
			require.True(t, c.Bio.Set)
			require.Nil(t, c.Bio.Value)
			return me, nil
		},
	}, chatStub{}, nil, nil)
	r := appRequest(formRequest("/app/profile", url.Values{"username": {"daniel_k"}, "first_name": {"Даниил"}, "last_name": {" "}, "bio": {""}}, true), userID, nil)
	w := httptest.NewRecorder()
	h.UpdateProfile(w, r)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "Профиль сохранён")
	require.NotContains(t, w.Body.String(), "Ваш ID")
}

func TestFeedOrderingAndPrivateConversationErrors(t *testing.T) {
	userID, chatID := uuid.New(), uuid.New()
	old, newer := domain.Message{ID: uuid.New(), SenderID: userID, CreatedAt: time.Now().Add(-time.Minute), Content: "old"}, domain.Message{ID: uuid.New(), SenderID: userID, CreatedAt: time.Now(), Content: "new"}
	h := NewAppHandler(testHandler(authStub{}), nil, userStub{get: func(uuid.UUID) (domain.User, error) {
		return domain.User{ID: userID, Profile: domain.UserProfile{FirstName: "Даниил"}}, nil
	}}, nil, messageStub{get: func(q messages_service.GetMessagesQuery) (messages_service.MessagePage, error) {
		if q.After {
			return messages_service.MessagePage{Messages: []domain.Message{old, newer}}, nil
		}
		return messages_service.MessagePage{Messages: []domain.Message{newer, old}}, nil
	}}, nil)
	r := appRequest(httptest.NewRequest("GET", "/", nil), userID, map[string]string{"chat_id": chatID.String()})
	for _, after := range []bool{false, true} {
		feed, err := h.feed(r, chatID, nil, after)
		require.NoError(t, err)
		require.Equal(t, old.ID.String(), feed.Messages[0].ID)
		require.Equal(t, newer.ID.String(), feed.LastID)
	}
	h.messages = messageStub{get: func(messages_service.GetMessagesQuery) (messages_service.MessagePage, error) {
		return messages_service.MessagePage{}, domain.ErrNotFound
	}}
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.Messages(w, r)
	require.Equal(t, 404, w.Code)
	require.NotContains(t, w.Body.String(), "old")
	require.NotContains(t, w.Body.String(), "new")
	r.URL.RawQuery = "cursor=invalid!"
	w = httptest.NewRecorder()
	h.Messages(w, r)
	require.Equal(t, 422, w.Code)
}

func TestParticipantUsernamesNormalizeAndPreservePositions(t *testing.T) {
	names, err := parseUsernames(" @Daniel_k,\nDANIEL_K; anya_1 ")
	require.NoError(t, err)
	require.Equal(t, []string{"daniel_k", "daniel_k", "anya_1"}, names)
	_, err = parseUsernames("invalid-name")
	require.Error(t, err)
	_, err = parseUsernames(strings.Repeat("daniel_k,", 101))
	require.Error(t, err)
}
