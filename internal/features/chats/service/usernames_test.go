package chats_service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/wrzdx/Nero/internal/core/domain"
)

func TestNormalizeUsernames(t *testing.T) {
	names, err := NormalizeUsernames([]string{" @Daniel_k ", "DANIEL_K", "anya_1"})
	require.NoError(t, err)
	require.Equal(t, []string{"daniel_k", "daniel_k", "anya_1"}, names)
	for _, name := range []string{"", "tiny", "@@daniel_k", "имя_123", "has space", "bad-name", strings.Repeat("a", 33)} {
		_, err := NormalizeUsernames([]string{name})
		require.ErrorIs(t, err, ErrInvalidInput, name)
	}
	_, err = NormalizeUsernames(make([]string, 101))
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateDirectByUsername(t *testing.T) {
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	peerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	t.Run("resolves username and returns existing stable chat", func(t *testing.T) {
		ctx := t.Context()
		users, chats, tx := NewMockUsersRepository(t), NewMockChatsRepository(t), NewMockTXManager(t)
		users.EXPECT().GetUserByUsername(ctx, "peer_user").Return(newCreateDirectTestUser(t, peerID, nil), nil).Once()
		expectCreateDirectActiveUsers(t, users, ctx, requesterID, peerID)
		expectCreateDirectTransaction(tx, ctx, ctx)
		chats.EXPECT().CreateDirect(ctx, mock.Anything, mock.Anything, mock.Anything).Return(domain.ErrAlreadyExists)
		existing, err := domain.NewDirectChat(uuid.New(), requesterID, peerID, time.Now())
		require.NoError(t, err)
		chats.EXPECT().GetDirectByUsers(ctx, requesterID, peerID).Return(existing, nil)
		actual, created, err := NewChatsService(chats, users, tx).CreateDirectByUsername(ctx, requesterID, " @PEER_USER ")
		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, existing, actual)
	})
	t.Run("rejects self chat after resolving name", func(t *testing.T) {
		users := NewMockUsersRepository(t)
		users.EXPECT().GetUserByUsername(t.Context(), "my_user").Return(newCreateDirectTestUser(t, requesterID, nil), nil)
		_, _, err := NewChatsService(NewMockChatsRepository(t), users, NewMockTXManager(t)).CreateDirectByUsername(t.Context(), requesterID, "my_user")
		require.ErrorIs(t, err, domain.ErrInvalidDirectChat)
	})
	deletedAt := time.Now()
	dbErr := errors.New("database unavailable")
	for _, tc := range []struct {
		name                string
		user                domain.User
		lookupErr, expected error
	}{
		{"missing", domain.User{}, domain.ErrNotFound, domain.ErrNotFound},
		{"deleted", newCreateDirectTestUser(t, peerID, &deletedAt), nil, domain.ErrNotFound},
		{"database failure", domain.User{}, dbErr, dbErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users := NewMockUsersRepository(t)
			users.EXPECT().GetUserByUsername(t.Context(), "peer_user").Return(tc.user, tc.lookupErr)
			_, created, err := NewChatsService(NewMockChatsRepository(t), users, NewMockTXManager(t)).CreateDirectByUsername(t.Context(), requesterID, "peer_user")
			require.ErrorIs(t, err, tc.expected)
			require.False(t, created)
		})
	}
}

func TestCreateGroupByUsernames(t *testing.T) {
	creatorID, memberID := uuid.New(), uuid.New()
	t.Run("deduplicates members and automatically adds owner", func(t *testing.T) {
		ctx := t.Context()
		users, chats, tx := NewMockUsersRepository(t), NewMockChatsRepository(t), NewMockTXManager(t)
		users.EXPECT().GetUserByUsername(ctx, "owner_user").Return(newCreateDirectTestUser(t, creatorID, nil), nil).Once()
		users.EXPECT().GetUserByUsername(ctx, "member_user").Return(newCreateDirectTestUser(t, memberID, nil), nil).Once()
		chats.EXPECT().GetParticipantsStatus(ctx, []uuid.UUID{memberID}).Return([]ParticipantStatus{{UserID: memberID, Found: true}}, nil)
		users.EXPECT().GetUserForUpdate(ctx, creatorID).Return(newCreateDirectTestUser(t, creatorID, nil), nil)
		chats.EXPECT().CreateGroup(ctx, mock.Anything, mock.Anything).Run(func(_ context.Context, _ domain.GroupChat, members []domain.GroupParticipant) {
			require.Len(t, members, 2)
			require.Equal(t, creatorID, members[0].UserID)
			require.Equal(t, domain.OwnerRole, members[0].Role())
			require.Equal(t, memberID, members[1].UserID)
		}).Return(nil)
		expectCreateGroupTransaction(tx, ctx, ctx)
		_, err := NewChatsService(chats, users, tx).CreateGroupByUsernames(ctx, creatorID, CreateGroupByUsernamesCommand{Title: "Team", ParticipantUsernames: []string{"@owner_user", "Member_User", "@MEMBER_USER"}})
		require.NoError(t, err)
	})
	t.Run("missing user prevents any group write", func(t *testing.T) {
		users := NewMockUsersRepository(t)
		users.EXPECT().GetUserByUsername(t.Context(), "missing_user").Return(domain.User{}, domain.ErrNotFound)
		_, err := NewChatsService(NewMockChatsRepository(t), users, NewMockTXManager(t)).CreateGroupByUsernames(t.Context(), creatorID, CreateGroupByUsernamesCommand{Title: "Team", ParticipantUsernames: []string{"@Missing_User"}})
		require.ErrorIs(t, err, domain.ErrNotFound)
		var detailed domain.DetailedError
		require.ErrorAs(t, err, &detailed)
		require.Equal(t, map[string]string{"missing_user": "user unavailable"}, detailed.Details)
	})
}

func TestAddGroupParticipantsByUsernames(t *testing.T) {
	groupID, requesterID, memberID := uuid.New(), uuid.New(), uuid.New()
	t.Run("preserves mixed input order and caches repeated lookups", func(t *testing.T) {
		ctx := t.Context()
		users, chats, tx := NewMockUsersRepository(t), NewMockChatsRepository(t), NewMockTXManager(t)
		users.EXPECT().GetUserByUsername(ctx, "member_user").Return(newCreateDirectTestUser(t, memberID, nil), nil).Once()
		users.EXPECT().GetUserByUsername(ctx, "missing_user").Return(domain.User{}, domain.ErrNotFound).Once()
		deletedAt := time.Now()
		users.EXPECT().GetUserByUsername(ctx, "deleted_user").Return(newCreateDirectTestUser(t, uuid.New(), &deletedAt), nil).Once()
		expectActiveAddGroupRequester(chats, ctx, requesterID)
		chats.EXPECT().GetGroupParticipant(ctx, groupID, requesterID).Return(newAddGroupRequester(t, groupID, requesterID, domain.OwnerRole), nil)
		chats.EXPECT().GetParticipantsStatus(ctx, []uuid.UUID{memberID, memberID}).Return([]ParticipantStatus{{UserID: memberID, Found: true}}, nil)
		chats.EXPECT().AddGroupParticipants(ctx, groupID, mock.Anything).RunAndReturn(func(_ context.Context, _ uuid.UUID, members []domain.GroupParticipant) ([]bool, error) {
			require.Len(t, members, 2)
			require.Equal(t, memberID, members[0].UserID)
			require.Equal(t, memberID, members[1].UserID)
			return []bool{true, false}, nil
		})
		expectAddGroupParticipantsTransaction(tx, ctx, ctx)
		actual, err := NewChatsService(chats, users, tx).AddGroupParticipantsByUsernames(ctx, AddGroupParticipantsByUsernamesCommand{GroupID: groupID, RequesterID: requesterID, ParticipantUsernames: []string{"@Member_User", "missing_user", "MEMBER_USER", "deleted_user"}})
		require.NoError(t, err)
		require.Equal(t, []UsernameParticipantResult{{"member_user", Added}, {"missing_user", Unavailable}, {"member_user", AlreadyMember}, {"deleted_user", Unavailable}}, actual)
	})
	t.Run("all unknown names still require management rights", func(t *testing.T) {
		ctx := t.Context()
		users, chats := NewMockUsersRepository(t), NewMockChatsRepository(t)
		users.EXPECT().GetUserByUsername(ctx, "missing_user").Return(domain.User{}, domain.ErrNotFound)
		expectActiveAddGroupRequester(chats, ctx, requesterID)
		chats.EXPECT().GetGroupParticipant(ctx, groupID, requesterID).Return(newAddGroupRequester(t, groupID, requesterID, domain.MemberRole), nil)
		result, err := NewChatsService(chats, users, NewMockTXManager(t)).AddGroupParticipantsByUsernames(ctx, AddGroupParticipantsByUsernamesCommand{GroupID: groupID, RequesterID: requesterID, ParticipantUsernames: []string{"missing_user"}})
		require.ErrorIs(t, err, ErrNotEnoughRights)
		require.Nil(t, result)
	})
}

func TestRemoveGroupParticipantByUsernameAllowsDeletedTarget(t *testing.T) {
	ctx := t.Context()
	groupID, requesterID, targetID := uuid.New(), uuid.New(), uuid.New()
	users, chats, tx := NewMockUsersRepository(t), NewMockChatsRepository(t), NewMockTXManager(t)
	deletedAt := time.Now()
	users.EXPECT().GetUserByUsername(ctx, "deleted_target").Return(newCreateDirectTestUser(t, targetID, &deletedAt), nil)
	owner := newAddGroupRequester(t, groupID, requesterID, domain.OwnerRole)
	target := newAddGroupRequester(t, groupID, targetID, domain.MemberRole)
	chats.EXPECT().GetParticipantsStatus(ctx, []uuid.UUID{requesterID}).Return([]ParticipantStatus{{UserID: requesterID, Found: true}}, nil)
	chats.EXPECT().GetGroupParticipant(ctx, groupID, requesterID).Return(owner, nil)
	chats.EXPECT().GetGroupParticipant(ctx, groupID, targetID).Return(target, nil)
	chats.EXPECT().RemoveGroupParticipant(ctx, target).Return(nil)
	expectCreateDirectTransaction(tx, ctx, ctx)
	err := NewChatsService(chats, users, tx).RemoveGroupParticipantByUsername(ctx, RemoveGroupParticipantByUsernameCommand{GroupID: groupID, RequesterID: requesterID, TargetUsername: "@DELETED_TARGET"})
	require.NoError(t, err)
}
