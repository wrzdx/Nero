package chats_transport_http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRemoveGroupParticipant(t *testing.T) {
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	groupID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	targetID := "target_user"
	command := chats_service.RemoveGroupParticipantByUsernameCommand{
		GroupID: groupID, RequesterID: requesterID, TargetUsername: targetID,
	}

	t.Run("removes participant through registered route", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().RemoveGroupParticipantByUsername(mock.Anything, command).Return(nil)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newRemoveGroupParticipantHTTPRequest(
			t,
			groupID.String(),
			map[string]any{"target_username": targetID},
		))

		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.Empty(t, recorder.Body.String())
	})

	t.Run("rejects malformed chat id", func(t *testing.T) {
		router := newListChatsTransportRouter(NewMockChatsService(t), requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newRemoveGroupParticipantHTTPRequest(
			t,
			"not-a-uuid",
			map[string]any{"target_username": targetID},
		))

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code: "invalid_request", Message: "invalid request",
			Fields: map[string]string{"chat_id": "invalid uuid"},
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("rejects malformed target id", func(t *testing.T) {
		router := newListChatsTransportRouter(NewMockChatsService(t), requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newRemoveGroupParticipantHTTPRequest(
			t,
			groupID.String(),
			map[string]any{"target_username": "not-a-uuid"},
		))

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code: "invalid_request", Message: "invalid request",
			Fields: map[string]string{"target_username": "must contain 5–32 ASCII letters, digits or underscores"},
		}, decodeChatsTransportError(t, recorder))
	})

	errorCases := []struct {
		name       string
		serviceErr error
		status     int
		response   http_response.APIErrorDetail
	}{
		{
			name: "maps invalid input", serviceErr: chats_service.ErrInvalidInput,
			status:   http.StatusBadRequest,
			response: http_response.APIErrorDetail{Code: "invalid_input", Message: "invalid input"},
		},
		{
			name: "maps insufficient rights", serviceErr: chats_service.ErrNotEnoughRights,
			status:   http.StatusForbidden,
			response: http_response.APIErrorDetail{Code: "not_enough_rights", Message: "not enough rights"},
		},
		{
			name: "maps owner leave restriction", serviceErr: chats_service.ErrOwnerCannotQuitGroup,
			status: http.StatusForbidden,
			response: http_response.APIErrorDetail{
				Code: "owner_cannot_quit_group", Message: "owner cannot quit group",
			},
		},
		{
			name: "maps missing participant", serviceErr: domain.ErrNotFound,
			status:   http.StatusNotFound,
			response: http_response.APIErrorDetail{Code: "not_found", Message: "not found"},
		},
	}
	for _, testCase := range errorCases {
		t.Run(testCase.name, func(t *testing.T) {
			service := NewMockChatsService(t)
			service.EXPECT().RemoveGroupParticipantByUsername(mock.Anything, command).Return(testCase.serviceErr)
			router := newListChatsTransportRouter(service, requesterID)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, newRemoveGroupParticipantHTTPRequest(
				t,
				groupID.String(),
				map[string]any{"target_username": targetID},
			))

			require.Equal(t, testCase.status, recorder.Code)
			require.Equal(t, testCase.response, decodeChatsTransportError(t, recorder))
		})
	}

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		serviceErr := errors.New("database unavailable")
		service := NewMockChatsService(t)
		service.EXPECT().RemoveGroupParticipantByUsername(mock.Anything, command).Return(serviceErr)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, newRemoveGroupParticipantHTTPRequest(
			t,
			groupID.String(),
			map[string]any{"target_username": targetID},
		))

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code: "internal_error", Message: "internal server error",
		}, decodeChatsTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}

func newRemoveGroupParticipantHTTPRequest(
	t *testing.T,
	groupID string,
	body any,
) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(
		http.MethodDelete,
		"/chats/groups/"+groupID+"/participants",
		bytes.NewReader(payload),
	)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}
