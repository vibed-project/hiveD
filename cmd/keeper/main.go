// Command hived-keeper is the control plane binary: serve runs the API
// server, migrate manages the Postgres schema, version prints build info.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/vibed-project/hiveD/internal/api"
	"github.com/vibed-project/hiveD/internal/config"
	"github.com/vibed-project/hiveD/internal/identity"
	"github.com/vibed-project/hiveD/internal/obs"
	"github.com/vibed-project/hiveD/internal/store"
	"github.com/vibed-project/hiveD/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: hived-keeper <serve|migrate|version> ...")
	}

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe()
	case "migrate":
		err = runMigrate(os.Args[2:])
	case "version":
		fmt.Printf("hived-keeper %s (commit %s, built %s)\n", version.Version, version.Commit, version.BuildDate)
	default:
		fatalf("unknown subcommand %q; usage: hived-keeper <serve|migrate|version> ...", os.Args[1])
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func runServe() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	logger := obs.NewLogger(cfg.LogFormat, cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.AutoMigrate {
		db, err := sql.Open("pgx", cfg.PGDSN)
		if err != nil {
			return fmt.Errorf("open db for migrate: %w", err)
		}
		if err := store.Migrate(db, "up"); err != nil {
			_ = db.Close()
			return fmt.Errorf("auto-migrate: %w", err)
		}
		_ = db.Close()
		logger.Info("migrations applied")
	}

	pool, err := pgxpool.New(ctx, cfg.PGDSN)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	resourceStore := store.NewPostgresStore(pool)
	ready := func(ctx context.Context) error { return pool.Ping(ctx) }

	handler := api.NewHandler(resourceStore, resourceStore, identity.StubVerifier{}, logger, ready)

	// connect-go serves gRPC, which needs HTTP/2. Enabling unencrypted HTTP/2
	// via Protocols (rather than the deprecated golang.org/x/net/http2/h2c
	// package) lets gRPC work without TLS in the compose dev stack.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	apiServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}
	metricsServer := &http.Server{
		Addr:              cfg.MetricsAddr,
		Handler:           obs.MetricsHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		logger.Info("api server listening", "addr", cfg.ListenAddr)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("api server: %w", err)
		}
	}()
	go func() {
		logger.Info("metrics server listening", "addr", cfg.MetricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("metrics server: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = apiServer.Shutdown(shutdownCtx)
	_ = metricsServer.Shutdown(shutdownCtx)
	return nil
}

func runMigrate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: hived-keeper migrate <up|down|status>")
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.PGDSN)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()
	return store.Migrate(db, args[0])
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
