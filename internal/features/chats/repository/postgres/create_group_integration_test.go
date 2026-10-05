//go:build integration

package chats_postgres_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestCreateGroup(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("persists group and all participants", func(t *testing.T) {
		repository, group, participants := newCreateGroupTestData(
			t,
			pool,
			config.Timeout,
		)

		err := repository.CreateGroup(t.Context(), group, participants)

		require.NoError(t, err)
		requireCreateGroupPersisted(t, pool, group, participants)
	})

	t.Run("rejects participant from another chat before persistence", func(t *testing.T) {
		repository, group, participants := newCreateGroupTestData(
			t,
			pool,
			config.Timeout,
		)
		participant := participants[0]
		participant.ChatID = uuid.New()

		err := repository.CreateGroup(
			t.Context(),
			group,
			[]domain.GroupParticipant{participant},
		)

		require.EqualError(t, err, "participant ids and chat ids mismatch")
		require.Equal(t, 0, createGroupRowCount(t, pool, "chats", group.Chat.ID))
		require.Equal(t, 0, createGroupRowCount(t, pool, "groups", group.Chat.ID))
	})

	t.Run("rolls back aggregate when participant user does not exist", func(t *testing.T) {
		repository := NewChatsRepository(pool, config.Timeout)
		createdAt := createDirectTestTime()
		group, err := domain.NewGroupChat(uuid.New(), "Unknown participant", createdAt)
		require.NoError(t, err)
		participant, err := domain.NewGroupParticipant(
			group.Chat.ID,
			uuid.New(),
			nil,
			createdAt,
			domain.OwnerRole,
		)
		require.NoError(t, err)

		err = repository.CreateGroup(
			t.Context(),
			group,
			[]domain.GroupParticipant{participant},
		)

		require.True(t, postgres.IsConstraintViolation(
			err,
			postgres.ForeignKeyViolation,
			"chat_participants_user_id_fkey",
		))
		require.Equal(t, 0, createGroupRowCount(t, pool, "chats", group.Chat.ID))
		require.Equal(t, 0, createGroupRowCount(t, pool, "groups", group.Chat.ID))
		require.Equal(t, 0, createGroupParticipantCount(t, pool, group.Chat.ID))
	})

	t.Run("uses transaction executor and rolls back with transaction", func(t *testing.T) {
		repository, group, participants := newCreateGroupTestData(
			t,
			pool,
			config.Timeout,
		)
		manager := postgres.NewTransactionManager(pool)
		rollbackErr := errors.New("rollback test transaction")

		err := manager.WithinTransaction(t.Context(), func(ctx context.Context) error {
			if err := repository.CreateGroup(ctx, group, participants); err != nil {
				return err
			}
			require.Equal(t, 1, createGroupRowCountWithContext(
				t,
				postgres.GetExecutor(ctx, pool),
				"chats",
				group.Chat.ID,
			))
			return rollbackErr
		})

		require.ErrorIs(t, err, rollbackErr)
		require.Equal(t, 0, createGroupRowCount(t, pool, "chats", group.Chat.ID))
	})
}

func newCreateGroupTestData(
	t *testing.T,
	pool *pgxpool.Pool,
	timeout time.Duration,
) (*ChatsRepository, domain.GroupChat, []domain.GroupParticipant) {
	t.Helper()

	createdAt := createDirectTestTime()
	userIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, userID := range userIDs {
		insertCreateDirectTestUser(t, pool, userID, createdAt)
	}

	group, err := domain.NewGroupChat(uuid.New(), "Backend group", createdAt)
	require.NoError(t, err)
	roles := []domain.GroupRole{domain.OwnerRole, domain.MemberRole, domain.AdminRole}
	participants := make([]domain.GroupParticipant, 0, len(userIDs))
	for i, userID := range userIDs {
		participant, err := domain.NewGroupParticipant(
			group.Chat.ID,
			userID,
			nil,
			createdAt,
			roles[i],
		)
		require.NoError(t, err)
		participants = append(participants, participant)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_, err := pool.Exec(ctx, `DELETE FROM chats WHERE id = $1`, group.Chat.ID)
		require.NoError(t, err)
		_, err = pool.Exec(
			ctx,
			`DELETE FROM users WHERE id = ANY($1::uuid[])`,
			userIDs,
		)
		require.NoError(t, err)
	})

	return NewChatsRepository(pool, timeout), group, participants
}

func requireCreateGroupPersisted(
	t *testing.T,
	pool *pgxpool.Pool,
	expectedGroup domain.GroupChat,
	expectedParticipants []domain.GroupParticipant,
) {
	t.Helper()

	var (
		chatType      string
		lastMessageID *uuid.UUID
		lastActivity  time.Time
		createdAt     time.Time
		title         string
	)
	err := pool.QueryRow(t.Context(), `
		SELECT c.type, c.last_message_id, c.last_activity_at, c.created_at, g.title
		FROM chats c
		JOIN groups g ON g.chat_id = c.id
		WHERE c.id = $1
	`, expectedGroup.Chat.ID).Scan(
		&chatType,
		&lastMessageID,
		&lastActivity,
		&createdAt,
		&title,
	)
	require.NoError(t, err)
	require.Equal(t, string(domain.ChatTypeGroup), chatType)
	require.Equal(t, expectedGroup.Chat.LastMessageID, lastMessageID)
	require.True(t, expectedGroup.Chat.LastActivityAt.Equal(lastActivity))
	require.True(t, expectedGroup.Chat.CreatedAt.Equal(createdAt))
	require.Equal(t, expectedGroup.Title, title)
	require.Equal(t, len(expectedParticipants), createGroupParticipantCount(
		t,
		pool,
		expectedGroup.Chat.ID,
	))

	for _, expected := range expectedParticipants {
		var (
			lastReadMessageID *uuid.UUID
			joinedAt          time.Time
			role              string
		)
		err := pool.QueryRow(t.Context(), `
			SELECT cp.last_read_message_id, cp.joined_at, gp.role
			FROM chat_participants cp
			JOIN group_participants gp
			  ON gp.chat_id = cp.chat_id AND gp.user_id = cp.user_id
			WHERE cp.chat_id = $1 AND cp.user_id = $2
		`, expected.ChatID, expected.UserID).Scan(
			&lastReadMessageID,
			&joinedAt,
			&role,
		)
		require.NoError(t, err)
		require.Equal(t, expected.LastReadMessageID, lastReadMessageID)
		require.True(t, expected.JoinedAt.Equal(joinedAt))
		require.Equal(t, string(expected.Role()), role)
	}
}

func createGroupRowCount(
	t *testing.T,
	pool *pgxpool.Pool,
	table string,
	chatID uuid.UUID,
) int {
	t.Helper()
	return createGroupRowCountWithContext(t, pool, table, chatID)
}

func createGroupRowCountWithContext(
	t *testing.T,
	db postgres.DBTX,
	table string,
	chatID uuid.UUID,
) int {
	t.Helper()

	queries := map[string]string{
		"chats":  `SELECT count(*) FROM chats WHERE id = $1`,
		"groups": `SELECT count(*) FROM groups WHERE chat_id = $1`,
	}
	query, ok := queries[table]
	require.True(t, ok)
	var count int
	require.NoError(t, db.QueryRow(t.Context(), query, chatID).Scan(&count))
	return count
}

func createGroupParticipantCount(
	t *testing.T,
	pool *pgxpool.Pool,
	chatID uuid.UUID,
) int {
	t.Helper()

	var count int
	err := pool.QueryRow(t.Context(), `
		SELECT count(*)
		FROM chat_participants cp
		JOIN group_participants gp
		  ON gp.chat_id = cp.chat_id AND gp.user_id = cp.user_id
		WHERE cp.chat_id = $1
	`, chatID).Scan(&count)
	require.NoError(t, err)
	return count
}
