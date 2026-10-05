package chats_transport_http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/wrzdx/Nero/internal/core/logger"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
)

func TestUsernameRequestsRejectObsoleteIDFields(t *testing.T) {
	requesterID, groupID := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"direct", "POST", "/chats/directs", `{"peer_id":"` + uuid.NewString() + `"}`},
		{"group", "POST", "/chats/groups", `{"title":"Team","participant_ids":[]}`},
		{"add", "POST", "/chats/groups/" + groupID.String() + "/participants", `{"participant_ids":[]}`},
		{"remove", "DELETE", "/chats/groups/" + groupID.String() + "/participants", `{"target_id":"` + uuid.NewString() + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := newListChatsTransportRouter(NewMockChatsService(t), requesterID)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r = r.WithContext(logger.WithLogger(r.Context(), logger.NewTestLogger()))
			router.ServeHTTP(w, r)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Equal(t, "invalid_request", decodeChatsTransportError(t, w).Code)
		})
	}
}

func TestAddGroupParticipantRequestNormalizesUsernames(t *testing.T) {
	requesterID, groupID := uuid.New(), uuid.New()
	service := NewMockChatsService(t)
	service.EXPECT().AddGroupParticipantsByUsernames(mock.Anything, chats_service.AddGroupParticipantsByUsernamesCommand{GroupID: groupID, RequesterID: requesterID, ParticipantUsernames: []string{"daniel_k", "daniel_k"}}).Return([]chats_service.UsernameParticipantResult{{Username: "daniel_k", Status: chats_service.Added}, {Username: "daniel_k", Status: chats_service.AlreadyMember}}, nil)
	w := httptest.NewRecorder()
	newListChatsTransportRouter(service, requesterID).ServeHTTP(w, newAddGroupParticipantsHTTPRequest(t, groupID.String(), map[string]any{"participant_usernames": []string{" @Daniel_k ", "DANIEL_K"}}))
	require.Equal(t, 200, w.Code)
	require.Equal(t, AddGroupParticipantsResponse{{Username: "daniel_k", Status: "added"}, {Username: "daniel_k", Status: "already_member"}}, decodeAddGroupParticipantsResponse(t, w))
}

func TestGroupUsernameRequestLimitAndSingleJSONValue(t *testing.T) {
	requesterID := uuid.New()
	names := make([]string, 101)
	for i := range names {
		names[i] = "daniel_k"
	}
	w := httptest.NewRecorder()
	NewChatsHandler(NewMockChatsService(t)).CreateGroup(w, newCreateGroupRequest(t, requesterID, map[string]any{"title": "Team", "participant_usernames": names}))
	require.Equal(t, 400, w.Code)
	require.Contains(t, decodeChatsTransportError(t, w).Fields, "participant_usernames")
	w = httptest.NewRecorder()
	NewChatsHandler(NewMockChatsService(t)).CreateGroup(w, newRawCreateGroupRequest(requesterID, []byte(`{"title":"Team"} {"title":"Another"}`)))
	require.Equal(t, 400, w.Code)
}
