package chats_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type removeGroupParticipantTxContextKey struct{}

func TestRemoveGroupParticipant(t *testing.T) {
	groupID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	targetID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	allowedCases := []struct {
		name          string
		requesterRole domain.GroupRole
		targetRole    domain.GroupRole
		self          bool
	}{
		{name: "owner removes member", requesterRole: domain.OwnerRole, targetRole: domain.MemberRole},
		{name: "admin removes member", requesterRole: domain.AdminRole, targetRole: domain.MemberRole},
		{name: "member leaves group", requesterRole: domain.MemberRole, targetRole: domain.MemberRole, self: true},
		{name: "admin leaves group", requesterRole: domain.AdminRole, targetRole: domain.AdminRole, self: true},
	}
	for _, testCase := range allowedCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualTargetID := targetID
			if testCase.self {
				actualTargetID = requesterID
			}
			requester := newRemoveGroupParticipant(t, groupID, requesterID, testCase.requesterRole)
			target := newRemoveGroupParticipant(t, groupID, actualTargetID, testCase.targetRole)
			outerCtx := t.Context()
			txCtx := context.WithValue(outerCtx, removeGroupParticipantTxContextKey{}, "transaction")
			repository := NewMockChatsRepository(t)
			expectActiveRemoveGroupRequester(repository, outerCtx, requesterID)
			if testCase.self {
				repository.EXPECT().
					GetGroupParticipant(outerCtx, groupID, requesterID).
					Return(requester, nil).
					Twice()
			} else {
				repository.EXPECT().
					GetGroupParticipant(outerCtx, groupID, requesterID).
					Return(requester, nil)
				repository.EXPECT().
					GetGroupParticipant(outerCtx, groupID, actualTargetID).
					Return(target, nil)
			}
			repository.EXPECT().
				RemoveGroupParticipant(txCtx, target).
				Return(nil)
			txManager := NewMockTXManager(t)
			txManager.EXPECT().
				WithinTransaction(outerCtx, mock.Anything).
				RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
					return fn(txCtx)
				})
			service := NewChatsService(repository, NewMockUsersRepository(t), txManager)

			err := service.RemoveGroupParticipant(outerCtx, RemoveGroupParticipantCommand{
				GroupID:     groupID,
				RequesterID: requesterID,
				TargetID:    actualTargetID,
			})

			require.NoError(t, err)
		})
	}

	deniedCases := []struct {
		name          string
		requesterRole domain.GroupRole
		targetRole    domain.GroupRole
		self          bool
		wantErr       error
	}{
		{
			name:          "owner cannot leave group",
			requesterRole: domain.OwnerRole,
			targetRole:    domain.OwnerRole,
			self:          true,
			wantErr:       ErrOwnerCannotQuitGroup,
		},
		{
			name:          "member cannot remove another member",
			requesterRole: domain.MemberRole,
			targetRole:    domain.MemberRole,
			wantErr:       ErrNotEnoughRights,
		},
		{
			name:          "admin cannot remove another admin",
			requesterRole: domain.AdminRole,
			targetRole:    domain.AdminRole,
			wantErr:       ErrNotEnoughRights,
		},
	}
	for _, testCase := range deniedCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualTargetID := targetID
			if testCase.self {
				actualTargetID = requesterID
			}
			requester := newRemoveGroupParticipant(t, groupID, requesterID, testCase.requesterRole)
			target := newRemoveGroupParticipant(t, groupID, actualTargetID, testCase.targetRole)
			repository := NewMockChatsRepository(t)
			expectActiveRemoveGroupRequester(repository, t.Context(), requesterID)
			if testCase.self {
				repository.EXPECT().
					GetGroupParticipant(t.Context(), groupID, requesterID).
					Return(requester, nil).
					Twice()
			} else {
				repository.EXPECT().
					GetGroupParticipant(t.Context(), groupID, requesterID).
					Return(requester, nil)
				repository.EXPECT().
					GetGroupParticipant(t.Context(), groupID, actualTargetID).
					Return(target, nil)
			}
			service := NewChatsService(
				repository,
				NewMockUsersRepository(t),
				NewMockTXManager(t),
			)

			err := service.RemoveGroupParticipant(t.Context(), RemoveGroupParticipantCommand{
				GroupID:     groupID,
				RequesterID: requesterID,
				TargetID:    actualTargetID,
			})

			require.ErrorIs(t, err, testCase.wantErr)
		})
	}

	validationCases := []struct {
		name    string
		command RemoveGroupParticipantCommand
	}{
		{name: "nil group id", command: RemoveGroupParticipantCommand{RequesterID: requesterID, TargetID: targetID}},
		{name: "nil requester id", command: RemoveGroupParticipantCommand{GroupID: groupID, TargetID: targetID}},
		{name: "nil target id", command: RemoveGroupParticipantCommand{GroupID: groupID, RequesterID: requesterID}},
	}
	for _, testCase := range validationCases {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			service := NewChatsService(
				NewMockChatsRepository(t),
				NewMockUsersRepository(t),
				NewMockTXManager(t),
			)

			err := service.RemoveGroupParticipant(t.Context(), testCase.command)

			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}

	t.Run("rejects inactive requester", func(t *testing.T) {
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{requesterID}).
			Return([]ParticipantStatus{{UserID: requesterID, Found: false}}, nil)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		err := service.RemoveGroupParticipant(t.Context(), RemoveGroupParticipantCommand{
			GroupID: groupID, RequesterID: requesterID, TargetID: targetID,
		})

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("returns requester status error", func(t *testing.T) {
		statusErr := errors.New("status lookup failed")
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{requesterID}).
			Return(nil, statusErr)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		err := service.RemoveGroupParticipant(t.Context(), RemoveGroupParticipantCommand{
			GroupID: groupID, RequesterID: requesterID, TargetID: targetID,
		})

		require.ErrorIs(t, err, statusErr)
	})

	t.Run("rejects unexpected requester status count", func(t *testing.T) {
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{requesterID}).
			Return([]ParticipantStatus{}, nil)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		err := service.RemoveGroupParticipant(t.Context(), RemoveGroupParticipantCommand{
			GroupID: groupID, RequesterID: requesterID, TargetID: targetID,
		})

		require.EqualError(t, err, "invalid get participant status len")
	})

	t.Run("returns target lookup error", func(t *testing.T) {
		lookupErr := errors.New("target lookup failed")
		repository := NewMockChatsRepository(t)
		expectActiveRemoveGroupRequester(repository, t.Context(), requesterID)
		repository.EXPECT().
			GetGroupParticipant(t.Context(), groupID, requesterID).
			Return(newRemoveGroupParticipant(t, groupID, requesterID, domain.AdminRole), nil)
		repository.EXPECT().
			GetGroupParticipant(t.Context(), groupID, targetID).
			Return(domain.GroupParticipant{}, lookupErr)
		service := NewChatsService(repository, NewMockUsersRepository(t), NewMockTXManager(t))

		err := service.RemoveGroupParticipant(t.Context(), RemoveGroupParticipantCommand{
			GroupID: groupID, RequesterID: requesterID, TargetID: targetID,
		})

		require.ErrorIs(t, err, lookupErr)
	})

	t.Run("returns repository error through transaction", func(t *testing.T) {
		removeErr := errors.New("remove failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, removeGroupParticipantTxContextKey{}, "transaction")
		requester := newRemoveGroupParticipant(t, groupID, requesterID, domain.AdminRole)
		target := newRemoveGroupParticipant(t, groupID, targetID, domain.MemberRole)
		repository := NewMockChatsRepository(t)
		expectActiveRemoveGroupRequester(repository, outerCtx, requesterID)
		repository.EXPECT().GetGroupParticipant(outerCtx, groupID, requesterID).Return(requester, nil)
		repository.EXPECT().GetGroupParticipant(outerCtx, groupID, targetID).Return(target, nil)
		repository.EXPECT().RemoveGroupParticipant(txCtx, target).Return(removeErr)
		txManager := NewMockTXManager(t)
		txManager.EXPECT().
			WithinTransaction(outerCtx, mock.Anything).
			RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				return fn(txCtx)
			})
		service := NewChatsService(repository, NewMockUsersRepository(t), txManager)

		err := service.RemoveGroupParticipant(outerCtx, RemoveGroupParticipantCommand{
			GroupID: groupID, RequesterID: requesterID, TargetID: targetID,
		})

		require.ErrorIs(t, err, removeErr)
	})
}

func expectActiveRemoveGroupRequester(
	repository *MockChatsRepository,
	ctx context.Context,
	requesterID uuid.UUID,
) {
	repository.EXPECT().
		GetParticipantsStatus(ctx, []uuid.UUID{requesterID}).
		Return([]ParticipantStatus{{UserID: requesterID, Found: true}}, nil)
}

func newRemoveGroupParticipant(
	t *testing.T,
	groupID, userID uuid.UUID,
	role domain.GroupRole,
) domain.GroupParticipant {
	t.Helper()
	participant, err := domain.NewGroupParticipant(
		groupID,
		userID,
		nil,
		time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC),
		role,
	)
	require.NoError(t, err)
	return participant
}
