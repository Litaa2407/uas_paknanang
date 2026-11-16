package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ensureGoogleOAuthSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		ALTER TABLE users ADD COLUMN IF NOT EXISTS google_sub TEXT;
		CREATE UNIQUE INDEX IF NOT EXISTS users_google_sub_unique
			ON users (google_sub) WHERE google_sub IS NOT NULL;
	`)
	return err
}
