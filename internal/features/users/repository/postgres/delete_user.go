package users_postgres_repository

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

func (r *UsersRepository) DeleteUser(
	ctx context.Context,
	id uuid.UUID,
	profile domain.UserProfile,
	deletedAt time.Time,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)
	query := `
	UPDATE users
	SET 
		username=$1,
		first_name=$2,
		last_name=$3,
		deleted_at=$4,
		bio=$5
	WHERE id=$6 AND deleted_at IS NULL
	RETURNING id;
	`

	err := db.QueryRow(
		ctx,
		query,
		profile.Username,
		profile.FirstName,
		profile.LastName,
		deletedAt,
		profile.Bio,
		id,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("delete user query: %w", err)
	}

	return nil
}
