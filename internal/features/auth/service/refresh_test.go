package auth_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRefresh(t *testing.T) {
	t.Run("rotates session and returns new token pair", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, false)
		fixture.expectParse()
		fixture.expectSuccessfulRotation()
		fixture.usersRepository.EXPECT().
			GetUser(fixture.txContextMatcher(), fixture.user.ID).
			Return(fixture.user, nil)
		var accessClaims auth.AccessTokenClaims
		var accessLifetime auth.TokenLifetime
		var refreshClaims auth.RefreshTokenClaims
		var refreshLifetime auth.TokenLifetime
		fixture.tokenProvider.EXPECT().
			GenerateAccessToken(mock.Anything, mock.Anything).
			Run(func(claims auth.AccessTokenClaims, lifetime auth.TokenLifetime) {
				accessClaims = claims
				accessLifetime = lifetime
			}).
			Return("new-access-token", nil)
		fixture.tokenProvider.EXPECT().
			GenerateRefreshToken(mock.Anything, mock.Anything).
			Run(func(claims auth.RefreshTokenClaims, lifetime auth.TokenLifetime) {
				refreshClaims = claims
				refreshLifetime = lifetime
			}).
			Return("new-refresh-token", nil)
		fixture.expectTransaction(t, nil)

		tokens, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.NoError(t, err)
		require.Equal(t, auth.TokenPair{
			Access:  "new-access-token",
			Refresh: "new-refresh-token",
		}, tokens)
		require.NotEqual(t, fixture.claims.TokenID, fixture.rotatedSession.CurrentTokenID)
		require.NotEqual(t, uuid.Nil, fixture.rotatedSession.CurrentTokenID)
		require.Equal(t, fixture.user.ID, accessClaims.UserID)
		require.True(t, fixture.rotatedSession.LastUsedAt.Equal(accessLifetime.IssuedAt))
		require.Equal(
			t,
			fixture.config.AccessTokenTTL,
			accessLifetime.ExpiresAt.Sub(accessLifetime.IssuedAt),
		)
		require.Equal(t, fixture.rotatedSession.ID, refreshClaims.SessionID)
		require.Equal(t, fixture.rotatedSession.CurrentTokenID, refreshClaims.TokenID)
		require.True(t, fixture.rotatedSession.LastUsedAt.Equal(refreshLifetime.IssuedAt))
		require.True(t, fixture.rotatedSession.ExpiresAt.Equal(refreshLifetime.ExpiresAt))
	})

	t.Run("returns parse error without opening transaction", func(t *testing.T) {
		tokenProvider := NewMockTokenProvider(t)
		parseErr := errors.New("malformed refresh token")
		tokenProvider.EXPECT().
			ParseRefreshToken("bad-token").
			Return(auth.RefreshTokenClaims{}, parseErr)
		service := &AuthService{tokenProvider: tokenProvider}

		tokens, err := service.Refresh(t.Context(), "bad-token")

		require.ErrorIs(t, err, parseErr)
		require.Equal(t, auth.TokenPair{}, tokens)
	})

	t.Run("maps missing session to invalid token", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, false)
		fixture.expectParse()
		fixture.sessionsRepository.EXPECT().
			RotateSession(
				fixture.txContextMatcher(),
				fixture.claims.SessionID,
				fixture.claims.TokenID,
				mock.Anything,
				mock.Anything,
			).
			Return(domain.Session{}, domain.ErrNotFound)
		fixture.expectTransaction(t, auth.ErrInvalidToken)

		tokens, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.ErrorIs(t, err, auth.ErrInvalidToken)
		require.Equal(t, auth.TokenPair{}, tokens)
	})

	t.Run("returns unexpected session rotation error", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, false)
		rotateErr := errors.New("database unavailable")
		fixture.expectParse()
		fixture.sessionsRepository.EXPECT().
			RotateSession(
				fixture.txContextMatcher(),
				fixture.claims.SessionID,
				fixture.claims.TokenID,
				mock.Anything,
				mock.Anything,
			).
			Return(domain.Session{}, rotateErr)
		fixture.expectTransaction(t, rotateErr)

		_, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.ErrorIs(t, err, rotateErr)
	})

	t.Run("maps missing session user to invalid token", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, false)
		fixture.expectParse()
		fixture.expectSuccessfulRotation()
		fixture.usersRepository.EXPECT().
			GetUser(fixture.txContextMatcher(), fixture.user.ID).
			Return(domain.User{}, domain.ErrNotFound)
		fixture.expectTransaction(t, auth.ErrInvalidToken)

		_, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("returns unexpected user repository error", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, false)
		getUserErr := errors.New("read user failure")
		fixture.expectParse()
		fixture.expectSuccessfulRotation()
		fixture.usersRepository.EXPECT().
			GetUser(fixture.txContextMatcher(), fixture.user.ID).
			Return(domain.User{}, getUserErr)
		fixture.expectTransaction(t, getUserErr)

		_, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.ErrorIs(t, err, getUserErr)
	})

	t.Run("rejects session belonging to deleted user", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, true)
		fixture.expectParse()
		fixture.expectSuccessfulRotation()
		fixture.usersRepository.EXPECT().
			GetUser(fixture.txContextMatcher(), fixture.user.ID).
			Return(fixture.user, nil)
		fixture.expectTransaction(t, auth.ErrInvalidToken)

		_, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("returns access token generation error to transaction", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, false)
		generateErr := errors.New("access signer failure")
		fixture.expectParse()
		fixture.expectSuccessfulRotation()
		fixture.usersRepository.EXPECT().
			GetUser(fixture.txContextMatcher(), fixture.user.ID).
			Return(fixture.user, nil)
		fixture.tokenProvider.EXPECT().
			GenerateAccessToken(mock.Anything, mock.Anything).
			Return("", generateErr)
		fixture.expectTransaction(t, generateErr)

		tokens, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.ErrorIs(t, err, generateErr)
		require.Equal(t, auth.TokenPair{}, tokens)
	})

	t.Run("returns refresh token generation error to transaction", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, false)
		generateErr := errors.New("refresh signer failure")
		fixture.expectParse()
		fixture.expectSuccessfulRotation()
		fixture.usersRepository.EXPECT().
			GetUser(fixture.txContextMatcher(), fixture.user.ID).
			Return(fixture.user, nil)
		fixture.tokenProvider.EXPECT().
			GenerateAccessToken(mock.Anything, mock.Anything).
			Return("new-access-token", nil)
		fixture.tokenProvider.EXPECT().
			GenerateRefreshToken(mock.Anything, mock.Anything).
			Return("", generateErr)
		fixture.expectTransaction(t, generateErr)

		tokens, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.ErrorIs(t, err, generateErr)
		require.Equal(t, auth.TokenPair{}, tokens)
	})

	t.Run("returns transaction begin error without rotating session", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, false)
		txErr := errors.New("begin transaction failure")
		fixture.expectParse()
		fixture.txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			Return(txErr)

		tokens, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.ErrorIs(t, err, txErr)
		require.Equal(t, auth.TokenPair{}, tokens)
	})

	t.Run("does not return generated tokens when transaction commit fails", func(t *testing.T) {
		fixture := newRefreshTestFixture(t, false)
		commitErr := errors.New("commit transaction failure")
		fixture.expectParse()
		fixture.expectSuccessfulRotation()
		fixture.usersRepository.EXPECT().
			GetUser(fixture.txContextMatcher(), fixture.user.ID).
			Return(fixture.user, nil)
		fixture.tokenProvider.EXPECT().
			GenerateAccessToken(mock.Anything, mock.Anything).
			Return("new-access-token", nil)
		fixture.tokenProvider.EXPECT().
			GenerateRefreshToken(mock.Anything, mock.Anything).
			Return("new-refresh-token", nil)
		fixture.txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				require.NoError(t, fn(fixture.txContext))
				return commitErr
			})

		tokens, err := fixture.service.Refresh(t.Context(), "refresh-token")

		require.ErrorIs(t, err, commitErr)
		require.Equal(t, auth.TokenPair{}, tokens)
	})
}

type refreshTxContextKey struct{}

type refreshTestFixture struct {
	usersRepository    *MockUsersRepository
	sessionsRepository *MockSessionsRepository
	tokenProvider      *MockTokenProvider
	txManager          *MockTXManager
	service            *AuthService
	config             AuthConfig
	claims             auth.RefreshTokenClaims
	user               domain.User
	session            domain.Session
	rotatedSession     domain.Session
	txContext          context.Context
}

func newRefreshTestFixture(t *testing.T, deletedUser bool) *refreshTestFixture {
	t.Helper()

	usersRepository := NewMockUsersRepository(t)
	sessionsRepository := NewMockSessionsRepository(t)
	tokenProvider := NewMockTokenProvider(t)
	txManager := NewMockTXManager(t)
	user := newLoginTestUser(t, deletedUser)
	createdAt := time.Now().Add(-time.Hour)
	session, err := domain.NewSession(
		uuid.New(),
		user.ID,
		uuid.New(),
		createdAt,
		createdAt.Add(24*time.Hour),
	)
	require.NoError(t, err)
	config := registerTestConfig()
	txContext := context.WithValue(t.Context(), refreshTxContextKey{}, "transaction")
	fixture := &refreshTestFixture{
		usersRepository:    usersRepository,
		sessionsRepository: sessionsRepository,
		tokenProvider:      tokenProvider,
		txManager:          txManager,
		config:             config,
		claims: auth.RefreshTokenClaims{
			SessionID: session.ID,
			TokenID:   session.CurrentTokenID,
		},
		user:      user,
		session:   session,
		txContext: txContext,
	}
	fixture.service = &AuthService{
		usersRepository:    usersRepository,
		sessionsRepository: sessionsRepository,
		tokenProvider:      tokenProvider,
		txManager:          txManager,
		config:             config,
	}
	return fixture
}

func (f *refreshTestFixture) txContextMatcher() any {
	return mock.MatchedBy(func(ctx context.Context) bool {
		return ctx.Value(refreshTxContextKey{}) == "transaction"
	})
}

func (f *refreshTestFixture) expectParse() {
	f.tokenProvider.EXPECT().
		ParseRefreshToken("refresh-token").
		Return(f.claims, nil)
}

func (f *refreshTestFixture) expectSuccessfulRotation() {
	f.sessionsRepository.EXPECT().
		RotateSession(
			f.txContextMatcher(),
			f.claims.SessionID,
			f.claims.TokenID,
			mock.Anything,
			mock.Anything,
		).
		RunAndReturn(func(
			_ context.Context,
			_ uuid.UUID,
			_ uuid.UUID,
			newTokenID uuid.UUID,
			usedAt time.Time,
		) (domain.Session, error) {
			f.rotatedSession = f.session
			f.rotatedSession.CurrentTokenID = newTokenID
			f.rotatedSession.LastUsedAt = usedAt
			return f.rotatedSession, nil
		})
}

func (f *refreshTestFixture) expectTransaction(t *testing.T, callbackErr error) {
	t.Helper()
	f.txManager.EXPECT().
		WithinTransaction(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			err := fn(f.txContext)
			if callbackErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, callbackErr)
			}
			return err
		})
}
