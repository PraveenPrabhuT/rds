package core

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// NewPgxConn creates a new pgx connection to a PostgreSQL database with sslmode=require.
func NewPgxConn(ctx context.Context, host string, port int32, user, password, dbname string) (*pgx.Conn, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
		host, port, user, password, dbname,
	)
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("connect to %s:%d/%s: %w", host, port, dbname, err)
	}
	return conn, nil
}
