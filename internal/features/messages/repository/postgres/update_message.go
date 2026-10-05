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

func (r *Repository) UpdateMessage(
	ctx context.Context,
	updated domain.Message,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	query := `
	UPDATE messages
	SET content=$1, updated_at=$2
	WHERE id=$3
	RETURNING id
	`

	var id uuid.UUID

	if err := db.QueryRow(
		ctx,
		query,
		updated.Content,
		updated.UpdatedAt,
		updated.ID,
	).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("update message sql query: %w", err)
	}

	return nil
}
