package users_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *UsersRepository) UpdateUserProfile(
	ctx context.Context,
	id uuid.UUID,
	profile domain.UserProfile,
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
		bio=$4
	WHERE id=$5
		AND deleted_at IS NULL
	RETURNING id;`

	err := db.QueryRow(
		ctx,
		query,
		profile.Username,
		profile.FirstName,
		profile.LastName,
		profile.Bio,
		id,
	).Scan(&id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		details := make(map[string]string)
		if postgres.IsConstraintViolation(err, postgres.UniqueViolation, usernameUK) {
			details["username"] = "username already taken"
		}
		if len(details) > 0 {
			return domain.DetailedError{
				Err:     fmt.Errorf("user %w", domain.ErrAlreadyExists),
				Details: details,
			}
		}
		return fmt.Errorf("update user profile: %w", err)
	}

	return nil
}
