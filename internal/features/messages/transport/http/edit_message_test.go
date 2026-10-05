package messages_transport_http

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
	messages_service "github.com/wrzdx/Nero/internal/features/messages/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestEditMessage(t *testing.T) {
	currentUserID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	messageID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	command := messages_service.UpdateMessageCommand{
		SenderID:  currentUserID,
		ChatID:    chatID,
		MessageID: messageID,
		Content:   "new content",
	}
	updated := newEditMessageTransportMessage(t, command)

	t.Run("returns updated message", func(t *testing.T) {
		service := NewMockMessagesService(t)
		service.EXPECT().EditMessage(mock.Anything, command).Return(updated, nil)
		router := newMessagesTransportRouter(service, currentUserID)
		request := newEditMessageRequest(t, chatID.String(), messageID.String(), map[string]any{
			"content": command.Content,
		})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		requireMessagesTransportResponseEqual(
			t,
			updated,
			SendMessageResponse(decodeEditMessageResponse(t, recorder)),
		)
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
			request := newEditMessageRequest(t, testCase.chatID, testCase.messageID, map[string]any{
				"content": command.Content,
			})
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

	t.Run("rejects missing content", func(t *testing.T) {
		router := newMessagesTransportRouter(NewMockMessagesService(t), currentUserID)
		request := newEditMessageRequest(t, chatID.String(), messageID.String(), map[string]any{})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields:  map[string]string{"content": "content is required"},
		}, decodeMessagesTransportError(t, recorder))
	})

	t.Run("rejects malformed json", func(t *testing.T) {
		router := newMessagesTransportRouter(NewMockMessagesService(t), currentUserID)
		request := newRawEditMessageRequest(
			t,
			chatID.String(),
			messageID.String(),
			[]byte(`{"content":`),
		)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
		}, decodeMessagesTransportError(t, recorder))
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
			name:   "returns detailed invalid message",
			status: http.StatusBadRequest,
			serviceErr: domain.DetailedError{
				Err: domain.ErrInvalidMessage,
				Details: map[string]string{
					"content": "content must contain between 1 and 4096 characters",
				},
			},
			expected: http_response.APIErrorDetail{
				Code:    "invalid_message",
				Message: "invalid message",
				Fields: map[string]string{
					"content": "content must contain between 1 and 4096 characters",
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
			service.EXPECT().EditMessage(mock.Anything, command).Return(domain.Message{}, testCase.serviceErr)
			router := newMessagesTransportRouter(service, currentUserID)
			request := newEditMessageRequest(t, chatID.String(), messageID.String(), map[string]any{
				"content": command.Content,
			})
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

func newEditMessageRequest(
	t *testing.T,
	chatID, messageID string,
	body any,
) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	return newRawEditMessageRequest(t, chatID, messageID, payload)
}

func newRawEditMessageRequest(
	t *testing.T,
	chatID, messageID string,
	body []byte,
) *http.Request {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPatch,
		"/chats/"+chatID+"/messages/"+messageID,
		bytes.NewReader(body),
	)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}

func newEditMessageTransportMessage(
	t *testing.T,
	command messages_service.UpdateMessageCommand,
) domain.Message {
	t.Helper()
	createdAt := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	message, err := domain.NewMessage(
		command.MessageID,
		uuid.New(),
		command.ChatID,
		command.SenderID,
		command.Content,
		createdAt,
	)
	require.NoError(t, err)
	updatedAt := createdAt.Add(time.Minute)
	message.UpdatedAt = &updatedAt
	require.NoError(t, message.Validate())
	return message
}

func decodeEditMessageResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) UpdateMessageResponse {
	t.Helper()
	var body struct {
		Data UpdateMessageResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Data
}
