// Package postgres is player-service's Postgres-backed implementation of
// domain.PlayerRepository and domain.LedgerRepository — the real
// persistence layer that replaces the in-memory default once this
// service is wired up with a DATABASE_URL.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	// Side-effect import: registers the "pgx" driver with database/sql.
	// We deliberately use the standard database/sql API (sql.Open,
	// QueryContext, …) rather than pgx's own richer native API — pgx here
	// is just the driver underneath, which keeps this code portable to
	// any database/sql driver, not pinned to pgx-specific features.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"

	// Side-effect import: registers the "file" source driver migrate
	// uses to read .up.sql/.down.sql files off disk (the "file://" URL
	// scheme passed to migrate.NewWithDatabaseInstance below).
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Open connects to Postgres and verifies the connection with a Ping —
// failing fast here with a clear error beats discovering a bad
// connection string on the first real request.
//
// database/sql's pool defaults to an *unbounded* number of open
// connections — fine for one process talking to one local Postgres,
// but a real bug waiting to happen once this runs as N horizontally
// scaled replicas (ECS desired_count, a k8s HPA): N replicas each
// opening connections without limit can collectively exceed Postgres'
// own max_connections (100 by default on RDS db.t3.micro), taking the
// database down for every replica at once instead of just slowing one
// down. SetMaxOpenConns caps this service's own share of that budget;
// SetConnMaxLifetime recycles connections periodically so a failover
// or a load balancer in front of Postgres (RDS Proxy, pgbouncer) is
// picked up without a restart.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() // best-effort: we're already returning the ping error, a close error wouldn't add information
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

// Migrate applies every pending migration in migrationsDir (a "file://"
// source golang-migrate reads .up.sql/.down.sql pairs from) to db.
//
// Running migrations automatically on service startup is a convenience
// that fits a small service with one owner; a larger team usually runs
// migrations as a distinct, auditable step in the deploy pipeline instead
// (phase 13 revisits this), so that a bad migration doesn't take down
// every replica trying to boot at once.
func Migrate(db *sql.DB, migrationsDir string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("migrate: build driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate: init: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}
