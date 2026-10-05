package realtime_transport_ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/logger"
	http_middleware "github.com/wrzdx/Nero/internal/core/transport/http/middleware"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The wrapper observes handler completion as well as network closure: hijacked
// WebSockets are not waited for by httptest.Server.Close.
func deliveryServer(t *testing.T, userID uuid.UUID) (*Hub, func() *websocket.Conn, <-chan struct{}, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	hub := NewHub()
	provider := tokenProviderStub{parse: func(string) (auth.ParsedAccessToken, error) {
		return auth.ParsedAccessToken{
			AccessTokenClaims: auth.AccessTokenClaims{UserID: userID},
			ExpiresAt:         time.Now().Add(time.Minute),
		}, nil
	}}
	handler := NewWSHandler(ctx, provider, hub)
	wrapped := http_middleware.Logging(logger.NewTestLogger())(
		http_middleware.Trace()(handler),
	)
	done := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { done <- struct{}{} }()
		wrapped.ServeHTTP(w, r)
	}))
	t.Cleanup(func() { cancel(); server.Close() })
	dial := func() *websocket.Conn {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.CloseNow() })
		writeJSON(t, conn, AuthRequest{Type: authReqType, AccessToken: "token"})
		var response AuthResponse
		readJSON(t, conn, &response)
		require.Equal(t, authResType, response.Type)
		return conn
	}
	return hub, dial, done, cancel
}

func waitHandler(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not finish")
	}
}

func TestDeliveryToMultipleSocketsAndDisconnectCleanup(t *testing.T) {
	userID := uuid.New()
	hub, dial, done, _ := deliveryServer(t, userID)
	first, second := dial(), dial()
	for _, kind := range []string{"message.created", "message.edited"} {
		require.NoError(t, hub.Publish(userID, Event{Type: kind}))
		for _, conn := range []*websocket.Conn{first, second} {
			var event Event
			readJSON(t, conn, &event)
			require.Equal(t, kind, event.Type)
		}
	}
	require.NoError(t, first.CloseNow())
	waitHandler(t, done)
	require.NoError(t, hub.Publish(userID, Event{Type: "still.connected"}))
	var event Event
	readJSON(t, second, &event)
	require.Equal(t, "still.connected", event.Type)
	require.NoError(t, second.CloseNow())
	waitHandler(t, done)
	hub.mu.RLock()
	empty := len(hub.clients) == 0
	hub.mu.RUnlock()
	require.True(t, empty, "disconnected clients remain registered")
}

func TestDeliveryStopsIdleWriterOnApplicationShutdown(t *testing.T) {
	hub, dial, done, cancel := deliveryServer(t, uuid.New())
	conn := dial()
	cancel()
	requireConnectionClosed(t, conn)
	waitHandler(t, done)
	hub.mu.RLock()
	empty := len(hub.clients) == 0
	hub.mu.RUnlock()
	require.True(t, empty)
}

func TestDeliveryRejectsApplicationMessagesAfterAuthentication(t *testing.T) {
	_, dial, done, _ := deliveryServer(t, uuid.New())
	conn := dial()
	writeJSON(t, conn, map[string]string{"type": "message.send"})
	requireCloseStatus(t, conn, websocket.StatusPolicyViolation)
	// Finish the peer side before waiting for server cleanup.
	_ = conn.CloseNow()
	waitHandler(t, done)
}

func TestClientClosesSocketAfterQueueOverflow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan *Client, 1)
	start := make(chan struct{})
	var startOnce sync.Once
	release := func() { startOnce.Do(func() { close(start) }) }
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		client := NewClient(ctx, conn)
		ready <- client
		// Hold the writer so queue exhaustion is deterministic, independent of
		// OS socket buffer sizes or the speed of this machine.
		<-start
		_ = client.Run()
	}))
	t.Cleanup(func() { cancel(); release(); server.Close() })
	dialCtx, cancelDial := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelDial()
	conn, _, err := websocket.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })
	var client *Client
	select {
	case client = <-ready:
	case <-dialCtx.Done():
		t.Fatal("server did not create client")
	}
	for range clientQueueSize {
		require.True(t, client.enqueue([]byte(`{"type":"test"}`)))
	}
	require.False(t, client.enqueue([]byte(`{"type":"overflow"}`)))
	release()
	requireConnectionClosed(t, conn)
	waitHandler(t, done)
}
