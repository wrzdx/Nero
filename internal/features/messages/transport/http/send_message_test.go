package messages_transport_http

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
	http_middleware "github.com/wrzdx/Nero/internal/core/transport/http/middleware"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	messages_service "github.com/wrzdx/Nero/internal/features/messages/service"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSendMessage(t *testing.T) {
	currentUserID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	clientMessageID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	command := messages_service.SendMessageCommand{
		SenderID:        currentUserID,
		ChatID:          chatID,
		ClientMessageID: clientMessageID,
		Content:         "Hello",
	}

	t.Run("returns created message", func(t *testing.T) {
		message := newMessagesTransportMessage(t, currentUserID, command)
		service := NewMockMessagesService(t)
		service.EXPECT().
			SendMessage(mock.Anything, command).
			Return(message, true, nil)
		router := newMessagesTransportRouter(service, currentUserID)
		request := newSendMessageRequest(t, chatID.String(), map[string]any{
			"client_message_id": clientMessageID,
			"content":           command.Content,
		})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusCreated, recorder.Code)
		require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		requireMessagesTransportResponseEqual(t, message, decodeSendMessageResponse(t, recorder))
	})

	t.Run("returns existing message with OK status", func(t *testing.T) {
		message := newMessagesTransportMessage(t, currentUserID, command)
		service := NewMockMessagesService(t)
		service.EXPECT().
			SendMessage(mock.Anything, command).
			Return(message, false, nil)
		router := newMessagesTransportRouter(service, currentUserID)
		request := newSendMessageRequest(t, chatID.String(), map[string]any{
			"client_message_id": clientMessageID,
			"content":           command.Content,
		})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		requireMessagesTransportResponseEqual(t, message, decodeSendMessageResponse(t, recorder))
	})

	t.Run("rejects malformed chat id without calling service", func(t *testing.T) {
		router := newMessagesTransportRouter(NewMockMessagesService(t), currentUserID)
		request := newSendMessageRequest(t, "not-a-uuid", map[string]any{
			"client_message_id": clientMessageID,
			"content":           command.Content,
		})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"chat_id": "invalid uuid",
			},
		}, decodeMessagesTransportError(t, recorder))
	})

	t.Run("rejects missing request fields without calling service", func(t *testing.T) {
		router := newMessagesTransportRouter(NewMockMessagesService(t), currentUserID)
		request := newSendMessageRequest(t, chatID.String(), map[string]any{})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"client_message_id": "client_message_id is required",
				"content":           "content is required",
			},
		}, decodeMessagesTransportError(t, recorder))
	})

	t.Run("rejects malformed json without calling service", func(t *testing.T) {
		router := newMessagesTransportRouter(NewMockMessagesService(t), currentUserID)
		request := newRawSendMessageRequest(t, chatID.String(), []byte(`{"content":`))
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
		}, decodeMessagesTransportError(t, recorder))
	})

	testCases := []struct {
		name       string
		serviceErr error
		status     int
		code       string
		message    string
	}{
		{
			name:       "returns not found",
			serviceErr: domain.ErrNotFound,
			status:     http.StatusNotFound,
			code:       "not_found",
			message:    "not found",
		},
		{
			name:       "returns idempotency conflict",
			serviceErr: messages_service.ErrMessageConflict,
			status:     http.StatusConflict,
			code:       "message_conflict",
			message:    "message conflict",
		},
		{
			name:       "returns unavailable target",
			serviceErr: messages_service.ErrMessageTargetUnavailable,
			status:     http.StatusConflict,
			code:       "message_target_unavailable",
			message:    "message target unavailable",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			service := NewMockMessagesService(t)
			service.EXPECT().
				SendMessage(mock.Anything, command).
				Return(domain.Message{}, false, testCase.serviceErr)
			router := newMessagesTransportRouter(service, currentUserID)
			request := newSendMessageRequest(t, chatID.String(), map[string]any{
				"client_message_id": clientMessageID,
				"content":           command.Content,
			})
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			require.Equal(t, testCase.status, recorder.Code)
			require.Equal(t, http_response.APIErrorDetail{
				Code:    testCase.code,
				Message: testCase.message,
			}, decodeMessagesTransportError(t, recorder))
		})
	}

	t.Run("returns detailed invalid message error", func(t *testing.T) {
		service := NewMockMessagesService(t)
		service.EXPECT().
			SendMessage(mock.Anything, command).
			Return(domain.Message{}, false, domain.DetailedError{
				Err: domain.ErrInvalidMessage,
				Details: map[string]string{
					"content": "content must contain between 1 and 4096 characters",
				},
			})
		router := newMessagesTransportRouter(service, currentUserID)
		request := newSendMessageRequest(t, chatID.String(), map[string]any{
			"client_message_id": clientMessageID,
			"content":           command.Content,
		})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_message",
			Message: "invalid message",
			Fields: map[string]string{
				"content": "content must contain between 1 and 4096 characters",
			},
		}, decodeMessagesTransportError(t, recorder))
	})

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		serviceErr := errors.New("database unavailable")
		service := NewMockMessagesService(t)
		service.EXPECT().
			SendMessage(mock.Anything, command).
			Return(domain.Message{}, false, serviceErr)
		router := newMessagesTransportRouter(service, currentUserID)
		request := newSendMessageRequest(t, chatID.String(), map[string]any{
			"client_message_id": clientMessageID,
			"content":           command.Content,
		})
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeMessagesTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}

func newMessagesTransportRouter(
	service MessagesService,
	currentUserID uuid.UUID,
) http.Handler {
	authMW := http_middleware.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := core_context.WithClaims(
				r.Context(),
				core_context.ContextClaims{UserID: currentUserID},
			)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router := chi.NewRouter()
	router.Mount(
		"/chats/{chat_id}/messages",
		NewMessagesHandler(service).Router(authMW),
	)
	return router
}

func newSendMessageRequest(
	t *testing.T,
	chatID string,
	body any,
) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	return newRawSendMessageRequest(t, chatID, payload)
}

func newRawSendMessageRequest(
	t *testing.T,
	chatID string,
	body []byte,
) *http.Request {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/chats/"+chatID+"/messages",
		bytes.NewReader(body),
	)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}

func newMessagesTransportMessage(
	t *testing.T,
	senderID uuid.UUID,
	command messages_service.SendMessageCommand,
) domain.Message {
	t.Helper()
	message, err := domain.NewMessage(
		uuid.New(),
		command.ClientMessageID,
		command.ChatID,
		senderID,
		command.Content,
		time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	updatedAt := message.CreatedAt.Add(time.Minute)
	message.UpdatedAt = &updatedAt
	require.NoError(t, message.Validate())
	return message
}

func decodeSendMessageResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) SendMessageResponse {
	t.Helper()
	var body struct {
		Data SendMessageResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Data
}

func decodeMessagesTransportError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) http_response.APIErrorDetail {
	t.Helper()
	var body struct {
		Error http_response.APIErrorDetail `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Error
}

func requireMessagesTransportResponseEqual(
	t *testing.T,
	expected domain.Message,
	actual SendMessageResponse,
) {
	t.Helper()
	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.ClientMessageID, actual.ClientMessageID)
	require.Equal(t, expected.ChatID, actual.ChatID)
	require.Equal(t, expected.SenderID, actual.SenderID)
	require.Equal(t, expected.Content, actual.Content)
	require.True(t, expected.CreatedAt.Equal(actual.CreatedAt))
	require.NotNil(t, actual.UpdatedAt)
	require.True(t, expected.UpdatedAt.Equal(*actual.UpdatedAt))
}
