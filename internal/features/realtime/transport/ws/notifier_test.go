package realtime_transport_ws

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/logger"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func notifierTestMessage() domain.Message {
	return domain.Message{
		ID: uuid.New(), ChatID: uuid.New(), SenderID: uuid.New(), ClientMessageID: uuid.New(),
		Content: "Hello", CreatedAt: time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC),
	}
}

func TestNotifierMessageCreatedDeliversToParticipantsIncludingSender(t *testing.T) {
	message := notifierTestMessage()
	updated := message.CreatedAt.Add(time.Minute)
	message.UpdatedAt = &updated
	recipients := NewMockParticipantsRepository(t)
	peerID := uuid.New()
	recipients.EXPECT().GetParticipants(t.Context(), message.ChatID).
		Return([]uuid.UUID{message.SenderID, peerID}, nil).Once()
	hub := NewHub()
	sender, peer, outsider := queuedClient(t, 1), queuedClient(t, 1), queuedClient(t, 1)
	hub.Register(message.SenderID, sender)
	hub.Register(peerID, peer)
	hub.Register(uuid.New(), outsider)
	NewNotifier(hub, recipients, logger.NewTestLogger()).MessageCreated(t.Context(), message)
	for _, client := range []*Client{sender, peer} {
		require.Len(t, client.send, 1)
		var event struct {
			Type string  `json:"type"`
			Data Message `json:"data"`
		}
		require.NoError(t, json.Unmarshal(<-client.send, &event))
		require.Equal(t, "message_created", event.Type)
		require.Equal(t, Message{
			ID: message.ID, ChatID: message.ChatID, SenderID: message.SenderID,
			Content: message.Content, CreatedAt: message.CreatedAt, UpdatedAt: message.UpdatedAt,
		}, event.Data)
	}
	require.Empty(t, outsider.send)
}

func TestNotifierLogsRecipientLookupFailureAndDoesNotPublish(t *testing.T) {
	message := notifierTestMessage()
	recipients := NewMockParticipantsRepository(t)
	recipients.EXPECT().GetParticipants(t.Context(), message.ChatID).
		Return([]uuid.UUID{message.SenderID}, errors.New("database unavailable")).Once()
	hub := NewHub()
	client := queuedClient(t, 1)
	hub.Register(message.SenderID, client)
	core, logs := observer.New(zap.ErrorLevel)
	NewNotifier(hub, recipients, &logger.Logger{Logger: zap.New(core)}).MessageCreated(t.Context(), message)
	require.Empty(t, client.send)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "get message notification recipients", entries[0].Message)
	fields := entries[0].ContextMap()
	require.Equal(t, "database unavailable", fields["error"])
	require.Equal(t, message.ChatID.String(), fields["chat_id"])
	require.Equal(t, message.ID.String(), fields["message_id"])
}

func TestNotifierLogsPublishFailure(t *testing.T) {
	message := notifierTestMessage()
	// time.Time rejects out-of-range years during JSON encoding. This exercises
	// a real hub error without adding an interface only for testing.
	message.CreatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	recipients := NewMockParticipantsRepository(t)
	recipients.EXPECT().GetParticipants(t.Context(), message.ChatID).
		Return([]uuid.UUID{message.SenderID}, nil).Once()
	hub := NewHub()
	client := queuedClient(t, 1)
	hub.Register(message.SenderID, client)
	core, logs := observer.New(zap.ErrorLevel)
	NewNotifier(hub, recipients, &logger.Logger{Logger: zap.New(core)}).MessageCreated(t.Context(), message)
	require.Empty(t, client.send)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "publish message event", entries[0].Message)
	require.Equal(t, message.SenderID.String(), entries[0].ContextMap()["recipient_id"])
	require.Contains(t, entries[0].ContextMap()["error"], "marshal event")
}

func TestNotifierWithNoRecipientsDoesNotLogError(t *testing.T) {
	message := notifierTestMessage()
	recipients := NewMockParticipantsRepository(t)
	recipients.EXPECT().GetParticipants(t.Context(), message.ChatID).Return(nil, nil).Once()
	core, logs := observer.New(zap.ErrorLevel)
	NewNotifier(NewHub(), recipients, &logger.Logger{Logger: zap.New(core)}).MessageCreated(t.Context(), message)
	require.Zero(t, logs.Len())
}

func TestNotifierMessageChangesOverWebSocket(t *testing.T) {
	message := notifierTestMessage()
	hub, dial, done, _ := deliveryServer(t, message.SenderID)
	first, second := dial(), dial()
	recipients := NewMockParticipantsRepository(t)
	recipients.EXPECT().GetParticipants(t.Context(), message.ChatID).
		Return([]uuid.UUID{message.SenderID}, nil).Times(2)
	notifier := NewNotifier(hub, recipients, logger.NewTestLogger())
	updatedAt := message.CreatedAt.Add(time.Minute)
	message.Content = "edited text"
	message.UpdatedAt = &updatedAt
	notifier.MessageEdited(t.Context(), message)
	for _, conn := range []*websocket.Conn{first, second} {
		var event struct {
			Type string  `json:"type"`
			Data Message `json:"data"`
		}
		readJSON(t, conn, &event)
		require.Equal(t, "message_edited", event.Type)
		require.Equal(t, Message{
			ID: message.ID, ChatID: message.ChatID, SenderID: message.SenderID,
			Content: "edited text", CreatedAt: message.CreatedAt, UpdatedAt: &updatedAt,
		}, event.Data)
	}
	notifier.MessageDeleted(t.Context(), message.ChatID, message.ID)
	for _, conn := range []*websocket.Conn{first, second} {
		var event struct {
			Type string            `json:"type"`
			Data map[string]string `json:"data"`
		}
		readJSON(t, conn, &event)
		require.Equal(t, "message_deleted", event.Type)
		require.Equal(t, map[string]string{
			"id": message.ID.String(), "chat_id": message.ChatID.String(),
		}, event.Data)
		_ = conn.CloseNow()
		waitHandler(t, done)
	}
}

func TestNotifierMessageChangesLogLookupFailure(t *testing.T) {
	for _, kind := range []string{"message_edited", "message_deleted"} {
		t.Run(kind, func(t *testing.T) {
			message := notifierTestMessage()
			recipients := NewMockParticipantsRepository(t)
			recipients.EXPECT().GetParticipants(t.Context(), message.ChatID).
				Return(nil, errors.New("lookup failed")).Once()
			core, logs := observer.New(zap.ErrorLevel)
			notifier := NewNotifier(NewHub(), recipients, &logger.Logger{Logger: zap.New(core)})
			if kind == "message_edited" {
				notifier.MessageEdited(t.Context(), message)
			} else {
				notifier.MessageDeleted(t.Context(), message.ChatID, message.ID)
			}
			entries := logs.All()
			require.Len(t, entries, 1)
			require.Equal(t, "get message notification recipients", entries[0].Message)
			fields := entries[0].ContextMap()
			require.Equal(t, kind, fields["event_type"])
			require.Equal(t, "lookup failed", fields["error"])
			require.Equal(t, message.ID.String(), fields["message_id"])
			require.Equal(t, message.ChatID.String(), fields["chat_id"])
		})
	}
}
