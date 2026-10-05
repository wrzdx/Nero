//go:build integration

package chats_postgres_repository

import (
	"context"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetParticipantsStatus(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	repository := NewChatsRepository(pool, config.Timeout)
	activeUserID := uuid.New()
	deletedUserID := uuid.New()
	unknownUserID := uuid.New()
	createdAt := createDirectTestTime()
	insertCreateDirectTestUser(t, pool, activeUserID, createdAt)
	insertCreateDirectTestUser(t, pool, deletedUserID, createdAt)
	deletedAt := createdAt.Add(time.Minute)
	_, err = pool.Exec(
		t.Context(),
		`UPDATE users SET deleted_at = $1 WHERE id = $2`,
		deletedAt,
		deletedUserID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
		defer cancel()
		_, err := pool.Exec(
			ctx,
			`DELETE FROM users WHERE id IN ($1, $2)`,
			activeUserID,
			deletedUserID,
		)
		require.NoError(t, err)
	})

	statuses, err := repository.GetParticipantsStatus(
		t.Context(),
		[]uuid.UUID{unknownUserID, activeUserID, deletedUserID},
	)

	require.NoError(t, err)
	require.Len(t, statuses, 3)
	statusByUserID := make(map[uuid.UUID]bool, len(statuses))
	for _, status := range statuses {
		statusByUserID[status.UserID] = status.Found
	}
	require.Equal(t, map[uuid.UUID]bool{
		unknownUserID: false,
		activeUserID:  true,
		deletedUserID: false,
	}, statusByUserID)
}
