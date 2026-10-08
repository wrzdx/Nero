package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, config Config) (*pgxpool.Pool, error) {
	connectionURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.User, config.Password),
		Host:   net.JoinHostPort(config.Host, config.Port),
		Path:   "/" + config.Database,
	}

	params := url.Values{}
	params.Set("sslmode", config.SSLMode)

	if config.SSLMode == "verify-full" {
		params.Set("sslrootcert", config.SSLRootCert)
	}

	connectionURL.RawQuery = params.Encode()
	connectionString := connectionURL.String()

	pgxConfig, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, fmt.Errorf("parse pgxconfig: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxConfig)
	if err != nil {
		return nil, fmt.Errorf("create pgxpool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgxpool ping: %w", err)
	}

	return pool, nil
}
