package users_postgres_repository

import (
	"context"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/postgres"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	"strings"

	"github.com/google/uuid"
)

func (r *UsersRepository) SearchUsers(ctx context.Context, requesterID uuid.UUID, prefix string, limit int) ([]users_service.UserSearchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	db := postgres.GetExecutor(ctx, r.db)
	// '_' is legal in usernames, but must be literal rather than a LIKE wildcard.
	pattern := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix) + "%"
	rows, err := db.Query(ctx, `
		SELECT id, username, first_name, last_name
		FROM users
		WHERE deleted_at IS NULL AND id <> $3
		  AND lower(username) LIKE $2 ESCAPE '\'
		ORDER BY (lower(username) = $1) DESC, lower(username), id
		LIMIT $4
	`, prefix, pattern, requesterID, limit)
	if err != nil {
		return nil, fmt.Errorf("query user search: %w", err)
	}
	defer rows.Close()
	users := make([]users_service.UserSearchResult, 0, limit)
	for rows.Next() {
		var user users_service.UserSearchResult
		if err := rows.Scan(&user.ID, &user.Username, &user.FirstName, &user.LastName); err != nil {
			return nil, fmt.Errorf("scan user search: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read user search: %w", err)
	}
	return users, nil
}
