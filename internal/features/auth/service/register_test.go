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

func TestRegister(t *testing.T) {
	t.Run("creates user and session in one transaction", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		hasher := NewMockHasher(t)
		tokenProvider := NewMockTokenProvider(t)
		txManager := NewMockTXManager(t)
		config := registerTestConfig()
		payload := validRegisterTestPayload()
		txContext := context.WithValue(t.Context(), registerTxContextKey{}, "transaction")
		isTxContext := mock.MatchedBy(func(ctx context.Context) bool {
			return ctx.Value(registerTxContextKey{}) == "transaction"
		})
		var createdUser domain.User
		var createdSession domain.Session
		var accessClaims auth.AccessTokenClaims
		var accessLifetime auth.TokenLifetime
		var refreshClaims auth.RefreshTokenClaims
		var refreshLifetime auth.TokenLifetime
		userCreated := false
		tokensGenerated := false

		hasher.EXPECT().
			Hash(payload.Password).
			Return("password-hash", nil)
		tokenProvider.EXPECT().
			GenerateAccessToken(mock.Anything, mock.Anything).
			Run(func(claims auth.AccessTokenClaims, lifetime auth.TokenLifetime) {
				accessClaims = claims
				accessLifetime = lifetime
			}).
			Return("access-token", nil)
		tokenProvider.EXPECT().
			GenerateRefreshToken(mock.Anything, mock.Anything).
			Run(func(claims auth.RefreshTokenClaims, lifetime auth.TokenLifetime) {
				refreshClaims = claims
				refreshLifetime = lifetime
				tokensGenerated = true
			}).
			Return("refresh-token", nil)
		usersRepository.EXPECT().
			CreateUser(isTxContext, mock.Anything).
			Run(func(_ context.Context, user domain.User) {
				createdUser = user
				userCreated = true
			}).
			Return(nil)
		sessionsRepository.EXPECT().
			CreateSession(isTxContext, mock.Anything).
			Run(func(_ context.Context, session domain.Session) {
				require.True(t, userCreated)
				createdSession = session
			}).
			Return(nil)
		txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				require.True(t, tokensGenerated)
				return fn(txContext)
			})
		service := &AuthService{
			usersRepository:    usersRepository,
			sessionsRepository: sessionsRepository,
			hasher:             hasher,
			tokenProvider:      tokenProvider,
			txManager:          txManager,
			config:             config,
		}

		user, tokens, err := service.Register(t.Context(), payload)

		require.NoError(t, err)
		require.Equal(t, createdUser, user)
		require.Equal(t, auth.TokenPair{
			Access:  "access-token",
			Refresh: "refresh-token",
		}, tokens)
		require.NotEqual(t, uuid.Nil, user.ID)
		require.Equal(t, "Username_1", user.Profile.Username)
		require.Equal(t, "First Name", user.Profile.FirstName)
		require.Equal(t, "Last Name", *user.Profile.LastName)
		require.Equal(t, "Bio", *user.Profile.Bio)
		require.Equal(t, "password-hash", user.PasswordHash)
		require.NotEqual(t, uuid.Nil, createdSession.ID)
		require.Equal(t, user.ID, createdSession.UserID)
		require.NotEqual(t, uuid.Nil, createdSession.CurrentTokenID)
		require.True(t, user.CreatedAt.Equal(createdSession.CreatedAt))
		require.True(t, createdSession.CreatedAt.Equal(createdSession.LastUsedAt))
		require.Equal(t, config.SessionTTL, createdSession.ExpiresAt.Sub(createdSession.CreatedAt))
		require.Equal(t, user.ID, accessClaims.UserID)
		require.True(t, createdSession.CreatedAt.Equal(accessLifetime.IssuedAt))
		require.Equal(t, config.AccessTokenTTL, accessLifetime.ExpiresAt.Sub(accessLifetime.IssuedAt))
		require.Equal(t, createdSession.ID, refreshClaims.SessionID)
		require.Equal(t, createdSession.CurrentTokenID, refreshClaims.TokenID)
		require.True(t, createdSession.CreatedAt.Equal(refreshLifetime.IssuedAt))
		require.True(t, createdSession.ExpiresAt.Equal(refreshLifetime.ExpiresAt))
	})

	t.Run("rejects invalid profile before hashing password", func(t *testing.T) {
		service := &AuthService{}
		payload := validRegisterTestPayload()
		payload.Username = "bad username"

		_, _, err := service.Register(t.Context(), payload)

		require.ErrorIs(t, err, domain.ErrInvalidUserProfile)
	})

	t.Run("rejects invalid password before hashing", func(t *testing.T) {
		hasher := NewMockHasher(t)
		service := &AuthService{hasher: hasher}
		payload := validRegisterTestPayload()
		payload.Password = "too short"

		_, _, err := service.Register(t.Context(), payload)

		require.ErrorIs(t, err, auth.ErrInvalidPassword)
	})

	t.Run("returns password hashing error before opening transaction", func(t *testing.T) {
		hasher := NewMockHasher(t)
		hashErr := errors.New("password hasher failure")
		payload := validRegisterTestPayload()
		hasher.EXPECT().Hash(payload.Password).Return("", hashErr)
		service := &AuthService{hasher: hasher}

		_, _, err := service.Register(t.Context(), payload)

		require.ErrorIs(t, err, hashErr)
	})

	t.Run("does not open transaction when access token generation fails", func(t *testing.T) {
		hasher := NewMockHasher(t)
		tokenProvider := NewMockTokenProvider(t)
		generateErr := errors.New("access signer failure")
		payload := validRegisterTestPayload()
		hasher.EXPECT().Hash(payload.Password).Return("password-hash", nil)
		tokenProvider.EXPECT().
			GenerateAccessToken(mock.Anything, mock.Anything).
			Return("", generateErr)
		service := &AuthService{
			hasher:        hasher,
			tokenProvider: tokenProvider,
			config:        registerTestConfig(),
		}

		_, _, err := service.Register(t.Context(), payload)

		require.ErrorIs(t, err, generateErr)
	})

	t.Run("does not open transaction when refresh token generation fails", func(t *testing.T) {
		hasher := NewMockHasher(t)
		tokenProvider := NewMockTokenProvider(t)
		generateErr := errors.New("refresh signer failure")
		payload := validRegisterTestPayload()
		hasher.EXPECT().Hash(payload.Password).Return("password-hash", nil)
		tokenProvider.EXPECT().
			GenerateAccessToken(mock.Anything, mock.Anything).
			Return("access-token", nil)
		tokenProvider.EXPECT().
			GenerateRefreshToken(mock.Anything, mock.Anything).
			Return("", generateErr)
		service := &AuthService{
			hasher:        hasher,
			tokenProvider: tokenProvider,
			config:        registerTestConfig(),
		}

		_, _, err := service.Register(t.Context(), payload)

		require.ErrorIs(t, err, generateErr)
	})

	t.Run("does not create session when creating user fails", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		hasher := NewMockHasher(t)
		tokenProvider := NewMockTokenProvider(t)
		txManager := NewMockTXManager(t)
		createErr := domain.ErrAlreadyExists
		payload := validRegisterTestPayload()
		expectRegisterPrerequisites(hasher, tokenProvider, payload.Password)
		usersRepository.EXPECT().
			CreateUser(mock.Anything, mock.Anything).
			Return(createErr)
		txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			RunAndReturn(runRegisterTransaction)
		service := &AuthService{
			usersRepository:    usersRepository,
			sessionsRepository: sessionsRepository,
			hasher:             hasher,
			tokenProvider:      tokenProvider,
			txManager:          txManager,
			config:             registerTestConfig(),
		}

		_, _, err := service.Register(t.Context(), payload)

		require.ErrorIs(t, err, createErr)
	})

	t.Run("returns session creation error from transaction", func(t *testing.T) {
		usersRepository := NewMockUsersRepository(t)
		sessionsRepository := NewMockSessionsRepository(t)
		hasher := NewMockHasher(t)
		tokenProvider := NewMockTokenProvider(t)
		txManager := NewMockTXManager(t)
		createErr := errors.New("create session failure")
		payload := validRegisterTestPayload()
		expectRegisterPrerequisites(hasher, tokenProvider, payload.Password)
		usersRepository.EXPECT().
			CreateUser(mock.Anything, mock.Anything).
			Return(nil)
		sessionsRepository.EXPECT().
			CreateSession(mock.Anything, mock.Anything).
			Return(createErr)
		txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			RunAndReturn(runRegisterTransaction)
		service := &AuthService{
			usersRepository:    usersRepository,
			sessionsRepository: sessionsRepository,
			hasher:             hasher,
			tokenProvider:      tokenProvider,
			txManager:          txManager,
			config:             registerTestConfig(),
		}

		_, _, err := service.Register(t.Context(), payload)

		require.ErrorIs(t, err, createErr)
	})

	t.Run("returns transaction manager error", func(t *testing.T) {
		hasher := NewMockHasher(t)
		tokenProvider := NewMockTokenProvider(t)
		txManager := NewMockTXManager(t)
		txErr := errors.New("begin transaction failure")
		payload := validRegisterTestPayload()
		expectRegisterPrerequisites(hasher, tokenProvider, payload.Password)
		txManager.EXPECT().
			WithinTransaction(mock.Anything, mock.Anything).
			Return(txErr)
		service := &AuthService{
			hasher:        hasher,
			tokenProvider: tokenProvider,
			txManager:     txManager,
			config:        registerTestConfig(),
		}

		_, _, err := service.Register(t.Context(), payload)

		require.ErrorIs(t, err, txErr)
	})
}

type registerTxContextKey struct{}

func validRegisterTestPayload() RegisterCommand {
	return RegisterCommand{
		Username:  "  Username_1  ",
		FirstName: "  First Name  ",
		LastName:  new("  Last Name  "),
		Bio:       new("  Bio  "),
		Password:  "valid password value",
	}
}

func registerTestConfig() AuthConfig {
	return AuthConfig{
		AccessTokenTTL: 15 * time.Minute,
		SessionTTL:     24 * time.Hour,
	}
}

func expectRegisterPrerequisites(
	hasher *MockHasher,
	tokenProvider *MockTokenProvider,
	password string,
) {
	hasher.EXPECT().Hash(password).Return("password-hash", nil)
	tokenProvider.EXPECT().
		GenerateAccessToken(mock.Anything, mock.Anything).
		Return("access-token", nil)
	tokenProvider.EXPECT().
		GenerateRefreshToken(mock.Anything, mock.Anything).
		Return("refresh-token", nil)
}

func runRegisterTransaction(
	ctx context.Context,
	fn func(context.Context) error,
) error {
	return fn(ctx)
}
