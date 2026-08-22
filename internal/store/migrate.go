package store

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies (direction="up"), reverts (direction="down"), or reports
// (direction="status") the state of internal/store/migrations against db.
// db must be a *sql.DB opened with the pgx stdlib driver
// ("github.com/jackc/pgx/v5/stdlib"), which cmd/keeper does before calling
// this so goose (a database/sql tool) and PostgresStore (a pgx/v5 pool)
// can share one Postgres connection string without pulling pgx pool
// semantics into goose.
func Migrate(db *sql.DB, direction string) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("store: set goose dialect: %w", err)
	}

	switch direction {
	case "up":
		return goose.Up(db, "migrations")
	case "down":
		return goose.Down(db, "migrations")
	case "status":
		return goose.Status(db, "migrations")
	default:
		return fmt.Errorf("store: unknown migrate direction %q", direction)
	}
}
