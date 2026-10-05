//go:build integration

package chats_postgres_repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestCreateDirect(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("persists direct chat and both participants", func(t *testing.T) {
		repository, direct, participant1, participant2 := newCreateDirectTestData(
			t,
			pool,
			config.Timeout,
		)

		err := repository.CreateDirect(t.Context(), direct, participant1, participant2)

		require.NoError(t, err)
		requireCreateDirectChatPersisted(t, pool, direct)
		requireCreateDirectParticipantPersisted(t, pool, participant1)
		requireCreateDirectParticipantPersisted(t, pool, participant2)
	})

	t.Run("maps duplicate user pair and rolls back the new chat", func(t *testing.T) {
		repository, existing, participant1, participant2 := newCreateDirectTestData(
			t,
			pool,
			config.Timeout,
		)
		require.NoError(t, repository.CreateDirect(
			t.Context(),
			existing,
			participant1,
			participant2,
		))
		duplicate, duplicateParticipant1, duplicateParticipant2 := newCreateDirectAggregate(
			t,
			existing.User1ID,
			existing.User2ID,
		)

		err := repository.CreateDirect(
			t.Context(),
			duplicate,
			duplicateParticipant1,
			duplicateParticipant2,
		)

		require.ErrorIs(t, err, domain.ErrAlreadyExists)
		requireCreateDirectChatPersisted(t, pool, existing)
		require.Equal(t, 0, createDirectRowCount(t, pool, "chats", duplicate.Chat.ID))
		require.Equal(t, 0, createDirectRowCount(t, pool, "directs", duplicate.Chat.ID))
		require.Equal(t, 0, createDirectParticipantCount(t, pool, duplicate.Chat.ID))
	})

	t.Run("rejects duplicate participants before persistence", func(t *testing.T) {
		repository, direct, participant1, _ := newCreateDirectTestData(
			t,
			pool,
			config.Timeout,
		)
		duplicateParticipant, err := domain.NewChatParticipant(
			direct.Chat.ID,
			participant1.UserID,
			nil,
			direct.Chat.CreatedAt,
		)
		require.NoError(t, err)

		err = repository.CreateDirect(
			t.Context(),
			direct,
			participant1,
			duplicateParticipant,
		)

		require.EqualError(t, err, "participant ids must be different")
		require.Equal(t, 0, createDirectRowCount(t, pool, "chats", direct.Chat.ID))
		require.Equal(t, 0, createDirectRowCount(t, pool, "directs", direct.Chat.ID))
		require.Equal(t, 0, createDirectParticipantCount(t, pool, direct.Chat.ID))
	})

	t.Run("rolls back aggregate when last read message does not exist", func(t *testing.T) {
		repository, direct, participant1, participant2 := newCreateDirectTestData(
			t,
			pool,
			config.Timeout,
		)
		unknownMessageID := uuid.New()
		participantWithUnknownMessage, err := domain.NewChatParticipant(
			direct.Chat.ID,
			participant2.UserID,
			&unknownMessageID,
			participant2.JoinedAt,
		)
		require.NoError(t, err)

		err = repository.CreateDirect(
			t.Context(),
			direct,
			participant1,
			participantWithUnknownMessage,
		)

		require.True(t, postgres.IsConstraintViolation(
			err,
			postgres.ForeignKeyViolation,
			"chat_participants_last_read_message_id_fkey",
		))
		require.Equal(t, 0, createDirectRowCount(t, pool, "chats", direct.Chat.ID))
		require.Equal(t, 0, createDirectRowCount(t, pool, "directs", direct.Chat.ID))
		require.Equal(t, 0, createDirectParticipantCount(t, pool, direct.Chat.ID))
	})
}

func newCreateDirectTestData(
	t *testing.T,
	pool *pgxpool.Pool,
	timeout time.Duration,
) (*ChatsRepository, domain.DirectChat, domain.ChatParticipant, domain.ChatParticipant) {
	t.Helper()

	user1ID := uuid.New()
	user2ID := uuid.New()
	now := createDirectTestTime()
	insertCreateDirectTestUser(t, pool, user1ID, now)
	insertCreateDirectTestUser(t, pool, user2ID, now)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		_, err := pool.Exec(ctx, `
			DELETE FROM chats
			WHERE id IN (
				SELECT chat_id
				FROM directs
				WHERE user1_id IN ($1, $2) OR user2_id IN ($1, $2)
			)
		`, user1ID, user2ID)
		require.NoError(t, err)
		_, err = pool.Exec(
			ctx,
			`DELETE FROM users WHERE id IN ($1, $2)`,
			user1ID,
			user2ID,
		)
		require.NoError(t, err)
	})

	direct, participant1, participant2 := newCreateDirectAggregate(t, user1ID, user2ID)
	return NewChatsRepository(pool, timeout), direct, participant1, participant2
}

func newCreateDirectAggregate(
	t *testing.T,
	user1ID uuid.UUID,
	user2ID uuid.UUID,
) (domain.DirectChat, domain.ChatParticipant, domain.ChatParticipant) {
	t.Helper()

	createdAt := createDirectTestTime()
	direct, err := domain.NewDirectChat(uuid.New(), user1ID, user2ID, createdAt)
	require.NoError(t, err)
	participant1, err := domain.NewChatParticipant(
		direct.Chat.ID,
		direct.User1ID,
		nil,
		createdAt,
	)
	require.NoError(t, err)
	participant2, err := domain.NewChatParticipant(
		direct.Chat.ID,
		direct.User2ID,
		nil,
		createdAt,
	)
	require.NoError(t, err)
	return direct, participant1, participant2
}

func insertCreateDirectTestUser(
	t *testing.T,
	pool *pgxpool.Pool,
	userID uuid.UUID,
	createdAt time.Time,
) {
	t.Helper()

	usernameSuffix := strings.ReplaceAll(userID.String(), "-", "")[:16]
	_, err := pool.Exec(t.Context(), `
		INSERT INTO users (id, username, first_name, created_at, password_hash)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, "chat_"+usernameSuffix, "Chat Test", createdAt, "password_hash")
	require.NoError(t, err)
}

func requireCreateDirectChatPersisted(
	t *testing.T,
	pool *pgxpool.Pool,
	expected domain.DirectChat,
) {
	t.Helper()

	var (
		chatType      string
		lastMessageID *uuid.UUID
		lastActivity  time.Time
		createdAt     time.Time
		user1ID       uuid.UUID
		user2ID       uuid.UUID
	)
	err := pool.QueryRow(t.Context(), `
		SELECT c.type, c.last_message_id, c.last_activity_at, c.created_at,
		       d.user1_id, d.user2_id
		FROM chats c
		JOIN directs d ON d.chat_id = c.id
		WHERE c.id = $1
	`, expected.Chat.ID).Scan(
		&chatType,
		&lastMessageID,
		&lastActivity,
		&createdAt,
		&user1ID,
		&user2ID,
	)
	require.NoError(t, err)
	require.Equal(t, "direct", chatType)
	require.Equal(t, expected.Chat.LastMessageID, lastMessageID)
	require.True(t, expected.Chat.LastActivityAt.Equal(lastActivity))
	require.True(t, expected.Chat.CreatedAt.Equal(createdAt))
	require.Equal(t, expected.User1ID, user1ID)
	require.Equal(t, expected.User2ID, user2ID)
	require.Equal(t, 2, createDirectParticipantCount(t, pool, expected.Chat.ID))
}

func requireCreateDirectParticipantPersisted(
	t *testing.T,
	pool *pgxpool.Pool,
	expected domain.ChatParticipant,
) {
	t.Helper()

	var (
		lastReadMessageID *uuid.UUID
		joinedAt          time.Time
	)
	err := pool.QueryRow(t.Context(), `
		SELECT last_read_message_id, joined_at
		FROM chat_participants
		WHERE chat_id = $1 AND user_id = $2
	`, expected.ChatID, expected.UserID).Scan(&lastReadMessageID, &joinedAt)
	require.NoError(t, err)
	require.Equal(t, expected.LastReadMessageID, lastReadMessageID)
	require.True(t, expected.JoinedAt.Equal(joinedAt))
}

func createDirectRowCount(
	t *testing.T,
	pool *pgxpool.Pool,
	table string,
	chatID uuid.UUID,
) int {
	t.Helper()

	queries := map[string]string{
		"chats":   `SELECT count(*) FROM chats WHERE id = $1`,
		"directs": `SELECT count(*) FROM directs WHERE chat_id = $1`,
	}
	query, ok := queries[table]
	require.True(t, ok)
	var count int
	require.NoError(t, pool.QueryRow(t.Context(), query, chatID).Scan(&count))
	return count
}

func createDirectParticipantCount(
	t *testing.T,
	pool *pgxpool.Pool,
	chatID uuid.UUID,
) int {
	t.Helper()

	var count int
	err := pool.QueryRow(
		t.Context(),
		`SELECT count(*) FROM chat_participants WHERE chat_id = $1`,
		chatID,
	).Scan(&count)
	require.NoError(t, err)
	return count
}

func createDirectTestTime() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}
