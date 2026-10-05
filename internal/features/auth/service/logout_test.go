package auth_service

import (
	"errors"
	"testing"

	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLogout(t *testing.T) {
	t.Run("deletes session identified by refresh token", func(t *testing.T) {
		sessionsRepository := NewMockSessionsRepository(t)
		tokenProvider := NewMockTokenProvider(t)
		claims := auth.RefreshTokenClaims{
			SessionID: uuid.New(),
			TokenID:   uuid.New(),
		}
		tokenProvider.EXPECT().
			ParseRefreshToken("refresh-token").
			Return(claims, nil)
		sessionsRepository.EXPECT().
			DeleteSession(mock.Anything, claims.SessionID, claims.TokenID).
			Return(nil)
		service := &AuthService{
			sessionsRepository: sessionsRepository,
			tokenProvider:      tokenProvider,
		}

		err := service.Logout(t.Context(), "refresh-token")

		require.NoError(t, err)
	})

	t.Run("returns refresh token parsing error", func(t *testing.T) {
		sessionsRepository := NewMockSessionsRepository(t)
		tokenProvider := NewMockTokenProvider(t)
		parseErr := errors.New("parse token")
		tokenProvider.EXPECT().
			ParseRefreshToken("invalid-token").
			Return(auth.RefreshTokenClaims{}, parseErr)
		service := &AuthService{
			sessionsRepository: sessionsRepository,
			tokenProvider:      tokenProvider,
		}

		err := service.Logout(t.Context(), "invalid-token")

		require.ErrorIs(t, err, parseErr)
	})

	t.Run("treats missing session as successful logout", func(t *testing.T) {
		sessionsRepository := NewMockSessionsRepository(t)
		tokenProvider := NewMockTokenProvider(t)
		claims := auth.RefreshTokenClaims{
			SessionID: uuid.New(),
			TokenID:   uuid.New(),
		}
		tokenProvider.EXPECT().
			ParseRefreshToken("refresh-token").
			Return(claims, nil)
		sessionsRepository.EXPECT().
			DeleteSession(mock.Anything, claims.SessionID, claims.TokenID).
			Return(domain.ErrNotFound)
		service := &AuthService{
			sessionsRepository: sessionsRepository,
			tokenProvider:      tokenProvider,
		}

		err := service.Logout(t.Context(), "refresh-token")

		require.NoError(t, err)
	})

	t.Run("returns unexpected session deletion error", func(t *testing.T) {
		sessionsRepository := NewMockSessionsRepository(t)
		tokenProvider := NewMockTokenProvider(t)
		claims := auth.RefreshTokenClaims{
			SessionID: uuid.New(),
			TokenID:   uuid.New(),
		}
		deleteErr := errors.New("delete session")
		tokenProvider.EXPECT().
			ParseRefreshToken("refresh-token").
			Return(claims, nil)
		sessionsRepository.EXPECT().
			DeleteSession(mock.Anything, claims.SessionID, claims.TokenID).
			Return(deleteErr)
		service := &AuthService{
			sessionsRepository: sessionsRepository,
			tokenProvider:      tokenProvider,
		}

		err := service.Logout(t.Context(), "refresh-token")

		require.ErrorIs(t, err, deleteErr)
	})
}
