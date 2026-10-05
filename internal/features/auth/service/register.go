package auth_service

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/auth"
	"github.com/wrzdx/Nero/internal/core/domain"
	"time"

	"github.com/google/uuid"
)

func (s *AuthService) Register(
	ctx context.Context,
	command RegisterCommand,
) (
	domain.User,
	auth.TokenPair,
	error,
) {
	profile, err := domain.NewUserProfile(
		command.Username,
		command.FirstName,
		command.LastName,
		command.Bio,
	)
	if err != nil {
		return domain.User{}, auth.TokenPair{}, fmt.Errorf("new user profile: %w", err)
	}
	if err := auth.ValidatePassword(command.Password); err != nil {
		return domain.User{}, auth.TokenPair{}, domain.DetailedError{
			Err:     err,
			Details: map[string]string{"password": err.Error()},
		}
	}
	passwordHash, err := s.hasher.Hash(command.Password)
	if err != nil {
		return domain.User{}, auth.TokenPair{}, fmt.Errorf("hash password: %w", err)
	}
	now := time.Now()
	user, err := domain.NewUser(
		uuid.New(),
		profile,
		now,
		nil,
		passwordHash,
	)
	if err != nil {
		return domain.User{}, auth.TokenPair{}, fmt.Errorf("new user: %w", err)
	}

	session, err := domain.NewSession(
		uuid.New(),
		user.ID,
		uuid.New(),
		now,
		now.Add(s.config.SessionTTL),
	)
	if err != nil {
		return domain.User{}, auth.TokenPair{}, fmt.Errorf("new session: %w", err)
	}
	tokens, err := s.generateTokenPair(session, now)
	if err != nil {
		return domain.User{}, auth.TokenPair{}, fmt.Errorf(
			"generate token pair: %w",
			err,
		)
	}
	err = s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.usersRepository.CreateUser(ctx, user); err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		if err := s.sessionsRepository.CreateSession(ctx, session); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		return nil
	})

	if err != nil {
		return domain.User{}, auth.TokenPair{}, fmt.Errorf("transaction: %w", err)
	}

	return user, tokens, nil
}

type RegisterCommand struct {
	Username  string
	FirstName string
	LastName  *string
	Bio       *string
	Password  string
}
