package chats_transport_http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_cursor "github.com/wrzdx/Nero/internal/core/transport/http/cursor"
	http_middleware "github.com/wrzdx/Nero/internal/core/transport/http/middleware"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestListChats(t *testing.T) {
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	peerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	activityAt := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)

	t.Run("returns direct and group chat summaries", func(t *testing.T) {
		direct := newListChatsTransportDirectItem(t, requesterID, peerID, activityAt)
		group := newListChatsTransportGroupItem(t, requesterID, activityAt.Add(-time.Minute))
		lastReadMessageID := uuid.New()
		group.LastReadMessageID = &lastReadMessageID
		group.UnreadCount = 3
		before := &chats_service.ChatCursor{
			ChatID:         uuid.New(),
			LastActivityAt: activityAt.Add(time.Hour),
		}
		next := &chats_service.ChatCursor{
			ChatID:         group.Chat.ID,
			LastActivityAt: group.Chat.LastActivityAt,
		}
		service := NewMockChatsService(t)
		service.EXPECT().ListChats(
			mock.Anything,
			requesterID,
			chats_service.ListChatsQuery{Before: before, Limit: 2},
		).Return(chats_service.ChatPage{
			Chats:      []chats_service.ChatItem{direct, group},
			NextCursor: next,
		}, nil)
		router := newListChatsTransportRouter(service, requesterID)
		encodedBefore, err := http_cursor.Encode(&chatCursorPayload{
			ChatID:         before.ChatID.String(),
			LastActivityAt: before.LastActivityAt,
		})
		require.NoError(t, err)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(
			recorder,
			newListChatsHTTPRequest(t, "?limit=2&cursor="+url.QueryEscape(*encodedBefore)),
		)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		response := decodeListChatsHTTPResponse(t, recorder)
		require.Len(t, response.Chats, 2)

		directResponse := response.Chats[0]
		require.Equal(t, direct.Chat.ID, directResponse.Chat.ID)
		require.NotNil(t, directResponse.Direct)
		require.Nil(t, directResponse.Group)
		require.Equal(t, peerID, directResponse.Direct.Peer.ID)
		require.Equal(t, direct.DirectPeer.Username, directResponse.Direct.Peer.Username)
		require.Equal(t, direct.DirectPeer.FirstName, directResponse.Direct.Peer.FirstName)
		require.Nil(t, directResponse.LastMessage)

		groupResponse := response.Chats[1]
		require.Equal(t, group.Chat.ID, groupResponse.Chat.ID)
		require.Nil(t, groupResponse.Direct)
		require.NotNil(t, groupResponse.Group)
		require.Equal(t, group.GroupInfo.Title, groupResponse.Group.Title)
		require.NotNil(t, groupResponse.LastMessage)
		require.Equal(t, group.LastMessage.Message.ID, groupResponse.LastMessage.ID)
		require.Equal(
			t,
			group.LastMessage.SenderFirstName,
			groupResponse.LastMessage.SenderFirstName,
		)
		require.Equal(t, group.LastReadMessageID, groupResponse.LastReadMessageID)
		require.Equal(t, group.UnreadCount, groupResponse.UnreadCount)
		require.Nil(t, directResponse.LastReadMessageID)
		require.Zero(t, directResponse.UnreadCount)

		require.NotNil(t, response.NextCursor)
		decodedNext, err := http_cursor.DecodeAndValidate[chatCursorPayload](
			*response.NextCursor,
		)
		require.NoError(t, err)
		require.Equal(t, next.ChatID.String(), decodedNext.ChatID)
		require.True(t, next.LastActivityAt.Equal(decodedNext.LastActivityAt))
	})

	t.Run("returns non nil empty final page", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().ListChats(
			mock.Anything,
			requesterID,
			chats_service.ListChatsQuery{},
		).Return(chats_service.ChatPage{Chats: []chats_service.ChatItem{}}, nil)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newListChatsHTTPRequest(t, ""))

		require.Equal(t, http.StatusOK, recorder.Code)
		response := decodeListChatsHTTPResponse(t, recorder)
		require.NotNil(t, response.Chats)
		require.Empty(t, response.Chats)
		require.Nil(t, response.NextCursor)
	})

	testCases := []struct {
		name   string
		query  string
		fields map[string]string
	}{
		{name: "malformed cursor", query: "?cursor=%25%25%25"},
		{
			name:  "malformed limit",
			query: "?limit=many",
			fields: map[string]string{
				"limit": "invalid limit query param",
			},
		},
	}
	for _, testCase := range testCases {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			router := newListChatsTransportRouter(NewMockChatsService(t), requesterID)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, newListChatsHTTPRequest(t, testCase.query))

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, http_response.APIErrorDetail{
				Code:    "invalid_request",
				Message: "invalid request",
				Fields:  testCase.fields,
			}, decodeChatsTransportError(t, recorder))
		})
	}

	t.Run("maps invalid service query", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().ListChats(
			mock.Anything,
			requesterID,
			chats_service.ListChatsQuery{Limit: -1},
		).Return(chats_service.ChatPage{}, domain.DetailedError{
			Err: chats_service.ErrInvalidInput,
			Details: map[string]string{
				"limit": "limit must be between 1 and 100",
			},
		})
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newListChatsHTTPRequest(t, "?limit=-1"))

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_input",
			Message: "invalid input",
			Fields: map[string]string{
				"limit": "limit must be between 1 and 100",
			},
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		serviceErr := errors.New("database unavailable")
		service := NewMockChatsService(t)
		service.EXPECT().ListChats(
			mock.Anything,
			requesterID,
			chats_service.ListChatsQuery{},
		).Return(chats_service.ChatPage{}, serviceErr)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newListChatsHTTPRequest(t, ""))

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeChatsTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}

func newListChatsTransportRouter(
	service ChatsService,
	currentUserID uuid.UUID,
) http.Handler {
	authMiddleware := http_middleware.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := core_context.WithClaims(
				r.Context(),
				core_context.ContextClaims{UserID: currentUserID},
			)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router := chi.NewRouter()
	router.Mount("/chats", NewChatsHandler(service).Router(authMiddleware))
	return router
}

func newListChatsHTTPRequest(t *testing.T, query string) *http.Request {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/chats/"+query, nil)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}

func decodeListChatsHTTPResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) ListChatsResponse {
	t.Helper()

	var body struct {
		Data ListChatsResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Data
}

func newListChatsTransportDirectItem(
	t *testing.T,
	requesterID uuid.UUID,
	peerID uuid.UUID,
	activityAt time.Time,
) chats_service.ChatItem {
	t.Helper()

	direct, err := domain.NewDirectChat(uuid.New(), requesterID, peerID, activityAt)
	require.NoError(t, err)
	peerProfile, err := domain.NewUserProfile("Peer_123", "Peer", nil, nil)
	require.NoError(t, err)
	peerUser, err := domain.NewUser(
		peerID,
		peerProfile,
		activityAt.Add(-24*time.Hour),
		nil,
		"password_hash",
	)
	require.NoError(t, err)
	peer, err := chats_service.PeerFromUser(peerUser)
	require.NoError(t, err)
	item := chats_service.ChatItem{Chat: direct.Chat, DirectPeer: &peer}
	require.NoError(t, item.Validate())
	return item
}

func newListChatsTransportGroupItem(
	t *testing.T,
	senderID uuid.UUID,
	activityAt time.Time,
) chats_service.ChatItem {
	t.Helper()

	group, err := domain.NewGroupChat(uuid.New(), "Backend", activityAt.Add(-time.Hour))
	require.NoError(t, err)
	groupInfo, err := chats_service.GroupInfoFromGroup(group)
	require.NoError(t, err)
	message, err := domain.NewMessage(
		uuid.New(), uuid.New(), group.Chat.ID, senderID, "group message", activityAt,
	)
	require.NoError(t, err)
	group.Chat.LastMessageID = &message.ID
	group.Chat.LastActivityAt = message.CreatedAt
	senderProfile, err := domain.NewUserProfile("Author_123", "Author", nil, nil)
	require.NoError(t, err)
	lastMessage, err := chats_service.NewLastMessageItem(message, senderProfile)
	require.NoError(t, err)
	item := chats_service.ChatItem{
		Chat:        group.Chat,
		GroupInfo:   &groupInfo,
		LastMessage: &lastMessage,
	}
	require.NoError(t, item.Validate())
	return item
}
