package chats_transport_http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestUpdateGroup(t *testing.T) {
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	groupID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	updated, err := domain.NewGroupChat(
		groupID,
		"Updated group",
		time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	command := chats_service.UpdateGroupCommand{
		RequesterID: requesterID,
		GroupID:     groupID,
		Title:       updated.Title,
	}

	t.Run("updates group through registered route", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().UpdateGroup(mock.Anything, command).Return(updated, nil)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newUpdateGroupHTTPRequest(
			t,
			groupID.String(),
			map[string]any{"title": updated.Title},
		))

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, UpdateGroupResponse{
			ChatResponse: chatResponseFromDomain(updated.Chat),
			Title:        updated.Title,
		}, decodeUpdateGroupResponse(t, recorder))
	})

	t.Run("rejects missing title", func(t *testing.T) {
		router := newListChatsTransportRouter(NewMockChatsService(t), requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newUpdateGroupHTTPRequest(t, groupID.String(), map[string]any{}))

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields:  map[string]string{"title": "title is required"},
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("rejects malformed chat id", func(t *testing.T) {
		router := newListChatsTransportRouter(NewMockChatsService(t), requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newUpdateGroupHTTPRequest(
			t,
			"not-a-uuid",
			map[string]any{"title": updated.Title},
		))

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields:  map[string]string{"chat_id": "invalid uuid"},
		}, decodeChatsTransportError(t, recorder))
	})

	errorCases := []struct {
		name       string
		serviceErr error
		status     int
		response   http_response.APIErrorDetail
	}{
		{
			name:       "maps invalid group title",
			serviceErr: domain.ErrInvalidGroupChat,
			status:     http.StatusBadRequest,
			response: http_response.APIErrorDetail{
				Code: "invalid_group_chat", Message: "invalid group chat",
			},
		},
		{
			name:       "maps insufficient rights",
			serviceErr: chats_service.ErrNotEnoughRights,
			status:     http.StatusForbidden,
			response: http_response.APIErrorDetail{
				Code: "not_enough_rights", Message: "not enough rights",
			},
		},
		{
			name:       "maps missing group",
			serviceErr: domain.ErrNotFound,
			status:     http.StatusNotFound,
			response:   http_response.APIErrorDetail{Code: "not_found", Message: "not found"},
		},
	}
	for _, testCase := range errorCases {
		t.Run(testCase.name, func(t *testing.T) {
			service := NewMockChatsService(t)
			service.EXPECT().UpdateGroup(mock.Anything, command).Return(domain.GroupChat{}, testCase.serviceErr)
			router := newListChatsTransportRouter(service, requesterID)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, newUpdateGroupHTTPRequest(
				t,
				groupID.String(),
				map[string]any{"title": updated.Title},
			))

			require.Equal(t, testCase.status, recorder.Code)
			require.Equal(t, testCase.response, decodeChatsTransportError(t, recorder))
		})
	}

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		serviceErr := errors.New("database unavailable")
		service := NewMockChatsService(t)
		service.EXPECT().UpdateGroup(mock.Anything, command).Return(domain.GroupChat{}, serviceErr)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newUpdateGroupHTTPRequest(
			t,
			groupID.String(),
			map[string]any{"title": updated.Title},
		))

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code: "internal_error", Message: "internal server error",
		}, decodeChatsTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}

func newUpdateGroupHTTPRequest(t *testing.T, groupID string, body any) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(
		http.MethodPut,
		"/chats/groups/"+groupID,
		bytes.NewReader(payload),
	)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}

func decodeUpdateGroupResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) UpdateGroupResponse {
	t.Helper()
	var body struct {
		Data UpdateGroupResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Data
}
