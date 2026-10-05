package chats_transport_http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	core_context "github.com/wrzdx/Nero/internal/core/context"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateGroup(t *testing.T) {
	creatorID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	member1ID := "member_one"
	member2ID := "member_two"
	command := chats_service.CreateGroupByUsernamesCommand{
		Title:                "Backend group",
		ParticipantUsernames: []string{member1ID, member2ID},
	}

	t.Run("returns created group", func(t *testing.T) {
		group, err := domain.NewGroupChat(
			uuid.New(),
			command.Title,
			time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC),
		)
		require.NoError(t, err)
		service := NewMockChatsService(t)
		service.EXPECT().
			CreateGroupByUsernames(mock.Anything, creatorID, command).
			Return(group, nil)
		handler := NewChatsHandler(service)
		request := newCreateGroupRequest(t, creatorID, map[string]any{
			"title":                 command.Title,
			"participant_usernames": []string{member1ID, member2ID},
		})
		recorder := httptest.NewRecorder()

		handler.CreateGroup(recorder, request)

		require.Equal(t, http.StatusCreated, recorder.Code)
		require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		response := decodeCreateGroupResponse(t, recorder)
		require.Equal(t, group.Chat.ID, response.ID)
		require.Equal(t, string(domain.ChatTypeGroup), response.Type)
		require.Equal(t, group.Chat.LastMessageID, response.LastMessageID)
		require.True(t, group.Chat.LastActivityAt.Equal(response.LastActivityAt))
		require.True(t, group.Chat.CreatedAt.Equal(response.CreatedAt))
		require.Equal(t, group.Title, response.Title)
		require.NotContains(t, recorder.Body.String(), "participant_usernames")
	})

	t.Run("rejects missing title without calling service", func(t *testing.T) {
		handler := NewChatsHandler(NewMockChatsService(t))
		request := newCreateGroupRequest(t, creatorID, map[string]any{
			"participant_usernames": []string{member1ID},
		})
		recorder := httptest.NewRecorder()

		handler.CreateGroup(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"title": "title is required",
			},
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("rejects malformed participant id without calling service", func(t *testing.T) {
		handler := NewChatsHandler(NewMockChatsService(t))
		request := newCreateGroupRequest(t, creatorID, map[string]any{
			"title":                 command.Title,
			"participant_usernames": []string{member1ID, "not-a-uuid"},
		})
		recorder := httptest.NewRecorder()

		handler.CreateGroup(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"participant_usernames[1]": "must contain 5–32 ASCII letters, digits or underscores",
			},
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("rejects malformed json without calling service", func(t *testing.T) {
		handler := NewChatsHandler(NewMockChatsService(t))
		request := newRawCreateGroupRequest(
			creatorID,
			[]byte(`{"title":`),
		)
		recorder := httptest.NewRecorder()

		handler.CreateGroup(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("returns detailed invalid group error", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().
			CreateGroupByUsernames(mock.Anything, creatorID, command).
			Return(domain.GroupChat{}, domain.DetailedError{
				Err: domain.ErrInvalidGroupChat,
				Details: map[string]string{
					"title": "title must contain between 1 and 128 characters",
				},
			})
		handler := NewChatsHandler(service)
		request := newCreateGroupRequest(t, creatorID, map[string]any{
			"title":                 command.Title,
			"participant_usernames": []string{member1ID, member2ID},
		})
		recorder := httptest.NewRecorder()

		handler.CreateGroup(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_group_chat",
			Message: "invalid group chat",
			Fields: map[string]string{
				"title": "title must contain between 1 and 128 characters",
			},
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		serviceErr := errors.New("database unavailable")
		service := NewMockChatsService(t)
		service.EXPECT().
			CreateGroupByUsernames(mock.Anything, creatorID, command).
			Return(domain.GroupChat{}, serviceErr)
		handler := NewChatsHandler(service)
		request := newCreateGroupRequest(t, creatorID, map[string]any{
			"title":                 command.Title,
			"participant_usernames": []string{member1ID, member2ID},
		})
		recorder := httptest.NewRecorder()

		handler.CreateGroup(recorder, request)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeChatsTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}

func newCreateGroupRequest(
	t *testing.T,
	creatorID uuid.UUID,
	body any,
) *http.Request {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)
	return newRawCreateGroupRequest(creatorID, payload)
}

func newRawCreateGroupRequest(
	creatorID uuid.UUID,
	body []byte,
) *http.Request {
	request := httptest.NewRequest(
		http.MethodPost,
		"/chats/groups",
		bytes.NewReader(body),
	)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	ctx = core_context.WithClaims(ctx, core_context.ContextClaims{UserID: creatorID})
	return request.WithContext(ctx)
}

func decodeCreateGroupResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) CreateGroupResponse {
	t.Helper()

	var body struct {
		Data CreateGroupResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Data
}
