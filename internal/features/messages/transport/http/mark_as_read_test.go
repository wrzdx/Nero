package messages_transport_http

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	messages_service "github.com/wrzdx/Nero/internal/features/messages/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMarkAsRead(t *testing.T) {
	currentUserID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	messageID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	command := messages_service.MarkAsReadCommand{
		ChatID:    chatID,
		UserID:    currentUserID,
		MessageID: messageID,
	}

	t.Run("returns no content through registered route", func(t *testing.T) {
		service := NewMockMessagesService(t)
		service.EXPECT().MarkAsRead(mock.Anything, command).Return(nil)
		router := newMessagesTransportRouter(service, currentUserID)
		request := newMarkAsReadRequest(chatID.String(), `{"message_id":"`+messageID.String()+`"}`)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.Empty(t, recorder.Body.Bytes())
	})

	for _, testCase := range []struct {
		name     string
		chatID   string
		body     string
		expected map[string]string
	}{
		{
			name:     "rejects malformed chat id",
			chatID:   "not-a-uuid",
			body:     `{"message_id":"` + messageID.String() + `"}`,
			expected: map[string]string{"chat_id": "invalid uuid"},
		},
		{
			name:     "rejects malformed message id",
			chatID:   chatID.String(),
			body:     `{"message_id":"not-a-uuid"}`,
			expected: map[string]string{"message_id": "invalid uuid"},
		},
		{
			name:     "rejects missing message id",
			chatID:   chatID.String(),
			body:     `{}`,
			expected: map[string]string{"message_id": "message_id is required"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			router := newMessagesTransportRouter(NewMockMessagesService(t), currentUserID)
			request := newMarkAsReadRequest(testCase.chatID, testCase.body)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, http_response.APIErrorDetail{
				Code:    "invalid_request",
				Message: "invalid request",
				Fields:  testCase.expected,
			}, decodeMessagesTransportError(t, recorder))
		})
	}

	t.Run("rejects malformed json", func(t *testing.T) {
		router := newMessagesTransportRouter(NewMockMessagesService(t), currentUserID)
		request := newMarkAsReadRequest(chatID.String(), `{"message_id":`)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, "invalid_request", decodeMessagesTransportError(t, recorder).Code)
	})

	for _, testCase := range []struct {
		name       string
		serviceErr error
		status     int
		expected   http_response.APIErrorDetail
	}{
		{
			name:       "returns not found",
			serviceErr: domain.ErrNotFound,
			status:     http.StatusNotFound,
			expected: http_response.APIErrorDetail{
				Code:    "not_found",
				Message: "not found",
			},
		},
		{
			name:       "does not expose unexpected service error",
			serviceErr: errors.New("database unavailable"),
			status:     http.StatusInternalServerError,
			expected: http_response.APIErrorDetail{
				Code:    "internal_error",
				Message: "internal server error",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := NewMockMessagesService(t)
			service.EXPECT().MarkAsRead(mock.Anything, command).Return(testCase.serviceErr)
			router := newMessagesTransportRouter(service, currentUserID)
			request := newMarkAsReadRequest(
				chatID.String(),
				`{"message_id":"`+messageID.String()+`"}`,
			)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			require.Equal(t, testCase.status, recorder.Code)
			require.Equal(t, testCase.expected, decodeMessagesTransportError(t, recorder))
			if testCase.status == http.StatusInternalServerError {
				require.NotContains(t, recorder.Body.String(), testCase.serviceErr.Error())
			}
		})
	}
}

func newMarkAsReadRequest(chatID, body string) *http.Request {
	request := httptest.NewRequest(
		http.MethodPut,
		"/chats/"+chatID+"/messages/read",
		bytes.NewBufferString(body),
	)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}
