//go:build integration

package chats_postgres_repository

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

func TestAddGroupParticipants(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("returns positional results for new duplicate and existing participants", func(t *testing.T) {
		repository, group, initialParticipants := newCreateGroupTestData(
			t,
			pool,
			config.Timeout,
		)
		require.NoError(t, repository.CreateGroup(t.Context(), group, initialParticipants))

		joinedAt := createDirectTestTime().Add(time.Minute)
		newUserID := uuid.New()
		insertAddGroupParticipantsTestUser(t, pool, config.Timeout, newUserID, joinedAt)
		newParticipant := newAddGroupRepositoryParticipant(
			t,
			group.Chat.ID,
			newUserID,
			joinedAt,
		)
		existingParticipant := initialParticipants[1]

		actual, err := repository.AddGroupParticipants(
			t.Context(),
			group.Chat.ID,
			[]domain.GroupParticipant{
				newParticipant,
				newParticipant,
				existingParticipant,
			},
		)

		require.NoError(t, err)
		require.Equal(t, []bool{true, false, false}, actual)
		require.Equal(t, len(initialParticipants)+1, createGroupParticipantCount(
			t,
			pool,
			group.Chat.ID,
		))

		var (
			persistedJoinedAt time.Time
			persistedRole     string
		)
		err = pool.QueryRow(t.Context(), `
			SELECT cp.joined_at, gp.role
			FROM chat_participants cp
			JOIN group_participants gp
			  ON gp.chat_id = cp.chat_id AND gp.user_id = cp.user_id
			WHERE cp.chat_id = $1 AND cp.user_id = $2
		`, group.Chat.ID, newUserID).Scan(&persistedJoinedAt, &persistedRole)
		require.NoError(t, err)
		require.True(t, joinedAt.Equal(persistedJoinedAt))
		require.Equal(t, string(domain.MemberRole), persistedRole)
	})

	t.Run("allows empty batch", func(t *testing.T) {
		repository, group, initialParticipants := newCreateGroupTestData(
			t,
			pool,
			config.Timeout,
		)
		require.NoError(t, repository.CreateGroup(t.Context(), group, initialParticipants))

		actual, err := repository.AddGroupParticipants(
			t.Context(),
			group.Chat.ID,
			[]domain.GroupParticipant{},
		)

		require.NoError(t, err)
		require.Empty(t, actual)
		require.Equal(t, len(initialParticipants), createGroupParticipantCount(
			t,
			pool,
			group.Chat.ID,
		))
	})

	t.Run("rejects participant from another chat before persistence", func(t *testing.T) {
		repository, group, initialParticipants := newCreateGroupTestData(
			t,
			pool,
			config.Timeout,
		)
		require.NoError(t, repository.CreateGroup(t.Context(), group, initialParticipants))
		participant := initialParticipants[0]
		participant.ChatID = uuid.New()

		actual, err := repository.AddGroupParticipants(
			t.Context(),
			group.Chat.ID,
			[]domain.GroupParticipant{participant},
		)

		require.EqualError(t, err, "participant ids and chat ids mismatch")
		require.Nil(t, actual)
		require.Equal(t, len(initialParticipants), createGroupParticipantCount(
			t,
			pool,
			group.Chat.ID,
		))
	})

	t.Run("rolls back whole batch when a participant user does not exist", func(t *testing.T) {
		repository, group, initialParticipants := newCreateGroupTestData(
			t,
			pool,
			config.Timeout,
		)
		require.NoError(t, repository.CreateGroup(t.Context(), group, initialParticipants))

		joinedAt := createDirectTestTime().Add(time.Minute)
		validUserID := uuid.New()
		insertAddGroupParticipantsTestUser(t, pool, config.Timeout, validUserID, joinedAt)
		validParticipant := newAddGroupRepositoryParticipant(
			t,
			group.Chat.ID,
			validUserID,
			joinedAt,
		)
		unknownParticipant := newAddGroupRepositoryParticipant(
			t,
			group.Chat.ID,
			uuid.New(),
			joinedAt,
		)

		actual, err := repository.AddGroupParticipants(
			t.Context(),
			group.Chat.ID,
			[]domain.GroupParticipant{validParticipant, unknownParticipant},
		)

		require.Error(t, err)
		require.True(t, postgres.IsConstraintViolation(
			err,
			postgres.ForeignKeyViolation,
			"chat_participants_user_id_fkey",
		))
		require.Nil(t, actual)
		require.Equal(t, len(initialParticipants), createGroupParticipantCount(
			t,
			pool,
			group.Chat.ID,
		))
	})
}

func newAddGroupRepositoryParticipant(
	t *testing.T,
	groupID, userID uuid.UUID,
	joinedAt time.Time,
) domain.GroupParticipant {
	t.Helper()
	participant, err := domain.NewGroupParticipant(
		groupID,
		userID,
		nil,
		joinedAt,
		domain.MemberRole,
	)
	require.NoError(t, err)
	return participant
}

func insertAddGroupParticipantsTestUser(
	t *testing.T,
	pool *pgxpool.Pool,
	timeout time.Duration,
	userID uuid.UUID,
	createdAt time.Time,
) {
	t.Helper()
	insertCreateDirectTestUser(t, pool, userID, createdAt)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		require.NoError(t, err)
	})
}
