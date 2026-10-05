package messages_transport_http

import (
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

func TestDeleteMessage(t *testing.T) {
	currentUserID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	messageID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	command := messages_service.DeleteMessageCommand{
		ChatID:    chatID,
		SenderID:  currentUserID,
		MessageID: messageID,
	}

	t.Run("returns no content", func(t *testing.T) {
		service := NewMockMessagesService(t)
		service.EXPECT().DeleteMessage(mock.Anything, command).Return(nil)
		router := newMessagesTransportRouter(service, currentUserID)
		request := newDeleteMessageRequest(t, chatID.String(), messageID.String())
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.Empty(t, recorder.Body.Bytes())
	})

	for _, testCase := range []struct {
		name      string
		chatID    string
		messageID string
		fields    map[string]string
	}{
		{
			name:      "rejects malformed chat id",
			chatID:    "not-a-uuid",
			messageID: messageID.String(),
			fields:    map[string]string{"chat_id": "invalid uuid"},
		},
		{
			name:      "rejects malformed message id",
			chatID:    chatID.String(),
			messageID: "not-a-uuid",
			fields:    map[string]string{"message_id": "invalid uuid"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			router := newMessagesTransportRouter(NewMockMessagesService(t), currentUserID)
			request := newDeleteMessageRequest(t, testCase.chatID, testCase.messageID)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, http_response.APIErrorDetail{
				Code:    "invalid_request",
				Message: "invalid request",
				Fields:  testCase.fields,
			}, decodeMessagesTransportError(t, recorder))
		})
	}

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
			name: "returns detailed invalid input",
			serviceErr: domain.DetailedError{
				Err: messages_service.ErrInvalidInput,
				Details: map[string]string{
					"message_id": "message id is nil",
				},
			},
			status: http.StatusBadRequest,
			expected: http_response.APIErrorDetail{
				Code:    "invalid_input",
				Message: "invalid input",
				Fields: map[string]string{
					"message_id": "message id is nil",
				},
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
			service.EXPECT().DeleteMessage(mock.Anything, command).Return(testCase.serviceErr)
			router := newMessagesTransportRouter(service, currentUserID)
			request := newDeleteMessageRequest(t, chatID.String(), messageID.String())
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

func newDeleteMessageRequest(
	t *testing.T,
	chatID, messageID string,
) *http.Request {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodDelete,
		"/chats/"+chatID+"/messages/"+messageID,
		nil,
	)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}
