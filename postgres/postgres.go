package postgres

import (
	"context"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations
var migrations embed.FS

type Store struct {
	*Queries
	conn *pgx.Conn
}

func NewStore(ctx context.Context, databaseURL string) (*Store, error) {
	conf, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}
	if err := ensureMigrations(ctx, *conf); err != nil {
		return nil, fmt.Errorf("failed to ensure migrations: %w", err)
	}
	conn, err := pgx.ConnectConfig(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Store{
		conn:    conn,
		Queries: New(conn),
	}, nil
}

func ensureMigrations(ctx context.Context, conf pgx.ConnConfig) (retErr error) {
	source, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migrations source: %w", err)
	}
	db := stdlib.OpenDB(conf)
	defer func() {
		err := db.Close()
		if retErr == nil && err != nil {
			retErr = fmt.Errorf("failed to close database: %w", err)
		}
	}()
	target, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migrations target: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "postgres", target)
	if err != nil {
		return fmt.Errorf("failed to create migrations instance: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

func (s *Store) Close(ctx context.Context) error {
	return s.conn.Close(ctx)
}
