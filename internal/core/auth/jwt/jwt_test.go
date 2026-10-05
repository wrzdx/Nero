package auth_jwt

import (
	"testing"
	"time"

	"github.com/wrzdx/Nero/internal/core/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndParseAccessToken(t *testing.T) {
	provider := NewTokenProvider(Config{Secret: []byte("secret")})
	now := time.Now()
	want := auth.AccessTokenClaims{UserID: uuid.New()}
	uuid.NewV7()
	token, err := provider.GenerateAccessToken(want, auth.TokenLifetime{
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
	})
	require.NoError(t, err)

	got, err := provider.ParseAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, want, got.AccessTokenClaims)
	require.WithinDuration(t, now.Add(time.Hour), got.ExpiresAt, time.Second)
}

func TestGenerateAndParseRefreshToken(t *testing.T) {
	provider := NewTokenProvider(Config{Secret: []byte("secret")})
	now := time.Now()
	want := auth.RefreshTokenClaims{
		SessionID: uuid.New(),
		TokenID:   uuid.New(),
	}

	token, err := provider.GenerateRefreshToken(want, auth.TokenLifetime{
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
	})
	require.NoError(t, err)

	got, err := provider.ParseRefreshToken(token)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestGenerateTokenValidation(t *testing.T) {
	provider := NewTokenProvider(Config{Secret: []byte("secret")})
	now := time.Now()
	validLifetime := auth.TokenLifetime{IssuedAt: now, ExpiresAt: now.Add(time.Hour)}

	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{
			name: "access claims",
			run: func() error {
				_, err := provider.GenerateAccessToken(auth.AccessTokenClaims{}, validLifetime)
				return err
			},
			want: auth.ErrInvalidClaims,
		},
		{
			name: "refresh session id",
			run: func() error {
				_, err := provider.GenerateRefreshToken(auth.RefreshTokenClaims{TokenID: uuid.New()}, validLifetime)
				return err
			},
			want: auth.ErrInvalidClaims,
		},
		{
			name: "refresh token id",
			run: func() error {
				_, err := provider.GenerateRefreshToken(auth.RefreshTokenClaims{SessionID: uuid.New()}, validLifetime)
				return err
			},
			want: auth.ErrInvalidClaims,
		},
		{
			name: "zero issued at",
			run: func() error {
				_, err := provider.GenerateAccessToken(
					auth.AccessTokenClaims{UserID: uuid.New()},
					auth.TokenLifetime{ExpiresAt: now.Add(time.Hour)},
				)
				return err
			},
			want: auth.ErrInvalidTokenLifetime,
		},
		{
			name: "zero expires at",
			run: func() error {
				_, err := provider.GenerateAccessToken(
					auth.AccessTokenClaims{UserID: uuid.New()},
					auth.TokenLifetime{IssuedAt: now},
				)
				return err
			},
			want: auth.ErrInvalidTokenLifetime,
		},
		{
			name: "expires at equals issued at",
			run: func() error {
				_, err := provider.GenerateAccessToken(
					auth.AccessTokenClaims{UserID: uuid.New()},
					auth.TokenLifetime{IssuedAt: now, ExpiresAt: now},
				)
				return err
			},
			want: auth.ErrInvalidTokenLifetime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, tt.run(), tt.want)
		})
	}
}

func TestParseAccessTokenRejectsInvalidTokens(t *testing.T) {
	secret := []byte("secret")
	provider := NewTokenProvider(Config{Secret: secret})
	now := time.Now()

	tests := []struct {
		name   string
		claims accessClaims
		method jwt.SigningMethod
		secret []byte
	}{
		{
			name: "wrong token type",
			claims: validAccessJWTClaims(now, func(c *accessClaims) {
				c.Type = tokenTypeRefresh
			}),
			method: jwt.SigningMethodHS256,
			secret: secret,
		},
		{
			name: "nil user id",
			claims: validAccessJWTClaims(now, func(c *accessClaims) {
				c.UserID = uuid.Nil
			}),
			method: jwt.SigningMethodHS256,
			secret: secret,
		},
		{
			name: "expired",
			claims: validAccessJWTClaims(now, func(c *accessClaims) {
				c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Hour))
			}),
			method: jwt.SigningMethodHS256,
			secret: secret,
		},
		{
			name: "missing expiration",
			claims: validAccessJWTClaims(now, func(c *accessClaims) {
				c.ExpiresAt = nil
			}),
			method: jwt.SigningMethodHS256,
			secret: secret,
		},
		{
			name:   "wrong algorithm",
			claims: validAccessJWTClaims(now),
			method: jwt.SigningMethodHS512,
			secret: secret,
		},
		{
			name:   "wrong signature",
			claims: validAccessJWTClaims(now),
			method: jwt.SigningMethodHS256,
			secret: []byte("other-secret"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := signedToken(t, tt.method, tt.claims, tt.secret)
			_, err := provider.ParseAccessToken(token)
			require.ErrorIs(t, err, auth.ErrInvalidToken)
		})
	}
}

func TestParseRefreshTokenRejectsInvalidTokens(t *testing.T) {
	secret := []byte("secret")
	provider := NewTokenProvider(Config{Secret: secret})
	now := time.Now()

	tests := []struct {
		name   string
		claims refreshClaims
	}{
		{
			name: "wrong token type",
			claims: validRefreshJWTClaims(now, func(c *refreshClaims) {
				c.Type = tokenTypeAccess
			}),
		},
		{
			name: "nil session id",
			claims: validRefreshJWTClaims(now, func(c *refreshClaims) {
				c.SessionID = uuid.Nil
			}),
		},
		{
			name: "nil token id",
			claims: validRefreshJWTClaims(now, func(c *refreshClaims) {
				c.TokenID = uuid.Nil
			}),
		},
		{
			name: "expired",
			claims: validRefreshJWTClaims(now, func(c *refreshClaims) {
				c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Hour))
			}),
		},
		{
			name: "missing expiration",
			claims: validRefreshJWTClaims(now, func(c *refreshClaims) {
				c.ExpiresAt = nil
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := signedToken(t, jwt.SigningMethodHS256, tt.claims, secret)
			_, err := provider.ParseRefreshToken(token)
			require.ErrorIs(t, err, auth.ErrInvalidToken)
		})
	}
}

func TestTokenTypesCannotBeInterchanged(t *testing.T) {
	provider := NewTokenProvider(Config{Secret: []byte("secret")})
	now := time.Now()
	lifetime := auth.TokenLifetime{IssuedAt: now, ExpiresAt: now.Add(time.Hour)}

	access, err := provider.GenerateAccessToken(auth.AccessTokenClaims{UserID: uuid.New()}, lifetime)
	require.NoError(t, err)

	refresh, err := provider.GenerateRefreshToken(auth.RefreshTokenClaims{
		SessionID: uuid.New(),
		TokenID:   uuid.New(),
	}, lifetime)
	require.NoError(t, err)

	_, err = provider.ParseRefreshToken(access)
	require.ErrorIs(t, err, auth.ErrInvalidToken)

	_, err = provider.ParseAccessToken(refresh)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func validAccessJWTClaims(now time.Time, mutate ...func(*accessClaims)) accessClaims {
	claims := accessClaims{
		Type: tokenTypeAccess,
		AccessTokenClaims: auth.AccessTokenClaims{
			UserID: uuid.New(),
		},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	for _, fn := range mutate {
		fn(&claims)
	}
	return claims
}

func validRefreshJWTClaims(now time.Time, mutate ...func(*refreshClaims)) refreshClaims {
	claims := refreshClaims{
		Type: tokenTypeRefresh,
		RefreshTokenClaims: auth.RefreshTokenClaims{
			SessionID: uuid.New(),
			TokenID:   uuid.New(),
		},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	for _, fn := range mutate {
		fn(&claims)
	}
	return claims
}

func signedToken(t *testing.T, method jwt.SigningMethod, claims jwt.Claims, secret []byte) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(secret)
	require.NoError(t, err)
	return token
}
