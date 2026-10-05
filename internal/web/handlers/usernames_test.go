package web_handlers

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/wrzdx/Nero/internal/core/domain"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
)

type usernameChatStub struct {
	chatStub
	direct func(uuid.UUID, string) (domain.DirectChat, bool, error)
	group  func(uuid.UUID, chats_service.CreateGroupByUsernamesCommand) (domain.GroupChat, error)
}

func (s usernameChatStub) CreateDirectByUsername(_ context.Context, id uuid.UUID, name string) (domain.DirectChat, bool, error) {
	return s.direct(id, name)
}
func (s usernameChatStub) CreateGroupByUsernames(_ context.Context, id uuid.UUID, c chats_service.CreateGroupByUsernamesCommand) (domain.GroupChat, error) {
	return s.group(id, c)
}

func TestHTMLCreateChatUsesUsernames(t *testing.T) {
	userID, chatID := uuid.New(), uuid.New()
	for _, group := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "group"}[group], func(t *testing.T) {
			called := false
			chats := usernameChatStub{
				direct: func(id uuid.UUID, name string) (domain.DirectChat, bool, error) {
					called = true
					require.Equal(t, userID, id)
					require.Equal(t, "daniel_k", name)
					return domain.DirectChat{Chat: domain.Chat{ID: chatID}}, true, nil
				},
				group: func(id uuid.UUID, c chats_service.CreateGroupByUsernamesCommand) (domain.GroupChat, error) {
					called = true
					require.Equal(t, userID, id)
					require.Equal(t, "Team", c.Title)
					require.Equal(t, []string{"daniel_k", "anya_1"}, c.ParticipantUsernames)
					return domain.GroupChat{Chat: domain.Chat{ID: chatID}}, nil
				},
			}
			form := url.Values{"type": {"direct"}, "peer_username": {" @Daniel_k "}}
			if group {
				form = url.Values{"type": {"group"}, "title": {"Team"}, "participant_username": {"@Daniel_k", "ANYA_1"}}
			}
			h := NewAppHandler(testHandler(authStub{}), nil, nil, chats, nil, nil)
			w := httptest.NewRecorder()
			h.CreateChat(w, appRequest(formRequest("/app/new", form, true), userID, nil))
			require.True(t, called)
			require.Equal(t, 204, w.Code)
			require.Equal(t, "/app/chats/"+chatID.String(), w.Header().Get("HX-Redirect"))
		})
	}
}

func TestHTMLUnknownUsernameKeepsInputAndShowsUsefulError(t *testing.T) {
	userID := uuid.New()
	h := NewAppHandler(testHandler(authStub{}), nil, userStub{get: func(uuid.UUID) (domain.User, error) {
		return domain.User{ID: userID, Profile: domain.UserProfile{Username: "my_user", FirstName: "Имя"}}, nil
	}}, usernameChatStub{direct: func(uuid.UUID, string) (domain.DirectChat, bool, error) {
		return domain.DirectChat{}, false, domain.ErrNotFound
	}}, nil, nil)
	w := httptest.NewRecorder()
	h.CreateChat(w, appRequest(formRequest("/app/new", url.Values{"type": {"direct"}, "peer_username": {"@Missing_User"}}, true), userID, nil))
	require.Equal(t, 404, w.Code)
	require.Contains(t, w.Body.String(), "Пользователь не найден")
	require.Contains(t, w.Body.String(), "@Missing_User")
	require.Contains(t, w.Body.String(), `name="peer_username"`)
	require.NotContains(t, w.Body.String(), `name="peer_id"`)
}
