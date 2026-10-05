//go:build integration

package chats_postgres_repository

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestListGroupParticipants(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	t.Run("returns ordered participant information including deleted account", func(t *testing.T) {
		fixture := newGroupParticipantsRepositoryFixture(t, pool, config.Timeout)
		expected := fixture.expectedParticipantInfos(t)

		actual, err := fixture.repository.ListGroupParticipants(
			t.Context(),
			fixture.group.Chat.ID,
			nil,
			10,
		)

		require.NoError(t, err)
		requireParticipantInfosEqual(t, expected, actual)

		var deletedInfo *chats_service.ParticipantInfo
		for index := range actual {
			if actual[index].ID == fixture.deletedUserID {
				deletedInfo = &actual[index]
				break
			}
		}
		require.NotNil(t, deletedInfo)
		require.Equal(t, "Deleted Account", deletedInfo.FirstName)
		require.Nil(t, deletedInfo.LastName)
	})

	t.Run("applies composite cursor when joined times are equal", func(t *testing.T) {
		fixture := newGroupParticipantsRepositoryFixture(t, pool, config.Timeout)
		expected := fixture.expectedParticipantInfos(t)
		require.True(t, expected[0].JoinedAt.Equal(expected[1].JoinedAt))

		firstPage, err := fixture.repository.ListGroupParticipants(
			t.Context(),
			fixture.group.Chat.ID,
			nil,
			1,
		)
		require.NoError(t, err)
		requireParticipantInfosEqual(t, expected[:1], firstPage)

		secondPage, err := fixture.repository.ListGroupParticipants(
			t.Context(),
			fixture.group.Chat.ID,
			&chats_service.GroupParticipantCursor{
				ParticipantID: firstPage[0].ID,
				JoinedAt:      firstPage[0].JoinedAt,
			},
			1,
		)

		require.NoError(t, err)
		requireParticipantInfosEqual(t, expected[1:2], secondPage)
	})

	t.Run("returns empty list after last participant", func(t *testing.T) {
		fixture := newGroupParticipantsRepositoryFixture(t, pool, config.Timeout)
		expected := fixture.expectedParticipantInfos(t)
		last := expected[len(expected)-1]

		actual, err := fixture.repository.ListGroupParticipants(
			t.Context(),
			fixture.group.Chat.ID,
			&chats_service.GroupParticipantCursor{
				ParticipantID: last.ID,
				JoinedAt:      last.JoinedAt,
			},
			10,
		)

		require.NoError(t, err)
		require.Empty(t, actual)
	})
}

type groupParticipantsRepositoryFixture struct {
	repository    *ChatsRepository
	group         domain.GroupChat
	participants  []domain.GroupParticipant
	profiles      map[uuid.UUID]domain.UserProfile
	deletedUserID uuid.UUID
}

func newGroupParticipantsRepositoryFixture(
	t *testing.T,
	pool *pgxpool.Pool,
	timeout time.Duration,
) groupParticipantsRepositoryFixture {
	t.Helper()

	createdAt := createDirectTestTime()
	userIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	lastName := "Participant"
	profiles := make(map[uuid.UUID]domain.UserProfile, len(userIDs))
	for index, userID := range userIDs {
		var userLastName *string
		if index == 1 {
			userLastName = &lastName
		}
		profile, err := domain.NewUserProfile(
			groupParticipantUsername(userID),
			[]string{"Owner", "Admin", "Member"}[index],
			userLastName,
			nil,
		)
		require.NoError(t, err)
		insertGroupParticipantTestUser(t, pool, userID, profile, createdAt)
		profiles[userID] = profile
	}

	group, err := domain.NewGroupChat(uuid.New(), "Participants", createdAt)
	require.NoError(t, err)
	roles := []domain.GroupRole{domain.OwnerRole, domain.AdminRole, domain.MemberRole}
	participants := make([]domain.GroupParticipant, 0, len(userIDs))
	for index, userID := range userIDs {
		joinedAt := createdAt.Add(time.Minute)
		if index == 2 {
			joinedAt = createdAt
		}
		participant, err := domain.NewGroupParticipant(
			group.Chat.ID,
			userID,
			nil,
			joinedAt,
			roles[index],
		)
		require.NoError(t, err)
		participants = append(participants, participant)
	}

	repository := NewChatsRepository(pool, timeout)
	require.NoError(t, repository.CreateGroup(t.Context(), group, participants))

	deletedUserID := userIDs[2]
	deletedProfile, err := domain.NewUserProfile(
		"deleted_"+strings.ReplaceAll(deletedUserID.String(), "-", "")[:16],
		"Deleted Account",
		nil,
		nil,
	)
	require.NoError(t, err)
	deletedAt := createdAt.Add(2 * time.Minute)
	_, err = pool.Exec(t.Context(), `
		UPDATE users
		SET username = $1,
		    first_name = $2,
		    last_name = NULL,
		    bio = NULL,
		    deleted_at = $3
		WHERE id = $4
	`, deletedProfile.Username, deletedProfile.FirstName, deletedAt, deletedUserID)
	require.NoError(t, err)
	profiles[deletedUserID] = deletedProfile

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

	return groupParticipantsRepositoryFixture{
		repository:    repository,
		group:         group,
		participants:  participants,
		profiles:      profiles,
		deletedUserID: deletedUserID,
	}
}

func (f groupParticipantsRepositoryFixture) expectedParticipantInfos(
	t *testing.T,
) []chats_service.ParticipantInfo {
	t.Helper()

	participants := append([]domain.GroupParticipant(nil), f.participants...)
	sort.Slice(participants, func(i, j int) bool {
		if participants[i].JoinedAt.Equal(participants[j].JoinedAt) {
			return bytes.Compare(participants[i].UserID[:], participants[j].UserID[:]) > 0
		}
		return participants[i].JoinedAt.After(participants[j].JoinedAt)
	})

	infos := make([]chats_service.ParticipantInfo, 0, len(participants))
	for _, participant := range participants {
		info, err := chats_service.NewParticipantInfo(
			participant.ChatParticipant,
			participant.Role(),
			f.profiles[participant.UserID],
		)
		require.NoError(t, err)
		infos = append(infos, info)
	}
	return infos
}

func insertGroupParticipantTestUser(
	t *testing.T,
	pool *pgxpool.Pool,
	userID uuid.UUID,
	profile domain.UserProfile,
	createdAt time.Time,
) {
	t.Helper()

	_, err := pool.Exec(t.Context(), `
		INSERT INTO users (
			id, username, first_name, last_name, bio, created_at, password_hash
		)
		VALUES ($1, $2, $3, $4, $5, $6, 'password_hash')
	`,
		userID,
		profile.Username,
		profile.FirstName,
		profile.LastName,
		profile.Bio,
		createdAt,
	)
	require.NoError(t, err)
}

func groupParticipantUsername(userID uuid.UUID) string {
	return "group_" + strings.ReplaceAll(userID.String(), "-", "")[:16]
}

func requireParticipantInfosEqual(
	t *testing.T,
	expected []chats_service.ParticipantInfo,
	actual []chats_service.ParticipantInfo,
) {
	t.Helper()

	require.Len(t, actual, len(expected))
	for index := range expected {
		require.Equal(t, expected[index].ID, actual[index].ID)
		require.Equal(t, expected[index].Username, actual[index].Username)
		require.Equal(t, expected[index].FirstName, actual[index].FirstName)
		require.Equal(t, expected[index].LastName, actual[index].LastName)
		require.Equal(t, expected[index].Role, actual[index].Role)
		require.True(t, expected[index].JoinedAt.Equal(actual[index].JoinedAt))
	}
}
