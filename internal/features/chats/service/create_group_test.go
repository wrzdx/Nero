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

type createGroupTxContextKey struct{}

func TestCreateGroup(t *testing.T) {
	creatorID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	member1ID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	member2ID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	t.Run("creates group with owner and members", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createGroupTxContextKey{}, "transaction")
		participantIDs := []uuid.UUID{member1ID, member2ID}
		chatsRepository := NewMockChatsRepository(t)
		usersRepository := NewMockUsersRepository(t)
		txManager := NewMockTXManager(t)
		service := NewChatsService(chatsRepository, usersRepository, txManager)

		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, participantIDs).
			Return([]ParticipantStatus{
				{UserID: member1ID, Found: true},
				{UserID: member2ID, Found: true},
			}, nil)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, creatorID).
			Return(newCreateDirectTestUser(t, creatorID, nil), nil)

		var savedGroup domain.GroupChat
		var savedParticipants []domain.GroupParticipant
		chatsRepository.EXPECT().
			CreateGroup(txCtx, mock.Anything, mock.Anything).
			Run(func(
				_ context.Context,
				group domain.GroupChat,
				participants []domain.GroupParticipant,
			) {
				savedGroup = group
				savedParticipants = append([]domain.GroupParticipant(nil), participants...)
			}).
			Return(nil)
		expectCreateGroupTransaction(txManager, outerCtx, txCtx)

		actual, err := service.CreateGroup(outerCtx, creatorID, CreateGroupCommand{
			Title:          "  Backend group  ",
			ParticipantIDs: participantIDs,
		})

		require.NoError(t, err)
		require.Equal(t, savedGroup, actual)
		require.Equal(t, "Backend group", actual.Title)
		require.Equal(t, domain.ChatTypeGroup, actual.Chat.Type)
		require.Len(t, savedParticipants, 3)

		require.Equal(t, creatorID, savedParticipants[0].UserID)
		require.Equal(t, domain.OwnerRole, savedParticipants[0].Role())
		require.Equal(t, member1ID, savedParticipants[1].UserID)
		require.Equal(t, domain.MemberRole, savedParticipants[1].Role())
		require.Equal(t, member2ID, savedParticipants[2].UserID)
		require.Equal(t, domain.MemberRole, savedParticipants[2].Role())
		for _, participant := range savedParticipants {
			require.Equal(t, actual.Chat.ID, participant.ChatID)
			require.Nil(t, participant.LastReadMessageID)
			require.True(t, actual.Chat.CreatedAt.Equal(participant.JoinedAt))
		}
	})

	t.Run("rejects invalid title before repository calls", func(t *testing.T) {
		service := NewChatsService(
			NewMockChatsRepository(t),
			NewMockUsersRepository(t),
			NewMockTXManager(t),
		)

		actual, err := service.CreateGroup(t.Context(), creatorID, CreateGroupCommand{})

		require.ErrorIs(t, err, domain.ErrInvalidGroupChat)
		require.Empty(t, actual)
	})

	t.Run("rejects nil participant before repository calls", func(t *testing.T) {
		service := NewChatsService(
			NewMockChatsRepository(t),
			NewMockUsersRepository(t),
			NewMockTXManager(t),
		)

		actual, err := service.CreateGroup(t.Context(), creatorID, CreateGroupCommand{
			Title:          "Backend group",
			ParticipantIDs: []uuid.UUID{uuid.Nil},
		})

		require.ErrorIs(t, err, domain.ErrInvalidGroupChat)
		require.ErrorIs(t, err, domain.ErrInvalidChatParticipant)
		require.Empty(t, actual)
	})

	t.Run("returns participant status error", func(t *testing.T) {
		statusErr := errors.New("status query failed")
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{member1ID}).
			Return(nil, statusErr)
		service := NewChatsService(
			chatsRepository,
			NewMockUsersRepository(t),
			NewMockTXManager(t),
		)

		actual, err := service.CreateGroup(t.Context(), creatorID, CreateGroupCommand{
			Title:          "Backend group",
			ParticipantIDs: []uuid.UUID{member1ID},
		})

		require.ErrorIs(t, err, statusErr)
		require.Empty(t, actual)
	})

	t.Run("returns details for unavailable members", func(t *testing.T) {
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			GetParticipantsStatus(t.Context(), []uuid.UUID{member1ID, member2ID}).
			Return([]ParticipantStatus{
				{UserID: member1ID, Found: false},
				{UserID: member2ID, Found: true},
			}, nil)
		service := NewChatsService(
			chatsRepository,
			NewMockUsersRepository(t),
			NewMockTXManager(t),
		)

		actual, err := service.CreateGroup(t.Context(), creatorID, CreateGroupCommand{
			Title:          "Backend group",
			ParticipantIDs: []uuid.UUID{member1ID, member2ID},
		})

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, actual)
		var detailed domain.DetailedError
		require.ErrorAs(t, err, &detailed)
		require.Equal(t, map[string]string{
			member1ID.String(): "not found",
		}, detailed.Fields())
	})

	t.Run("returns not found when creator does not exist", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createGroupTxContextKey{}, "transaction")
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, []uuid.UUID{member1ID}).
			Return([]ParticipantStatus{{UserID: member1ID, Found: true}}, nil)
		usersRepository := NewMockUsersRepository(t)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, creatorID).
			Return(domain.User{}, domain.ErrNotFound)
		txManager := NewMockTXManager(t)
		expectCreateGroupTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, usersRepository, txManager)

		actual, err := service.CreateGroup(outerCtx, creatorID, CreateGroupCommand{
			Title:          "Backend group",
			ParticipantIDs: []uuid.UUID{member1ID},
		})

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, actual)
		var detailed domain.DetailedError
		require.ErrorAs(t, err, &detailed)
		require.Equal(t, map[string]string{
			"creator_id": "creator not found",
		}, detailed.Fields())
	})

	t.Run("returns not found when creator is deleted", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createGroupTxContextKey{}, "transaction")
		deletedAt := time.Now().UTC()
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, []uuid.UUID{member1ID}).
			Return([]ParticipantStatus{{UserID: member1ID, Found: true}}, nil)
		usersRepository := NewMockUsersRepository(t)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, creatorID).
			Return(newCreateDirectTestUser(t, creatorID, &deletedAt), nil)
		txManager := NewMockTXManager(t)
		expectCreateGroupTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, usersRepository, txManager)

		actual, err := service.CreateGroup(outerCtx, creatorID, CreateGroupCommand{
			Title:          "Backend group",
			ParticipantIDs: []uuid.UUID{member1ID},
		})

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, actual)
	})

	t.Run("returns unexpected creator lookup error", func(t *testing.T) {
		lookupErr := errors.New("creator query failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createGroupTxContextKey{}, "transaction")
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, []uuid.UUID{member1ID}).
			Return([]ParticipantStatus{{UserID: member1ID, Found: true}}, nil)
		usersRepository := NewMockUsersRepository(t)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, creatorID).
			Return(domain.User{}, lookupErr)
		txManager := NewMockTXManager(t)
		expectCreateGroupTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, usersRepository, txManager)

		actual, err := service.CreateGroup(outerCtx, creatorID, CreateGroupCommand{
			Title:          "Backend group",
			ParticipantIDs: []uuid.UUID{member1ID},
		})

		require.ErrorIs(t, err, lookupErr)
		require.Empty(t, actual)
	})

	t.Run("returns group creation error", func(t *testing.T) {
		createErr := errors.New("insert failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createGroupTxContextKey{}, "transaction")
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, []uuid.UUID{member1ID}).
			Return([]ParticipantStatus{{UserID: member1ID, Found: true}}, nil)
		chatsRepository.EXPECT().
			CreateGroup(txCtx, mock.Anything, mock.Anything).
			Return(createErr)
		usersRepository := NewMockUsersRepository(t)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, creatorID).
			Return(newCreateDirectTestUser(t, creatorID, nil), nil)
		txManager := NewMockTXManager(t)
		expectCreateGroupTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, usersRepository, txManager)

		actual, err := service.CreateGroup(outerCtx, creatorID, CreateGroupCommand{
			Title:          "Backend group",
			ParticipantIDs: []uuid.UUID{member1ID},
		})

		require.ErrorIs(t, err, createErr)
		require.Empty(t, actual)
	})

	t.Run("returns transaction manager error", func(t *testing.T) {
		transactionErr := errors.New("cannot begin transaction")
		outerCtx := t.Context()
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			GetParticipantsStatus(outerCtx, []uuid.UUID{member1ID}).
			Return([]ParticipantStatus{{UserID: member1ID, Found: true}}, nil)
		txManager := NewMockTXManager(t)
		txManager.EXPECT().
			WithinTransaction(outerCtx, mock.Anything).
			Return(transactionErr)
		service := NewChatsService(
			chatsRepository,
			NewMockUsersRepository(t),
			txManager,
		)

		actual, err := service.CreateGroup(outerCtx, creatorID, CreateGroupCommand{
			Title:          "Backend group",
			ParticipantIDs: []uuid.UUID{member1ID},
		})

		require.ErrorIs(t, err, transactionErr)
		require.Empty(t, actual)
	})
}

func expectCreateGroupTransaction(
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
