package users_postgres_repository

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"github.com/wrzdx/Nero/internal/core/postgres"
)

func (r *UsersRepository) CreateUser(
	ctx context.Context,
	user domain.User,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	db := postgres.GetExecutor(ctx, r.db)

	query := `
	INSERT INTO users (id, username, first_name, last_name, created_at, deleted_at, bio, password_hash)
	VALUES ($1, $2,$3,$4,$5,$6, $7,$8);
	`

	_, err := db.Exec(
		ctx,
		query,
		user.ID,
		user.Profile.Username,
		user.Profile.FirstName,
		user.Profile.LastName,
		user.CreatedAt,
		user.DeletedAt,
		user.Profile.Bio,
		user.PasswordHash,
	)
	if err != nil {
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
		return fmt.Errorf("scan error: %w", err)
	}

	return nil
}
