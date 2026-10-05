//go:build integration

package messages_postgres_repository

import (
	"context"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestGetDirectMessageState(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("returns both accounts and their deletion state", func(t *testing.T) {
		repository, direct := newDirectMessageStateFixture(t, pool, config.Timeout)
		deletedAt := direct.Chat.CreatedAt.Add(time.Hour)
		_, err := pool.Exec(t.Context(), `
			UPDATE users SET deleted_at = $1 WHERE id = $2
		`, deletedAt, direct.User2ID)
		require.NoError(t, err)

		state, err := repository.GetDirectMessageState(t.Context(), direct.Chat.ID)

		require.NoError(t, err)
		require.Equal(t, direct.User1ID, state.Users[0].UserID)
		require.False(t, state.Users[0].Deleted)
		require.Equal(t, direct.User2ID, state.Users[1].UserID)
		require.True(t, state.Users[1].Deleted)
	})

	t.Run("returns not found when direct state does not exist", func(t *testing.T) {
		repository := NewRepository(pool, config.Timeout)

		state, err := repository.GetDirectMessageState(t.Context(), uuid.New())

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Zero(t, state)
	})

	t.Run("uses transaction from context", func(t *testing.T) {
		repository, direct := newDirectMessageStateFixture(t, pool, config.Timeout)
		manager := postgres.NewTransactionManager(pool)
		rollbackErr := context.Canceled
		var stateDeleted bool

		err := manager.WithinTransaction(t.Context(), func(ctx context.Context) error {
			db := postgres.GetExecutor(ctx, pool)
			_, err := db.Exec(ctx, `
				UPDATE users SET deleted_at = $1 WHERE id = $2
			`, direct.Chat.CreatedAt.Add(time.Hour), direct.User1ID)
			if err != nil {
				return err
			}

			state, err := repository.GetDirectMessageState(ctx, direct.Chat.ID)
			if err != nil {
				return err
			}
			stateDeleted = state.Users[0].Deleted
			return rollbackErr
		})

		require.ErrorIs(t, err, rollbackErr)
		require.True(t, stateDeleted)
		state, err := repository.GetDirectMessageState(t.Context(), direct.Chat.ID)
		require.NoError(t, err)
		require.False(t, state.Users[0].Deleted)
	})
}

func TestGetGroupSenderState(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("returns active group participant account", func(t *testing.T) {
		repository, group, senderID := newGroupSenderStateFixture(t, pool, config.Timeout)

		state, err := repository.GetGroupSenderState(t.Context(), group.Chat.ID, senderID)

		require.NoError(t, err)
		require.Equal(t, senderID, state.UserID)
		require.False(t, state.Deleted)
	})

	t.Run("returns deleted group participant account", func(t *testing.T) {
		repository, group, senderID := newGroupSenderStateFixture(t, pool, config.Timeout)
		_, err := pool.Exec(t.Context(), `
			UPDATE users SET deleted_at = $1 WHERE id = $2
		`, group.Chat.CreatedAt.Add(time.Hour), senderID)
		require.NoError(t, err)

		state, err := repository.GetGroupSenderState(t.Context(), group.Chat.ID, senderID)

		require.NoError(t, err)
		require.Equal(t, senderID, state.UserID)
		require.True(t, state.Deleted)
	})

	t.Run("returns not found for non-participant", func(t *testing.T) {
		repository, group, _ := newGroupSenderStateFixture(t, pool, config.Timeout)
		outsiderID := uuid.New()
		insertMessageRepositoryUser(t, pool, outsiderID)
		t.Cleanup(func() {
			deleteMessageRepositoryUser(t, pool, config.Timeout, outsiderID)
		})

		state, err := repository.GetGroupSenderState(t.Context(), group.Chat.ID, outsiderID)

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Zero(t, state)
	})
}

func newDirectMessageStateFixture(
	t *testing.T,
	pool *pgxpool.Pool,
	timeout time.Duration,
) (*Repository, domain.DirectChat) {
	t.Helper()
	user1ID := uuid.New()
	user2ID := uuid.New()
	createdAt := repositoryTestTime()
	direct, err := domain.NewDirectChat(uuid.New(), user1ID, user2ID, createdAt)
	require.NoError(t, err)
	insertMessageRepositoryUser(t, pool, direct.User1ID)
	insertMessageRepositoryUser(t, pool, direct.User2ID)
	_, err = pool.Exec(t.Context(), `
		INSERT INTO chats (id, type, last_activity_at, created_at)
		VALUES ($1, $2, $3, $4)
	`, direct.Chat.ID, direct.Chat.Type, direct.Chat.LastActivityAt, direct.Chat.CreatedAt)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `
		INSERT INTO directs (chat_id, user1_id, user2_id)
		VALUES ($1, $2, $3)
	`, direct.Chat.ID, direct.User1ID, direct.User2ID)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `
		INSERT INTO chat_participants (chat_id, user_id, joined_at)
		VALUES ($1, $2, $4), ($1, $3, $4)
	`, direct.Chat.ID, direct.User1ID, direct.User2ID, direct.Chat.CreatedAt)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_, err := pool.Exec(ctx, `DELETE FROM chats WHERE id = $1`, direct.Chat.ID)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, direct.User1ID, direct.User2ID)
		require.NoError(t, err)
	})
	return NewRepository(pool, timeout), direct
}

func newGroupSenderStateFixture(
	t *testing.T,
	pool *pgxpool.Pool,
	timeout time.Duration,
) (*Repository, domain.GroupChat, uuid.UUID) {
	t.Helper()
	senderID := uuid.New()
	createdAt := repositoryTestTime()
	group, err := domain.NewGroupChat(uuid.New(), "Message state test", createdAt)
	require.NoError(t, err)
	insertMessageRepositoryUser(t, pool, senderID)
	_, err = pool.Exec(t.Context(), `
		INSERT INTO chats (id, type, last_activity_at, created_at)
		VALUES ($1, $2, $3, $4)
	`, group.Chat.ID, group.Chat.Type, group.Chat.LastActivityAt, group.Chat.CreatedAt)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `
		INSERT INTO groups (chat_id, title)
		VALUES ($1, $2)
	`, group.Chat.ID, group.Title)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `
		INSERT INTO chat_participants (chat_id, user_id, joined_at)
		VALUES ($1, $2, $3)
	`, group.Chat.ID, senderID, group.Chat.CreatedAt)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `
		INSERT INTO group_participants (chat_id, user_id, role)
		VALUES ($1, $2, 'member')
	`, group.Chat.ID, senderID)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_, err := pool.Exec(ctx, `DELETE FROM chats WHERE id = $1`, group.Chat.ID)
		require.NoError(t, err)
		deleteMessageRepositoryUser(t, pool, timeout, senderID)
	})
	return NewRepository(pool, timeout), group, senderID
}
