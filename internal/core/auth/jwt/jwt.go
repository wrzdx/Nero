package auth_jwt

import (
	"fmt"
	"github.com/wrzdx/Nero/internal/core/auth"

	"github.com/golang-jwt/jwt/v5"
)

type tokenType string

const (
	tokenTypeAccess  tokenType = "access"
	tokenTypeRefresh tokenType = "refresh"
)

type accessClaims struct {
	Type tokenType `json:"type"`
	auth.AccessTokenClaims
	jwt.RegisteredClaims
}

type refreshClaims struct {
	Type tokenType `json:"type"`
	auth.RefreshTokenClaims
	jwt.RegisteredClaims
}
type TokenProvider struct {
	config Config
}

func NewTokenProvider(config Config) *TokenProvider {
	return &TokenProvider{
		config: config,
	}
}

func (s *TokenProvider) GenerateAccessToken(
	accessTokenClaims auth.AccessTokenClaims,
	tokenLifetime auth.TokenLifetime,
) (string, error) {
	if err := accessTokenClaims.Validate(); err != nil {
		return "", fmt.Errorf("validate access claims: %w", err)
	}
	if err := tokenLifetime.Validate(); err != nil {
		return "", fmt.Errorf("validate token lifetime: %w", err)
	}
	aClaims := accessClaims{
		Type:              tokenTypeAccess,
		AccessTokenClaims: accessTokenClaims,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(tokenLifetime.ExpiresAt),
			IssuedAt:  jwt.NewNumericDate(tokenLifetime.IssuedAt),
		},
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, aClaims).SignedString(s.config.Secret)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return accessToken, nil
}

func (s *TokenProvider) GenerateRefreshToken(
	refreshTokenClaims auth.RefreshTokenClaims,
	tokenLifetime auth.TokenLifetime,
) (string, error) {
	if err := refreshTokenClaims.Validate(); err != nil {
		return "", fmt.Errorf("validate refresh claims: %w", err)
	}
	if err := tokenLifetime.Validate(); err != nil {
		return "", fmt.Errorf("validate token lifetime: %w", err)
	}

	rClaims := refreshClaims{
		Type:               tokenTypeRefresh,
		RefreshTokenClaims: refreshTokenClaims,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(tokenLifetime.ExpiresAt),
			IssuedAt:  jwt.NewNumericDate(tokenLifetime.IssuedAt),
		},
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, rClaims).SignedString(s.config.Secret)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}

	return refreshToken, nil
}

func (s *TokenProvider) ParseAccessToken(tokenStr string) (auth.ParsedAccessToken, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &accessClaims{}, func(token *jwt.Token) (any, error) {
		return s.config.Secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return auth.ParsedAccessToken{}, auth.ErrInvalidToken
	}

	claims, ok := token.Claims.(*accessClaims)
	if !ok || claims.Type != tokenTypeAccess || claims.Validate() != nil {
		return auth.ParsedAccessToken{}, auth.ErrInvalidToken
	}

	return auth.ParsedAccessToken{
		AccessTokenClaims: claims.AccessTokenClaims,
		ExpiresAt:         claims.ExpiresAt.Time,
	}, nil
}

func (s *TokenProvider) ParseRefreshToken(tokenStr string) (auth.RefreshTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &refreshClaims{}, func(token *jwt.Token) (any, error) {
		return s.config.Secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return auth.RefreshTokenClaims{}, auth.ErrInvalidToken
	}

	claims, ok := token.Claims.(*refreshClaims)
	if !ok || claims.Type != tokenTypeRefresh || claims.Validate() != nil {
		return auth.RefreshTokenClaims{}, auth.ErrInvalidToken
	}

	return claims.RefreshTokenClaims, nil
}
