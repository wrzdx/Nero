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

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateDirect(t *testing.T) {
	currentUserID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	peerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	t.Run("returns created direct chat", func(t *testing.T) {
		direct := newChatsTransportDirect(t, currentUserID, peerID)
		service := NewMockChatsService(t)
		service.EXPECT().
			CreateDirectByUsername(mock.Anything, currentUserID, "peer_user").
			Return(direct, true, nil)
		handler := NewChatsHandler(service)
		request := newCreateDirectRequest(t, currentUserID, map[string]any{
			"peer_username": "peer_user",
		})
		recorder := httptest.NewRecorder()

		handler.CreateDirect(recorder, request)

		require.Equal(t, http.StatusCreated, recorder.Code)
		response := decodeCreateDirectResponse(t, recorder)
		require.Equal(t, direct.Chat.ID, response.ID)
		require.Equal(t, direct.Chat.LastMessageID, response.LastMessageID)
		require.True(t, direct.Chat.LastActivityAt.Equal(response.LastActivityAt))
		require.True(t, direct.Chat.CreatedAt.Equal(response.CreatedAt))
		require.NotContains(t, recorder.Body.String(), "peer_username")
	})

	t.Run("returns existing direct chat with OK status", func(t *testing.T) {
		direct := newChatsTransportDirect(t, currentUserID, peerID)
		messageID := uuid.New()
		direct.Chat.LastMessageID = &messageID
		direct.Chat.LastActivityAt = direct.Chat.CreatedAt.Add(time.Minute)
		service := NewMockChatsService(t)
		service.EXPECT().
			CreateDirectByUsername(mock.Anything, currentUserID, "peer_user").
			Return(direct, false, nil)
		handler := NewChatsHandler(service)
		request := newCreateDirectRequest(t, currentUserID, map[string]any{
			"peer_username": "peer_user",
		})
		recorder := httptest.NewRecorder()

		handler.CreateDirect(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		response := decodeCreateDirectResponse(t, recorder)
		require.Equal(t, direct.Chat.ID, response.ID)
		require.Equal(t, direct.Chat.LastMessageID, response.LastMessageID)
		require.True(t, direct.Chat.LastActivityAt.Equal(response.LastActivityAt))
	})

	t.Run("rejects missing peer id without calling service", func(t *testing.T) {
		handler := NewChatsHandler(NewMockChatsService(t))
		request := newCreateDirectRequest(t, currentUserID, map[string]any{})
		recorder := httptest.NewRecorder()

		handler.CreateDirect(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"peer_username": "peer_username is required",
			},
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("rejects malformed peer id without calling service", func(t *testing.T) {
		handler := NewChatsHandler(NewMockChatsService(t))
		request := newCreateDirectRequest(t, currentUserID, map[string]any{
			"peer_username": "not-a-uuid",
		})
		recorder := httptest.NewRecorder()

		handler.CreateDirect(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_request",
			Message: "invalid request",
			Fields: map[string]string{
				"peer_username": "must contain 5–32 ASCII letters, digits or underscores",
			},
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("returns peer not found", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().
			CreateDirectByUsername(mock.Anything, currentUserID, "peer_user").
			Return(domain.DirectChat{}, false, domain.ErrNotFound)
		handler := NewChatsHandler(service)
		request := newCreateDirectRequest(t, currentUserID, map[string]any{
			"peer_username": "peer_user",
		})
		recorder := httptest.NewRecorder()

		handler.CreateDirect(recorder, request)

		require.Equal(t, http.StatusNotFound, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "not_found",
			Message: "not found",
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("returns invalid direct chat", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().
			CreateDirectByUsername(mock.Anything, currentUserID, "current_user").
			Return(domain.DirectChat{}, false, domain.ErrInvalidDirectChat)
		handler := NewChatsHandler(service)
		request := newCreateDirectRequest(t, currentUserID, map[string]any{
			"peer_username": "current_user",
		})
		recorder := httptest.NewRecorder()

		handler.CreateDirect(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_direct_chat",
			Message: "invalid direct chat",
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		serviceErr := errors.New("database unavailable")
		service := NewMockChatsService(t)
		service.EXPECT().
			CreateDirectByUsername(mock.Anything, currentUserID, "peer_user").
			Return(domain.DirectChat{}, false, serviceErr)
		handler := NewChatsHandler(service)
		request := newCreateDirectRequest(t, currentUserID, map[string]any{
			"peer_username": "peer_user",
		})
		recorder := httptest.NewRecorder()

		handler.CreateDirect(recorder, request)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeChatsTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}

func newChatsTransportDirect(
	t *testing.T,
	currentUserID uuid.UUID,
	peerID uuid.UUID,
) domain.DirectChat {
	t.Helper()

	direct, err := domain.NewDirectChat(
		uuid.New(),
		currentUserID,
		peerID,
		time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	return direct
}

func newCreateDirectRequest(
	t *testing.T,
	currentUserID uuid.UUID,
	body any,
) *http.Request {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(
		http.MethodPost,
		"/chats/directs",
		bytes.NewReader(payload),
	)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	ctx = core_context.WithClaims(ctx, core_context.ContextClaims{UserID: currentUserID})
	return request.WithContext(ctx)
}

func decodeCreateDirectResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) CreateDirectResponse {
	t.Helper()

	var body struct {
		Data CreateDirectResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Data
}

func decodeChatsTransportError(
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
