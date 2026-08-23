//go:build integration

package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func testDSN() string {
	if dsn := os.Getenv("HIVED_TEST_PG_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://hived:hived@localhost:5432/hived?sslmode=disable"
}

// setupPostgres connects, migrates, and returns a pool shared by every
// subtest in this file; each subtest's newStore truncates the tables first
// so subtests stay independent despite sharing one database.
func setupPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testDSN()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		// `make test-integration` sets HIVED_REQUIRE_PG=1. Without it a
		// developer running `go test -tags=integration` by hand still gets a
		// skip, but the documented command can never report a false green.
		if os.Getenv("HIVED_REQUIRE_PG") != "" {
			t.Fatalf("postgres required but not reachable at %s: %v", dsn, err)
		}
		t.Skipf("postgres not reachable at %s (run `make compose-up` first): %v", dsn, err)
	}
	if err := Migrate(db, "up"); err != nil {
		t.Fatalf("Migrate up: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE resources, events`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// Reset the resource_version counter as well: several conformance cases
	// depend on starting from a genuinely empty store (the first resource
	// ever created used to be invisible to a list-then-watch client).
	if _, err := pool.Exec(context.Background(),
		`UPDATE hived_resource_version_counter SET value = 0`); err != nil {
		t.Fatalf("reset resource version counter: %v", err)
	}
}

func TestPostgresStore_ResourceStoreConformance(t *testing.T) {
	pool := setupPostgres(t)
	runResourceStoreConformance(t, func(t *testing.T) ResourceStore {
		truncate(t, pool)
		return NewPostgresStore(pool)
	})
}

func TestPostgresStore_EventStoreConformance(t *testing.T) {
	pool := setupPostgres(t)
	runEventStoreConformance(t, func(t *testing.T) EventStore {
		truncate(t, pool)
		return NewPostgresStore(pool)
	})
}
