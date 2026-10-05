//go:build integration

package users_postgres_repository

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/wrzdx/Nero/internal/core/postgres"
	"strings"
	"testing"
	"time"
)

func TestSearchUsersPrefixAndVisibility(t *testing.T) {
	config := postgres.NewConfigMust()
	pool, err := postgres.NewPool(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	tx, repo := newGetUserTestRepository(t, pool, config.Timeout)
	prefix := "s" + uuid.NewString()[:8] + "_"
	ids := map[string]uuid.UUID{}
	for _, suffix := range []string{"", "a", "b", "self", "deleted", "wildcard"} {
		user := newGetUserRepositoryTestUser(t, nil, nil, nil)
		user.Profile.Username = prefix + suffix
		if suffix == "a" {
			user.Profile.Username = strings.ToUpper(user.Profile.Username)
		}
		if suffix == "wildcard" {
			user.Profile.Username = prefix[:len(prefix)-1] + "xother"
		}
		if suffix == "deleted" {
			now := time.Now()
			user.DeletedAt = &now
		}
		insertGetUserTestUser(t, tx, user)
		ids[suffix] = user.ID
	}
	users, err := repo.SearchUsers(t.Context(), ids["self"], prefix, 20)
	require.NoError(t, err)
	require.Len(t, users, 3)
	require.Equal(t, []uuid.UUID{ids[""], ids["a"], ids["b"]}, []uuid.UUID{users[0].ID, users[1].ID, users[2].ID})
	users, err = repo.SearchUsers(t.Context(), ids["self"], prefix, 1)
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, ids[""], users[0].ID)
}
