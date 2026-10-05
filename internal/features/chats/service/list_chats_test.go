package chats_service

import (
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestListChats(t *testing.T) {
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	baseTime := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)

	t.Run("uses default limit and returns non nil empty page", func(t *testing.T) {
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			ListChats(mock.Anything, requesterID, (*ChatCursor)(nil), 51).
			Return(nil, nil)
		service := NewChatsService(repository, nil, nil)

		page, err := service.ListChats(t.Context(), requesterID, ListChatsQuery{})

		require.NoError(t, err)
		require.NotNil(t, page.Chats)
		require.Empty(t, page.Chats)
		require.Nil(t, page.NextCursor)
	})

	t.Run("requests one extra item and builds cursor from last visible chat", func(t *testing.T) {
		first := newListChatsTestGroupItem(t, baseTime.Add(2*time.Minute), false)
		second := newListChatsTestDirectItem(t, requesterID, baseTime.Add(time.Minute), true)
		extra := newListChatsTestGroupItem(t, baseTime, false)
		before := &ChatCursor{ChatID: uuid.New(), LastActivityAt: baseTime.Add(time.Hour)}
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			ListChats(mock.Anything, requesterID, before, 3).
			Return([]ChatItem{first, second, extra}, nil)
		service := NewChatsService(repository, nil, nil)

		page, err := service.ListChats(t.Context(), requesterID, ListChatsQuery{
			Before: before,
			Limit:  2,
		})

		require.NoError(t, err)
		require.Equal(t, []ChatItem{first, second}, page.Chats)
		require.Equal(t, &ChatCursor{
			ChatID:         second.Chat.ID,
			LastActivityAt: second.Chat.LastActivityAt,
		}, page.NextCursor)
	})

	t.Run("does not return cursor for complete page", func(t *testing.T) {
		item := newListChatsTestGroupItem(t, baseTime, false)
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			ListChats(mock.Anything, requesterID, (*ChatCursor)(nil), 2).
			Return([]ChatItem{item}, nil)
		service := NewChatsService(repository, nil, nil)

		page, err := service.ListChats(
			t.Context(), requesterID, ListChatsQuery{Limit: 1},
		)

		require.NoError(t, err)
		require.Equal(t, []ChatItem{item}, page.Chats)
		require.Nil(t, page.NextCursor)
	})

	t.Run("rejects nil requester before repository call", func(t *testing.T) {
		service := NewChatsService(NewMockChatsRepository(t), nil, nil)

		page, err := service.ListChats(t.Context(), uuid.Nil, ListChatsQuery{})

		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Empty(t, page)
	})

	testCases := []struct {
		name  string
		query ListChatsQuery
	}{
		{name: "negative limit", query: ListChatsQuery{Limit: -1}},
		{name: "limit above maximum", query: ListChatsQuery{Limit: 101}},
		{
			name: "nil cursor chat id",
			query: ListChatsQuery{Before: &ChatCursor{
				LastActivityAt: baseTime,
			}},
		},
		{
			name: "zero cursor activity",
			query: ListChatsQuery{Before: &ChatCursor{
				ChatID: uuid.New(),
			}},
		},
	}
	for _, testCase := range testCases {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			service := NewChatsService(NewMockChatsRepository(t), nil, nil)

			page, err := service.ListChats(t.Context(), requesterID, testCase.query)

			require.ErrorIs(t, err, ErrInvalidInput)
			require.Empty(t, page)
		})
	}

	t.Run("returns repository error", func(t *testing.T) {
		repositoryErr := errors.New("database unavailable")
		repository := NewMockChatsRepository(t)
		repository.EXPECT().
			ListChats(mock.Anything, requesterID, (*ChatCursor)(nil), 51).
			Return(nil, repositoryErr)
		service := NewChatsService(repository, nil, nil)

		page, err := service.ListChats(t.Context(), requesterID, ListChatsQuery{})

		require.ErrorIs(t, err, repositoryErr)
		require.Empty(t, page)
	})
}

func TestChatItemValidate(t *testing.T) {
	requesterID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	activityAt := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)

	t.Run("accepts valid direct with last message", func(t *testing.T) {
		item := newListChatsTestDirectItem(t, requesterID, activityAt, true)

		require.NoError(t, item.Validate())
	})

	t.Run("accepts valid group without last message", func(t *testing.T) {
		item := newListChatsTestGroupItem(t, activityAt, false)

		require.NoError(t, item.Validate())
	})

	t.Run("rejects invalid chat", func(t *testing.T) {
		item := newListChatsTestGroupItem(t, activityAt, false)
		item.Chat.ID = uuid.Nil

		require.ErrorIs(t, item.Validate(), ErrInvalidChatItem)
	})

	t.Run("rejects direct without peer", func(t *testing.T) {
		item := newListChatsTestDirectItem(t, requesterID, activityAt, false)
		item.DirectPeer = nil

		require.ErrorIs(t, item.Validate(), ErrInvalidChatItem)
	})

	t.Run("rejects group without group info", func(t *testing.T) {
		item := newListChatsTestGroupItem(t, activityAt, false)
		item.GroupInfo = nil

		require.ErrorIs(t, item.Validate(), ErrInvalidChatItem)
	})

	t.Run("rejects data for both chat types", func(t *testing.T) {
		item := newListChatsTestDirectItem(t, requesterID, activityAt, false)
		group := newListChatsTestGroupItem(t, activityAt, false)
		item.GroupInfo = group.GroupInfo

		require.ErrorIs(t, item.Validate(), ErrInvalidChatItem)
	})

	t.Run("rejects missing last message", func(t *testing.T) {
		item := newListChatsTestDirectItem(t, requesterID, activityAt, true)
		item.LastMessage = nil

		require.ErrorIs(t, item.Validate(), ErrInvalidChatItem)
	})

	t.Run("rejects unexpected last message", func(t *testing.T) {
		item := newListChatsTestDirectItem(t, requesterID, activityAt, true)
		item.Chat.LastMessageID = nil

		require.ErrorIs(t, item.Validate(), ErrInvalidChatItem)
	})

	t.Run("rejects mismatched last message id", func(t *testing.T) {
		item := newListChatsTestDirectItem(t, requesterID, activityAt, true)
		otherMessageID := uuid.New()
		item.Chat.LastMessageID = &otherMessageID

		require.ErrorIs(t, item.Validate(), ErrInvalidChatItem)
	})

	t.Run("rejects message from another chat", func(t *testing.T) {
		item := newListChatsTestDirectItem(t, requesterID, activityAt, true)
		item.LastMessage.Message.ChatID = uuid.New()

		require.ErrorIs(t, item.Validate(), ErrInvalidChatItem)
	})
}

func newListChatsTestDirectItem(
	t *testing.T,
	requesterID uuid.UUID,
	activityAt time.Time,
	withMessage bool,
) ChatItem {
	t.Helper()

	peerID := uuid.New()
	direct, err := domain.NewDirectChat(
		uuid.New(), requesterID, peerID, activityAt.Add(-time.Hour),
	)
	require.NoError(t, err)
	peer, err := PeerFromUser(newListChatsTestUser(t, peerID, "Peer_123", "Peer"))
	require.NoError(t, err)
	item := ChatItem{Chat: direct.Chat, DirectPeer: &peer}
	if withMessage {
		message, err := domain.NewMessage(
			uuid.New(), uuid.New(), direct.Chat.ID, requesterID, "hello", activityAt,
		)
		require.NoError(t, err)
		item.Chat.LastMessageID = &message.ID
		item.Chat.LastActivityAt = message.CreatedAt
		profile, err := domain.NewUserProfile("Sender_123", "Sender", nil, nil)
		require.NoError(t, err)
		lastMessage, err := NewLastMessageItem(message, profile)
		require.NoError(t, err)
		item.LastMessage = &lastMessage
	} else {
		item.Chat.LastActivityAt = activityAt
	}
	return item
}

func newListChatsTestGroupItem(
	t *testing.T,
	activityAt time.Time,
	withMessage bool,
) ChatItem {
	t.Helper()

	group, err := domain.NewGroupChat(uuid.New(), "Backend", activityAt.Add(-time.Hour))
	require.NoError(t, err)
	groupInfo, err := GroupInfoFromGroup(group)
	require.NoError(t, err)
	item := ChatItem{Chat: group.Chat, GroupInfo: &groupInfo}
	if withMessage {
		senderID := uuid.New()
		message, err := domain.NewMessage(
			uuid.New(), uuid.New(), group.Chat.ID, senderID, "group message", activityAt,
		)
		require.NoError(t, err)
		item.Chat.LastMessageID = &message.ID
		item.Chat.LastActivityAt = message.CreatedAt
		profile, err := domain.NewUserProfile("Author_123", "Author", nil, nil)
		require.NoError(t, err)
		lastMessage, err := NewLastMessageItem(message, profile)
		require.NoError(t, err)
		item.LastMessage = &lastMessage
	} else {
		item.Chat.LastActivityAt = activityAt
	}
	return item
}

func newListChatsTestUser(
	t *testing.T,
	id uuid.UUID,
	username string,
	firstName string,
) domain.User {
	t.Helper()

	profile, err := domain.NewUserProfile(username, firstName, nil, nil)
	require.NoError(t, err)
	user, err := domain.NewUser(
		id,
		profile,
		time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC),
		nil,
		"password_hash",
	)
	require.NoError(t, err)
	return user
}
