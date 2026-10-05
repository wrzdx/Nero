package auth_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *SessionsRepository) RotateSession(
	ctx context.Context,
	sessionID, currentTokenID, newTokenID uuid.UUID,
	usedAt time.Time,
) (domain.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	db := postgres.GetExecutor(ctx, r.db)

	var session domain.Session
	err := db.QueryRow(ctx, `
		UPDATE sessions
		SET current_token_id=$1, last_used_at = GREATEST(last_used_at, $2)
		WHERE id=$3
			AND current_token_id=$4
			AND expires_at > $2
		RETURNING id, user_id, current_token_id, last_used_at, created_at, expires_at;
	`, newTokenID, usedAt, sessionID, currentTokenID).Scan(
		&session.ID,
		&session.UserID,
		&session.CurrentTokenID,
		&session.LastUsedAt,
		&session.CreatedAt,
		&session.ExpiresAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Session{}, fmt.Errorf(
				"matching session: %w",
				domain.ErrNotFound,
			)
		}
		return domain.Session{}, fmt.Errorf("rotate session: %w", err)
	}
	if err := session.Validate(); err != nil {
		return domain.Session{}, err
	}

	return session, nil
}
