package chats_service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUpdateGroup(t *testing.T) {
	groupID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	for _, role := range []domain.GroupRole{domain.OwnerRole, domain.AdminRole} {
		t.Run(string(role)+" updates and normalizes title", func(t *testing.T) {
			group := newUpdateGroupTestGroup(t, groupID, "Old title")
			expected, err := group.Update("New title")
			require.NoError(t, err)
			repository := NewMockChatsRepository(t)
			expectActiveUpdateGroupRequester(repository, t, requesterID)
			repository.EXPECT().
				GetGroupParticipant(t.Context(), groupID, requesterID).
				Return(newRemoveGroupParticipant(t, groupID, requesterID, role), nil)
			repository.EXPECT().GetGroup(t.Context(), groupID).Return(group, nil)
			repository.EXPECT().UpdateGroup(t.Context(), expected).Return(nil)
			service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

			actual, err := service.UpdateGroup(t.Context(), UpdateGroupCommand{
				RequesterID: requesterID,
				GroupID:     groupID,
				Title:       "  New title  ",
			})

			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}

	t.Run("rejects member", func(t *testing.T) {
		repository := NewMockChatsRepository(t)
		expectActiveUpdateGroupRequester(repository, t, requesterID)
		repository.EXPECT().
			GetGroupParticipant(t.Context(), groupID, requesterID).
			Return(newRemoveGroupParticipant(t, groupID, requesterID, domain.MemberRole), nil)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		group, err := service.UpdateGroup(t.Context(), UpdateGroupCommand{
			RequesterID: requesterID, GroupID: groupID, Title: "New title",
		})

		require.ErrorIs(t, err, ErrNotEnoughRights)
		require.Zero(t, group)
	})

	validationCases := []struct {
		name    string
		command UpdateGroupCommand
	}{
		{name: "nil requester id", command: UpdateGroupCommand{GroupID: groupID, Title: "New title"}},
		{name: "nil group id", command: UpdateGroupCommand{RequesterID: requesterID, Title: "New title"}},
	}
	for _, testCase := range validationCases {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			service := NewChatsService(
				NewMockChatsRepository(t),
				NewMockUsersRepository(t),
				NewMockTXManager(t),
			)

			group, err := service.UpdateGroup(t.Context(), testCase.command)

			require.ErrorIs(t, err, ErrInvalidInput)
			require.Zero(t, group)
		})
	}

	t.Run("rejects inactive requester", func(t *testing.T) {
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{requesterID}).
			Return([]ParticipantStatus{{UserID: requesterID, Found: false}}, nil)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		group, err := service.UpdateGroup(t.Context(), UpdateGroupCommand{
			RequesterID: requesterID, GroupID: groupID, Title: "New title",
		})

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Zero(t, group)
	})

	t.Run("rejects unexpected requester status count", func(t *testing.T) {
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{requesterID}).
			Return([]ParticipantStatus{}, nil)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		group, err := service.UpdateGroup(t.Context(), UpdateGroupCommand{
			RequesterID: requesterID, GroupID: groupID, Title: "New title",
		})

		require.EqualError(t, err, "invalid get participant status len")
		require.Zero(t, group)
	})

	t.Run("returns requester status error", func(t *testing.T) {
		statusErr := errors.New("status lookup failed")
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{requesterID}).
			Return(nil, statusErr)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		group, err := service.UpdateGroup(t.Context(), UpdateGroupCommand{
			RequesterID: requesterID, GroupID: groupID, Title: "New title",
		})

		require.ErrorIs(t, err, statusErr)
		require.Zero(t, group)
	})

	t.Run("returns requester participant error", func(t *testing.T) {
		lookupErr := errors.New("requester lookup failed")
		repository := NewMockChatsRepository(t)
		expectActiveUpdateGroupRequester(repository, t, requesterID)
		repository.EXPECT().
			GetGroupParticipant(t.Context(), groupID, requesterID).
			Return(domain.GroupParticipant{}, lookupErr)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		group, err := service.UpdateGroup(t.Context(), UpdateGroupCommand{
			RequesterID: requesterID, GroupID: groupID, Title: "New title",
		})

		require.ErrorIs(t, err, lookupErr)
		require.Zero(t, group)
	})

	t.Run("returns group lookup error", func(t *testing.T) {
		lookupErr := errors.New("group lookup failed")
		repository := NewMockChatsRepository(t)
		expectUpdateGroupRequesterWithRole(repository, t, groupID, requesterID, domain.AdminRole)
		repository.EXPECT().GetGroup(t.Context(), groupID).Return(domain.GroupChat{}, lookupErr)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		group, err := service.UpdateGroup(t.Context(), UpdateGroupCommand{
			RequesterID: requesterID, GroupID: groupID, Title: "New title",
		})

		require.ErrorIs(t, err, lookupErr)
		require.Zero(t, group)
	})

	t.Run("rejects invalid title without persistence", func(t *testing.T) {
		original := newUpdateGroupTestGroup(t, groupID, "Old title")
		repository := NewMockChatsRepository(t)
		expectUpdateGroupRequesterWithRole(repository, t, groupID, requesterID, domain.OwnerRole)
		repository.EXPECT().GetGroup(t.Context(), groupID).Return(original, nil)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		group, err := service.UpdateGroup(t.Context(), UpdateGroupCommand{
			RequesterID: requesterID,
			GroupID:     groupID,
			Title:       strings.Repeat("я", 129),
		})

		require.ErrorIs(t, err, domain.ErrInvalidGroupChat)
		require.Zero(t, group)
		require.Equal(t, "Old title", original.Title)
	})

	t.Run("returns repository update error", func(t *testing.T) {
		updateErr := errors.New("update failed")
		original := newUpdateGroupTestGroup(t, groupID, "Old title")
		updated, err := original.Update("New title")
		require.NoError(t, err)
		repository := NewMockChatsRepository(t)
		expectUpdateGroupRequesterWithRole(repository, t, groupID, requesterID, domain.OwnerRole)
		repository.EXPECT().GetGroup(t.Context(), groupID).Return(original, nil)
		repository.EXPECT().UpdateGroup(t.Context(), updated).Return(updateErr)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		group, err := service.UpdateGroup(t.Context(), UpdateGroupCommand{
			RequesterID: requesterID, GroupID: groupID, Title: "New title",
		})

		require.ErrorIs(t, err, updateErr)
		require.Zero(t, group)
	})
}

func expectActiveUpdateGroupRequester(
	repository *MockChatsRepository,
	t *testing.T,
	requesterID uuid.UUID,
) {
	t.Helper()
	repository.EXPECT().
		GetParticipantsStatus(t.Context(), []uuid.UUID{requesterID}).
		Return([]ParticipantStatus{{UserID: requesterID, Found: true}}, nil)
}

func expectUpdateGroupRequesterWithRole(
	repository *MockChatsRepository,
	t *testing.T,
	groupID, requesterID uuid.UUID,
	role domain.GroupRole,
) {
	t.Helper()
	expectActiveUpdateGroupRequester(repository, t, requesterID)
	repository.EXPECT().
		GetGroupParticipant(t.Context(), groupID, requesterID).
		Return(newRemoveGroupParticipant(t, groupID, requesterID, role), nil)
}

func newUpdateGroupTestGroup(
	t *testing.T,
	groupID uuid.UUID,
	title string,
) domain.GroupChat {
	t.Helper()
	group, err := domain.NewGroupChat(
		groupID,
		title,
		time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	return group
}
