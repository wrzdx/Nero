package messages_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) DeleteMessage(
	ctx context.Context,
	id uuid.UUID,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	query := `--sql
	DELETE FROM messages
	WHERE id=$1
	RETURNING id;
	`

	if err := db.QueryRow(ctx, query, id).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("delete message sql: %w", err)
	}

	return nil
}
