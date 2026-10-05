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

type createDirectTxContextKey struct{}

func TestCreateDirect(t *testing.T) {
	user1ID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	user2ID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	t.Run("creates chat with normalized users and participants", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createDirectTxContextKey{}, "transaction")
		usersRepository := NewMockUsersRepository(t)
		chatsRepository := NewMockChatsRepository(t)
		txManager := NewMockTXManager(t)
		service := NewChatsService(chatsRepository, usersRepository, txManager)
		user1 := newCreateDirectTestUser(t, user1ID, nil)
		user2 := newCreateDirectTestUser(t, user2ID, nil)
		var lockedUserIDs []uuid.UUID
		var savedDirect domain.DirectChat
		var savedParticipants []domain.ChatParticipant

		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, user1ID).
			Run(func(_ context.Context, id uuid.UUID) {
				lockedUserIDs = append(lockedUserIDs, id)
			}).
			Return(user1, nil)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, user2ID).
			Run(func(_ context.Context, id uuid.UUID) {
				lockedUserIDs = append(lockedUserIDs, id)
			}).
			Return(user2, nil)
		chatsRepository.EXPECT().
			CreateDirect(txCtx, mock.Anything, mock.Anything, mock.Anything).
			Run(func(
				_ context.Context,
				direct domain.DirectChat,
				participant1 domain.ChatParticipant,
				participant2 domain.ChatParticipant,
			) {
				savedDirect = direct
				savedParticipants = []domain.ChatParticipant{participant1, participant2}
			}).
			Return(nil)
		expectCreateDirectTransaction(txManager, outerCtx, txCtx)

		actual, created, err := service.CreateDirect(outerCtx, user2ID, user1ID)

		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, savedDirect, actual)
		require.Equal(t, user1ID, actual.User1ID)
		require.Equal(t, user2ID, actual.User2ID)
		require.Equal(t, []uuid.UUID{user1ID, user2ID}, lockedUserIDs)
		require.Len(t, savedParticipants, 2)
		require.Equal(t, user1ID, savedParticipants[0].UserID)
		require.Equal(t, user2ID, savedParticipants[1].UserID)
		for _, participant := range savedParticipants {
			require.Equal(t, actual.Chat.ID, participant.ChatID)
			require.Nil(t, participant.LastReadMessageID)
			require.True(t, actual.Chat.CreatedAt.Equal(participant.JoinedAt))
		}
	})

	t.Run("rejects chat with the same user before transaction", func(t *testing.T) {
		service := NewChatsService(
			NewMockChatsRepository(t),
			NewMockUsersRepository(t),
			NewMockTXManager(t),
		)

		actual, created, err := service.CreateDirect(t.Context(), user1ID, user1ID)

		require.ErrorIs(t, err, domain.ErrInvalidDirectChat)
		require.False(t, created)
		require.Empty(t, actual)
	})

	t.Run("returns first user lookup error", func(t *testing.T) {
		repositoryErr := errors.New("database unavailable")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createDirectTxContextKey{}, "transaction")
		usersRepository := NewMockUsersRepository(t)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, user1ID).
			Return(domain.User{}, repositoryErr)
		txManager := NewMockTXManager(t)
		expectCreateDirectTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(NewMockChatsRepository(t), usersRepository, txManager)

		actual, created, err := service.CreateDirect(outerCtx, user1ID, user2ID)

		require.ErrorIs(t, err, repositoryErr)
		require.False(t, created)
		require.Empty(t, actual)
	})

	t.Run("returns second user lookup error", func(t *testing.T) {
		repositoryErr := errors.New("database unavailable")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createDirectTxContextKey{}, "transaction")
		usersRepository := NewMockUsersRepository(t)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, user1ID).
			Return(newCreateDirectTestUser(t, user1ID, nil), nil)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, user2ID).
			Return(domain.User{}, repositoryErr)
		txManager := NewMockTXManager(t)
		expectCreateDirectTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(NewMockChatsRepository(t), usersRepository, txManager)

		actual, created, err := service.CreateDirect(outerCtx, user1ID, user2ID)

		require.ErrorIs(t, err, repositoryErr)
		require.False(t, created)
		require.Empty(t, actual)
	})

	t.Run("returns not found when first user is deleted", func(t *testing.T) {
		deletedAt := time.Now().UTC()
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createDirectTxContextKey{}, "transaction")
		usersRepository := NewMockUsersRepository(t)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, user1ID).
			Return(newCreateDirectTestUser(t, user1ID, &deletedAt), nil)
		txManager := NewMockTXManager(t)
		expectCreateDirectTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(NewMockChatsRepository(t), usersRepository, txManager)

		actual, created, err := service.CreateDirect(outerCtx, user1ID, user2ID)

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.False(t, created)
		require.Empty(t, actual)
	})

	t.Run("returns not found when second user is deleted", func(t *testing.T) {
		deletedAt := time.Now().UTC()
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createDirectTxContextKey{}, "transaction")
		usersRepository := NewMockUsersRepository(t)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, user1ID).
			Return(newCreateDirectTestUser(t, user1ID, nil), nil)
		usersRepository.EXPECT().
			GetUserForUpdate(txCtx, user2ID).
			Return(newCreateDirectTestUser(t, user2ID, &deletedAt), nil)
		txManager := NewMockTXManager(t)
		expectCreateDirectTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(NewMockChatsRepository(t), usersRepository, txManager)

		actual, created, err := service.CreateDirect(outerCtx, user1ID, user2ID)

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.False(t, created)
		require.Empty(t, actual)
	})

	t.Run("returns repository creation error", func(t *testing.T) {
		createErr := errors.New("insert failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createDirectTxContextKey{}, "transaction")
		usersRepository := NewMockUsersRepository(t)
		expectCreateDirectActiveUsers(t, usersRepository, txCtx, user1ID, user2ID)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			CreateDirect(txCtx, mock.Anything, mock.Anything, mock.Anything).
			Return(createErr)
		txManager := NewMockTXManager(t)
		expectCreateDirectTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, usersRepository, txManager)

		actual, created, err := service.CreateDirect(outerCtx, user1ID, user2ID)

		require.ErrorIs(t, err, createErr)
		require.False(t, created)
		require.Empty(t, actual)
	})

	t.Run("returns existing chat after pair conflict", func(t *testing.T) {
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createDirectTxContextKey{}, "transaction")
		usersRepository := NewMockUsersRepository(t)
		expectCreateDirectActiveUsers(t, usersRepository, txCtx, user1ID, user2ID)
		existing, err := domain.NewDirectChat(uuid.New(), user1ID, user2ID, time.Now().Add(-time.Hour))
		require.NoError(t, err)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			CreateDirect(txCtx, mock.Anything, mock.Anything, mock.Anything).
			Return(domain.ErrAlreadyExists)
		chatsRepository.EXPECT().
			GetDirectByUsers(outerCtx, user1ID, user2ID).
			Return(existing, nil)
		txManager := NewMockTXManager(t)
		expectCreateDirectTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, usersRepository, txManager)

		actual, created, err := service.CreateDirect(outerCtx, user2ID, user1ID)

		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, existing, actual)
	})

	t.Run("returns lookup error after pair conflict", func(t *testing.T) {
		lookupErr := errors.New("select failed")
		outerCtx := t.Context()
		txCtx := context.WithValue(outerCtx, createDirectTxContextKey{}, "transaction")
		usersRepository := NewMockUsersRepository(t)
		expectCreateDirectActiveUsers(t, usersRepository, txCtx, user1ID, user2ID)
		chatsRepository := NewMockChatsRepository(t)
		chatsRepository.EXPECT().
			CreateDirect(txCtx, mock.Anything, mock.Anything, mock.Anything).
			Return(domain.ErrAlreadyExists)
		chatsRepository.EXPECT().
			GetDirectByUsers(outerCtx, user1ID, user2ID).
			Return(domain.DirectChat{}, lookupErr)
		txManager := NewMockTXManager(t)
		expectCreateDirectTransaction(txManager, outerCtx, txCtx)
		service := NewChatsService(chatsRepository, usersRepository, txManager)

		actual, created, err := service.CreateDirect(outerCtx, user1ID, user2ID)

		require.ErrorIs(t, err, lookupErr)
		require.False(t, created)
		require.Empty(t, actual)
	})

	t.Run("returns transaction manager error", func(t *testing.T) {
		transactionErr := errors.New("cannot begin transaction")
		outerCtx := t.Context()
		txManager := NewMockTXManager(t)
		txManager.EXPECT().
			WithinTransaction(outerCtx, mock.Anything).
			Return(transactionErr)
		service := NewChatsService(
			NewMockChatsRepository(t),
			NewMockUsersRepository(t),
			txManager,
		)

		actual, created, err := service.CreateDirect(outerCtx, user1ID, user2ID)

		require.ErrorIs(t, err, transactionErr)
		require.False(t, created)
		require.Empty(t, actual)
	})
}

func expectCreateDirectTransaction(
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

func expectCreateDirectActiveUsers(
	t *testing.T,
	repository *MockUsersRepository,
	ctx context.Context,
	user1ID uuid.UUID,
	user2ID uuid.UUID,
) {
	t.Helper()
	repository.EXPECT().
		GetUserForUpdate(ctx, user1ID).
		Return(newCreateDirectTestUser(t, user1ID, nil), nil)
	repository.EXPECT().
		GetUserForUpdate(ctx, user2ID).
		Return(newCreateDirectTestUser(t, user2ID, nil), nil)
}

func newCreateDirectTestUser(
	t *testing.T,
	id uuid.UUID,
	deletedAt *time.Time,
) domain.User {
	t.Helper()
	profile, err := domain.NewUserProfile("User_123", "Test user", nil, nil)
	require.NoError(t, err)
	user, err := domain.NewUser(
		id,
		profile,
		time.Now().Add(-24*time.Hour),
		deletedAt,
		"password hash",
	)
	require.NoError(t, err)
	return user
}
