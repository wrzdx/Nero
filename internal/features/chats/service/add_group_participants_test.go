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

type addGroupParticipantsTxContextKey struct{}

func TestAddGroupParticipants(t *testing.T) {
	groupID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	memberID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	unavailableID := uuid.MustParse("00000000-0000-0000-0000-000000000004")
	existingID := uuid.MustParse("00000000-0000-0000-0000-000000000005")

	t.Run("returns positional results for mixed batch with duplicates", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, addGroupParticipantsTxContextKey{}, "transaction")
		participantIDs := []uuid.UUID{memberID, unavailableID, memberID, existingID}
		chatsRepository := NewMockChatsRepository(t)
		expectActiveAddGroupRequester(chatsRepository, outerCtx, requesterID)
		chatsRepository.EXPECT().
			GetGroupParticipant(outerCtx, groupID, requesterID).
			Return(newAddGroupRequester(t, groupID, requesterID, domain.OwnerRole), nil)
		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, participantIDs).
			Return([]ParticipantStatus{
				{UserID: existingID, Found: true},
				{UserID: unavailableID, Found: false},
				{UserID: memberID, Found: true},
			}, nil)

		chatsRepository.EXPECT().
			AddGroupParticipants(txCtx, groupID, mock.Anything).
			RunAndReturn(func(
				_ context.Context,
				_ uuid.UUID,
				participants []domain.GroupParticipant,
			) ([]bool, error) {
				require.Len(t, participants, 3)
				require.Equal(t, []uuid.UUID{memberID, memberID, existingID}, []uuid.UUID{
					participants[0].UserID,
					participants[1].UserID,
					participants[2].UserID,
				})
				for _, participant := range participants {
					require.Equal(t, groupID, participant.ChatID)
					require.Equal(t, domain.MemberRole, participant.Role())
					require.Nil(t, participant.LastReadMessageID)
					require.False(t, participant.JoinedAt.IsZero())
				}
				require.True(t, participants[0].JoinedAt.Equal(participants[1].JoinedAt))
				require.True(t, participants[1].JoinedAt.Equal(participants[2].JoinedAt))
				return []bool{true, false, false}, nil
			})
		txManager := NewMockTXManager(t)
		expectAddGroupParticipantsTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, NewMockUsersRepository(t), txManager)

		actual, err := service.AddGroupParticipants(outerCtx, AddGroupParticipantsCommand{
			GroupID:        groupID,
			RequesterID:    requesterID,
			ParticipantIDs: participantIDs,
		})

		require.NoError(t, err)
		require.Equal(t, []AddGroupParticipantResult{
			{UserID: memberID, Status: "added"},
			{UserID: unavailableID, Status: "unavailable"},
			{UserID: memberID, Status: "already_member"},
			{UserID: existingID, Status: "already_member"},
		}, actual)
	})

	t.Run("allows admin to submit empty batch", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, addGroupParticipantsTxContextKey{}, "transaction")
		chatsRepository := NewMockChatsRepository(t)
		expectActiveAddGroupRequester(chatsRepository, outerCtx, requesterID)
		chatsRepository.EXPECT().
			GetGroupParticipant(outerCtx, groupID, requesterID).
			Return(newAddGroupRequester(t, groupID, requesterID, domain.AdminRole), nil)
		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, []uuid.UUID(nil)).
			Return([]ParticipantStatus{}, nil)
		chatsRepository.EXPECT().
			AddGroupParticipants(txCtx, groupID, []domain.GroupParticipant{}).
			Return([]bool{}, nil)
		txManager := NewMockTXManager(t)
		expectAddGroupParticipantsTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, NewMockUsersRepository(t), txManager)

		actual, err := service.AddGroupParticipants(outerCtx, AddGroupParticipantsCommand{
			GroupID:     groupID,
			RequesterID: requesterID,
		})

		require.NoError(t, err)
		require.NotNil(t, actual)
		require.Empty(t, actual)
	})

	t.Run("rejects requester without management role", func(t *testing.T) {
		chatsRepository := NewMockChatsRepository(t)
		expectActiveAddGroupRequester(chatsRepository, t.Context(), requesterID)
		chatsRepository.EXPECT().
			GetGroupParticipant(t.Context(), groupID, requesterID).
			Return(newAddGroupRequester(t, groupID, requesterID, domain.MemberRole), nil)
		service := NewChatsService(
			chatsRepository,
			NewMockUsersRepository(t),
			NewMockTXManager(t),
		)

		actual, err := service.AddGroupParticipants(t.Context(), AddGroupParticipantsCommand{
			GroupID:        groupID,
			RequesterID:    requesterID,
			ParticipantIDs: []uuid.UUID{memberID},
		})

		require.ErrorIs(t, err, ErrNotEnoughRights)
		require.Nil(t, actual)
	})

	validationTests := []struct {
		name    string
		command AddGroupParticipantsCommand
	}{
		{
			name: "nil group id",
			command: AddGroupParticipantsCommand{
				RequesterID:    requesterID,
				ParticipantIDs: []uuid.UUID{memberID},
			},
		},
		{
			name: "nil requester id",
			command: AddGroupParticipantsCommand{
				GroupID:        groupID,
				ParticipantIDs: []uuid.UUID{memberID},
			},
		},
		{
			name: "nil participant id",
			command: AddGroupParticipantsCommand{
				GroupID:        groupID,
				RequesterID:    requesterID,
				ParticipantIDs: []uuid.UUID{uuid.Nil},
			},
		},
		{
			name: "batch larger than limit",
			command: AddGroupParticipantsCommand{
				GroupID:        groupID,
				RequesterID:    requesterID,
				ParticipantIDs: make([]uuid.UUID, 101),
			},
		},
	}
	for _, testCase := range validationTests {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			service := NewChatsService(
				NewMockChatsRepository(t),
				NewMockUsersRepository(t),
				NewMockTXManager(t),
			)

			actual, err := service.AddGroupParticipants(t.Context(), testCase.command)

			require.ErrorIs(t, err, ErrInvalidInput)
			require.Nil(t, actual)
		})
	}

	t.Run("returns requester lookup error", func(t *testing.T) {
		lookupErr := errors.New("requester lookup failed")
		chatsRepository := NewMockChatsRepository(t)
		expectActiveAddGroupRequester(chatsRepository, t.Context(), requesterID)
		chatsRepository.EXPECT().
			GetGroupParticipant(t.Context(), groupID, requesterID).
			Return(domain.GroupParticipant{}, lookupErr)
		service := NewChatsService(
			chatsRepository,
			NewMockUsersRepository(t),
			NewMockTXManager(t),
		)

		actual, err := service.AddGroupParticipants(t.Context(), AddGroupParticipantsCommand{
			GroupID:        groupID,
			RequesterID:    requesterID,
			ParticipantIDs: []uuid.UUID{memberID},
		})

		require.ErrorIs(t, err, lookupErr)
		require.Nil(t, actual)
	})

	t.Run("returns participant status error", func(t *testing.T) {
		statusErr := errors.New("status query failed")
		chatsRepository := NewMockChatsRepository(t)
		expectActiveAddGroupRequester(chatsRepository, t.Context(), requesterID)
		chatsRepository.EXPECT().
			GetGroupParticipant(t.Context(), groupID, requesterID).
			Return(newAddGroupRequester(t, groupID, requesterID, domain.OwnerRole), nil)
		chatsRepository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{memberID}).
			Return(nil, statusErr)
		service := NewChatsService(
			chatsRepository,
			NewMockUsersRepository(t),
			NewMockTXManager(t),
		)

		actual, err := service.AddGroupParticipants(t.Context(), AddGroupParticipantsCommand{
			GroupID:        groupID,
			RequesterID:    requesterID,
			ParticipantIDs: []uuid.UUID{memberID},
		})

		require.ErrorIs(t, err, statusErr)
		require.Nil(t, actual)
	})

	t.Run("returns repository insertion error", func(t *testing.T) {
		insertErr := errors.New("insert failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, addGroupParticipantsTxContextKey{}, "transaction")
		chatsRepository := NewMockChatsRepository(t)
		expectActiveAddGroupRequester(chatsRepository, outerCtx, requesterID)
		chatsRepository.EXPECT().
			GetGroupParticipant(outerCtx, groupID, requesterID).
			Return(newAddGroupRequester(t, groupID, requesterID, domain.OwnerRole), nil)
		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, []uuid.UUID{memberID}).
			Return([]ParticipantStatus{{UserID: memberID, Found: true}}, nil)
		chatsRepository.EXPECT().
			AddGroupParticipants(txCtx, groupID, mock.Anything).
			Return(nil, insertErr)
		txManager := NewMockTXManager(t)
		expectAddGroupParticipantsTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, NewMockUsersRepository(t), txManager)

		actual, err := service.AddGroupParticipants(outerCtx, AddGroupParticipantsCommand{
			GroupID:        groupID,
			RequesterID:    requesterID,
			ParticipantIDs: []uuid.UUID{memberID},
		})

		require.ErrorIs(t, err, insertErr)
		require.Nil(t, actual)
	})

	t.Run("rejects repository result with unexpected length", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, addGroupParticipantsTxContextKey{}, "transaction")
		chatsRepository := NewMockChatsRepository(t)
		expectActiveAddGroupRequester(chatsRepository, outerCtx, requesterID)
		chatsRepository.EXPECT().
			GetGroupParticipant(outerCtx, groupID, requesterID).
			Return(newAddGroupRequester(t, groupID, requesterID, domain.OwnerRole), nil)
		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, []uuid.UUID{memberID}).
			Return([]ParticipantStatus{{UserID: memberID, Found: true}}, nil)
		chatsRepository.EXPECT().
			AddGroupParticipants(txCtx, groupID, mock.Anything).
			Return([]bool{}, nil)
		txManager := NewMockTXManager(t)
		expectAddGroupParticipantsTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, NewMockUsersRepository(t), txManager)

		actual, err := service.AddGroupParticipants(outerCtx, AddGroupParticipantsCommand{
			GroupID:        groupID,
			RequesterID:    requesterID,
			ParticipantIDs: []uuid.UUID{memberID},
		})

		require.EqualError(t, err, "transaction: len of add group result and toAdd not match")
		require.Nil(t, actual)
	})

	t.Run("returns transaction manager error", func(t *testing.T) {
		transactionErr := errors.New("cannot begin transaction")
		chatsRepository := NewMockChatsRepository(t)
		expectActiveAddGroupRequester(chatsRepository, t.Context(), requesterID)
		chatsRepository.EXPECT().
			GetGroupParticipant(t.Context(), groupID, requesterID).
			Return(newAddGroupRequester(t, groupID, requesterID, domain.OwnerRole), nil)
		chatsRepository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{memberID}).
			Return([]ParticipantStatus{{UserID: memberID, Found: true}}, nil)
		txManager := NewMockTXManager(t)
		txManager.EXPECT().
			WithinTransaction(t.Context(), mock.Anything).
			Return(transactionErr)
		service := NewChatsService(chatsRepository, NewMockUsersRepository(t), txManager)

		actual, err := service.AddGroupParticipants(t.Context(), AddGroupParticipantsCommand{
			GroupID:        groupID,
			RequesterID:    requesterID,
			ParticipantIDs: []uuid.UUID{memberID},
		})

		require.ErrorIs(t, err, transactionErr)
		require.Nil(t, actual)
	})
}

func expectActiveAddGroupRequester(
	repository *MockChatsRepository,
	ctx context.Context,
	requesterID uuid.UUID,
) {
	repository.EXPECT().
		GetParticipantsStatus(ctx, []uuid.UUID{requesterID}).
		Return([]ParticipantStatus{{UserID: requesterID, Found: true}}, nil)
}

func newAddGroupRequester(
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

func expectAddGroupParticipantsTransaction(
	txManager *MockTXManager,
	outerCtx context.Context,
	txCtx context.Context,
) {
	txManager.EXPECT().
		WithinTransaction(outerCtx, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			return fn(txCtx)
		})
}
