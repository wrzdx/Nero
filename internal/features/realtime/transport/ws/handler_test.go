package realtime_transport_ws

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/auth"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tokenProviderStub struct {
	parse func(string) (auth.ParsedAccessToken, error)
}

func (s tokenProviderStub) ParseAccessToken(token string) (auth.ParsedAccessToken, error) {
	return s.parse(token)
}

func TestHandlerAuthenticatesConnection(t *testing.T) {
	userID := uuid.New()
	provider := tokenProviderStub{parse: func(token string) (auth.ParsedAccessToken, error) {
		assert.Equal(t, "access-token", token)
		return auth.ParsedAccessToken{
			AccessTokenClaims: auth.AccessTokenClaims{UserID: userID},
			ExpiresAt:         time.Now().Add(time.Minute),
		}, nil
	}}

	conn := dialHandler(t, context.Background(), provider)
	writeJSON(t, conn, AuthRequest{Type: authReqType, AccessToken: "access-token"})

	var response AuthResponse
	readJSON(t, conn, &response)
	require.Equal(t, AuthResponse{Type: authResType}, response)
}

func TestHandlerRejectsUnexpectedFirstMessage(t *testing.T) {
	provider := tokenProviderStub{parse: func(string) (auth.ParsedAccessToken, error) {
		t.Error("token provider must not be called")
		return auth.ParsedAccessToken{}, auth.ErrInvalidToken
	}}

	conn := dialHandler(t, context.Background(), provider)
	writeJSON(t, conn, AuthRequest{Type: "message.send", AccessToken: "access-token"})

	requireCloseStatus(t, conn, websocket.StatusPolicyViolation)
}

func TestHandlerRejectsInvalidToken(t *testing.T) {
	provider := tokenProviderStub{parse: func(string) (auth.ParsedAccessToken, error) {
		return auth.ParsedAccessToken{}, auth.ErrInvalidToken
	}}

	conn := dialHandler(t, context.Background(), provider)
	writeJSON(t, conn, AuthRequest{Type: authReqType, AccessToken: "invalid-token"})

	requireCloseStatus(t, conn, websocket.StatusPolicyViolation)
}

func TestHandlerClosesConnectionOnMalformedJSON(t *testing.T) {
	provider := tokenProviderStub{parse: func(string) (auth.ParsedAccessToken, error) {
		t.Error("token provider must not be called")
		return auth.ParsedAccessToken{}, auth.ErrInvalidToken
	}}

	conn := dialHandler(t, context.Background(), provider)
	writeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, conn.Write(writeCtx, websocket.MessageText, []byte("{")))

	requireConnectionClosed(t, conn)
}

func TestHandlerClosesConnectionWhenTokenExpires(t *testing.T) {
	provider := tokenProviderStub{parse: func(string) (auth.ParsedAccessToken, error) {
		return auth.ParsedAccessToken{
			AccessTokenClaims: auth.AccessTokenClaims{UserID: uuid.New()},
			ExpiresAt:         time.Now().Add(200 * time.Millisecond),
		}, nil
	}}

	conn := dialHandler(t, context.Background(), provider)
	writeJSON(t, conn, AuthRequest{Type: authReqType, AccessToken: "access-token"})
	var response AuthResponse
	readJSON(t, conn, &response)

	requireConnectionClosed(t, conn)
}

func TestHandlerClosesConnectionWhenApplicationStops(t *testing.T) {
	appCtx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	provider := tokenProviderStub{parse: func(string) (auth.ParsedAccessToken, error) {
		return auth.ParsedAccessToken{
			AccessTokenClaims: auth.AccessTokenClaims{UserID: uuid.New()},
			ExpiresAt:         time.Now().Add(time.Minute),
		}, nil
	}}

	conn := dialHandler(t, appCtx, provider)
	writeJSON(t, conn, AuthRequest{Type: authReqType, AccessToken: "access-token"})
	var response AuthResponse
	readJSON(t, conn, &response)
	stop()

	requireConnectionClosed(t, conn)
}

func dialHandler(t *testing.T, appCtx context.Context, provider TokenProvider) *websocket.Conn {
	t.Helper()

	server := httptest.NewServer(NewWSHandler(appCtx, provider, NewHub()))
	t.Cleanup(server.Close)

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	dialCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })

	return conn
}

func writeJSON(t *testing.T, conn *websocket.Conn, value any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, wsjson.Write(ctx, conn, value))
}

func readJSON(t *testing.T, conn *websocket.Conn, value any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, wsjson.Read(ctx, conn, value))
}

func requireCloseStatus(t *testing.T, conn *websocket.Conn, status websocket.StatusCode) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := conn.Read(ctx)
	require.Error(t, err)
	require.Equal(t, status, websocket.CloseStatus(err))
}

func requireConnectionClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := conn.Read(ctx)
	require.Error(t, err)
	require.False(t, errors.Is(err, context.DeadlineExceeded), "connection remained open")
}
