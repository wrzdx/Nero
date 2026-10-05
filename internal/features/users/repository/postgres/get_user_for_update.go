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

func (r *UsersRepository) GetUserForUpdate(
	ctx context.Context,
	id uuid.UUID,
) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	SELECT id,
		   username,
		   first_name,
		   last_name,
		   created_at,
		   deleted_at,
		   bio,
		   password_hash
	FROM users
	WHERE id=$1
	FOR UPDATE;
	`

	row := db.QueryRow(ctx, query, id)

	var userModel UserModel
	err := row.Scan(
		&userModel.ID,
		&userModel.Username,
		&userModel.FirstName,
		&userModel.LastName,
		&userModel.CreatedAt,
		&userModel.DeletedAt,
		&userModel.Bio,
		&userModel.PasswordHash,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}

		return domain.User{}, fmt.Errorf("scan user by id: %w", err)
	}

	userDomain, err := UserDomainFromModel(userModel)
	if err != nil {
		return domain.User{}, err
	}
	return userDomain, nil
}
