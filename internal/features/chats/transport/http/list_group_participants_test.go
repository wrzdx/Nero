package chats_transport_http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_cursor "github.com/wrzdx/Nero/internal/core/transport/http/cursor"
	http_response "github.com/wrzdx/Nero/internal/core/transport/http/response"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestListGroupParticipants(t *testing.T) {
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	chatID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	joinedAt := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)

	t.Run("returns participant page", func(t *testing.T) {
		lastName := "Participant"
		participants := []chats_service.ParticipantInfo{
			{
				ID:        uuid.MustParse("00000000-0000-0000-0000-000000000003"),
				Username:  "owner_user",
				FirstName: "Owner",
				LastName:  &lastName,
				Role:      string(domain.OwnerRole),
				JoinedAt:  joinedAt,
			},
			{
				ID:        uuid.MustParse("00000000-0000-0000-0000-000000000004"),
				Username:  "deleted_0000000000000000",
				FirstName: "Deleted Account",
				Role:      string(domain.MemberRole),
				JoinedAt:  joinedAt.Add(-time.Minute),
			},
		}
		before := &chats_service.GroupParticipantCursor{
			ParticipantID: uuid.MustParse("00000000-0000-0000-0000-000000000005"),
			JoinedAt:      joinedAt.Add(time.Hour),
		}
		next := &chats_service.GroupParticipantCursor{
			ParticipantID: participants[1].ID,
			JoinedAt:      participants[1].JoinedAt,
		}
		service := NewMockChatsService(t)
		service.EXPECT().ListGroupParticipants(
			mock.Anything,
			requesterID,
			chats_service.ListGroupParticipantsQuery{
				ChatID: chatID,
				Before: before,
				Limit:  2,
			},
		).Return(chats_service.GroupParticipantPage{
			Participants: participants,
			NextCursor:   next,
		}, nil)
		encodedBefore, err := http_cursor.Encode(&groupParticipantCursorPayload{
			ParticipantID: before.ParticipantID.String(),
			JoinedAt:      before.JoinedAt,
		})
		require.NoError(t, err)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(
			recorder,
			newListGroupParticipantsRequest(
				t,
				chatID.String(),
				"?limit=2&cursor="+url.QueryEscape(*encodedBefore),
			),
		)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		response := decodeListGroupParticipantsResponse(t, recorder)
		require.Len(t, response.Participants, 2)
		for index, expected := range participants {
			actual := response.Participants[index]
			require.Equal(t, expected.ID, actual.UserID)
			require.Equal(t, expected.Username, actual.Username)
			require.Equal(t, expected.FirstName, actual.FirstName)
			require.Equal(t, expected.LastName, actual.LastName)
			require.Equal(t, expected.Role, actual.Role)
			require.True(t, expected.JoinedAt.Equal(actual.JoinedAt))
		}
		require.NotNil(t, response.NextCursor)
		decodedNext, err := http_cursor.DecodeAndValidate[groupParticipantCursorPayload](
			*response.NextCursor,
		)
		require.NoError(t, err)
		require.Equal(t, next.ParticipantID.String(), decodedNext.ParticipantID)
		require.True(t, next.JoinedAt.Equal(decodedNext.JoinedAt))
	})

	t.Run("returns non nil empty final page", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().ListGroupParticipants(
			mock.Anything,
			requesterID,
			chats_service.ListGroupParticipantsQuery{ChatID: chatID},
		).Return(chats_service.GroupParticipantPage{
			Participants: []chats_service.ParticipantInfo{},
		}, nil)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(
			recorder,
			newListGroupParticipantsRequest(t, chatID.String(), ""),
		)

		require.Equal(t, http.StatusOK, recorder.Code)
		response := decodeListGroupParticipantsResponse(t, recorder)
		require.NotNil(t, response.Participants)
		require.Empty(t, response.Participants)
		require.Nil(t, response.NextCursor)
	})

	invalidCursor, err := http_cursor.Encode(&groupParticipantCursorPayload{
		ParticipantID: "not-a-uuid",
		JoinedAt:      joinedAt,
	})
	require.NoError(t, err)
	testCases := []struct {
		name     string
		chatID   string
		query    string
		response http_response.APIErrorDetail
	}{
		{
			name:   "malformed chat id",
			chatID: "not-a-uuid",
			response: http_response.APIErrorDetail{
				Code:    "invalid_request",
				Message: "invalid request",
				Fields: map[string]string{
					"chat_id": "invalid uuid",
				},
			},
		},
		{
			name:   "malformed limit",
			chatID: chatID.String(),
			query:  "?limit=many",
			response: http_response.APIErrorDetail{
				Code:    "invalid_request",
				Message: "invalid request",
				Fields: map[string]string{
					"limit": "invalid limit query param",
				},
			},
		},
		{
			name:   "malformed cursor",
			chatID: chatID.String(),
			query:  "?cursor=%25%25%25",
			response: http_response.APIErrorDetail{
				Code:    "invalid_request",
				Message: "invalid request",
			},
		},
		{
			name:   "invalid cursor payload",
			chatID: chatID.String(),
			query:  "?cursor=" + url.QueryEscape(*invalidCursor),
			response: http_response.APIErrorDetail{
				Code:    "invalid_request",
				Message: "invalid request",
				Fields: map[string]string{
					"cursor": "invalid cursor",
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			router := newListChatsTransportRouter(NewMockChatsService(t), requesterID)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(
				recorder,
				newListGroupParticipantsRequest(t, testCase.chatID, testCase.query),
			)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, testCase.response, decodeChatsTransportError(t, recorder))
		})
	}

	t.Run("maps invalid service query", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().ListGroupParticipants(
			mock.Anything,
			requesterID,
			chats_service.ListGroupParticipantsQuery{ChatID: chatID, Limit: -1},
		).Return(chats_service.GroupParticipantPage{}, domain.DetailedError{
			Err: chats_service.ErrInvalidInput,
			Details: map[string]string{
				"limit": "limit must be between 1 and 100",
			},
		})
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(
			recorder,
			newListGroupParticipantsRequest(t, chatID.String(), "?limit=-1"),
		)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "invalid_input",
			Message: "invalid input",
			Fields: map[string]string{
				"limit": "limit must be between 1 and 100",
			},
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("returns not found", func(t *testing.T) {
		service := NewMockChatsService(t)
		service.EXPECT().ListGroupParticipants(
			mock.Anything,
			requesterID,
			chats_service.ListGroupParticipantsQuery{ChatID: chatID},
		).Return(chats_service.GroupParticipantPage{}, domain.ErrNotFound)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(
			recorder,
			newListGroupParticipantsRequest(t, chatID.String(), ""),
		)

		require.Equal(t, http.StatusNotFound, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "not_found",
			Message: "not found",
		}, decodeChatsTransportError(t, recorder))
	})

	t.Run("does not expose unexpected service error", func(t *testing.T) {
		serviceErr := errors.New("database unavailable")
		service := NewMockChatsService(t)
		service.EXPECT().ListGroupParticipants(
			mock.Anything,
			requesterID,
			chats_service.ListGroupParticipantsQuery{ChatID: chatID},
		).Return(chats_service.GroupParticipantPage{}, serviceErr)
		router := newListChatsTransportRouter(service, requesterID)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(
			recorder,
			newListGroupParticipantsRequest(t, chatID.String(), ""),
		)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Equal(t, http_response.APIErrorDetail{
			Code:    "internal_error",
			Message: "internal server error",
		}, decodeChatsTransportError(t, recorder))
		require.NotContains(t, recorder.Body.String(), serviceErr.Error())
	})
}

func newListGroupParticipantsRequest(
	t *testing.T,
	chatID string,
	query string,
) *http.Request {
	t.Helper()

	request := httptest.NewRequest(
		http.MethodGet,
		"/chats/groups/"+chatID+"/participants"+query,
		nil,
	)
	ctx := logger.WithLogger(request.Context(), logger.NewTestLogger())
	return request.WithContext(ctx)
}

func decodeListGroupParticipantsResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) ListGroupParticipantsResponse {
	t.Helper()

	var body struct {
		Data ListGroupParticipantsResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Data
}
