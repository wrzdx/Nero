package auth_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s *AuthService) Login(
	ctx context.Context,
	username string,
	password string,
) (auth.TokenPair, error) {
	user, err := s.usersRepository.GetUserByUsername(
		ctx,
		username,
	)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.hasher.DummyCompare()
			return auth.TokenPair{}, auth.ErrInvalidCredentials
		}
		return auth.TokenPair{}, fmt.Errorf("get user by username from repo: %w", err)
	}
	if user.DeletedAt != nil {
		s.hasher.DummyCompare()
		return auth.TokenPair{}, auth.ErrInvalidCredentials
	}

	if err := s.hasher.Compare(user.PasswordHash, password); err != nil {
		if errors.Is(err, auth.ErrPasswordMismatch) {
			return auth.TokenPair{}, auth.ErrInvalidCredentials
		}
		return auth.TokenPair{}, fmt.Errorf(
			"compare passwords: %w",
			err,
		)
	}
	now := time.Now()
	session, err := domain.NewSession(uuid.New(), user.ID, uuid.New(), now, now.Add(s.config.SessionTTL))
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("new session: %w", err)
	}

	tokens, err := s.generateTokenPair(session, now)
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("generate token pair: %w", err)
	}
	err = s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		lockedUser, err := s.usersRepository.GetUserForUpdate(ctx, user.ID)
		if err != nil {
			return fmt.Errorf("get user by id from repo: %w", err)
		}
		if lockedUser.DeletedAt != nil || lockedUser.PasswordHash != user.PasswordHash {
			return auth.ErrInvalidCredentials
		}

		if err := s.sessionsRepository.CreateSession(ctx, session); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		return nil
	})
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("transaction: %w", err)
	}
	return tokens, nil
}
