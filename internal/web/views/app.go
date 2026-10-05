package web_views

import (
	"strings"
	"unicode/utf8"
)

type Person struct {
	ID, Username, FirstName, LastName, Bio, Name, Initials string
}

type ChatRow struct {
	ID, Title, Initials, Preview, Time string
	Unread                             int
	Group                              bool
}

type MessageRow struct {
	ID, Sender, Initials, Content, Time, Day string
	Mine, Edited, NewDay                     bool
}

type FeedData struct {
	ChatID, Cursor, OlderURL, LastID string
	Messages                         []MessageRow
}

type AppData struct {
	Me                                                    Person
	Chats                                                 []ChatRow
	MoreChatsURL, Active, Screen, Title, Subtitle, PeerID string
	Group                                                 bool
	Feed                                                  FeedData
	ClientMessageID, Draft, Notice                        string
}

type ProfileData struct {
	Person  Person
	Message string
	Errors  map[string]string
	Saved   bool
}

type GroupMember struct {
	ID, Username, Name, Initials, Role string
	CanRemove                          bool
}
type GroupData struct {
	ID, Title, Message, MoreURL string
	Members                     []GroupMember
	Manage, CanLeave            bool
	PendingUsernames            []string
}

func GroupUsernames(members []GroupMember) []string {
	names := make([]string, 0, len(members))
	for _, member := range members {
		names = append(names, member.Username)
	}
	return names
}

func Initials(name string) string {
	parts := strings.Fields(name)
	result := ""
	for i, part := range parts {
		if i == 2 {
			break
		}
		r, _ := utf8.DecodeRuneInString(part)
		result += string(r)
	}
	return strings.ToUpper(result)
}
