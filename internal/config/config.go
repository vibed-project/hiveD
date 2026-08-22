// Package config holds the Keeper's environment-driven configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	PGDSN        string
	ListenAddr   string
	MetricsAddr  string
	LogLevel     string
	LogFormat    string
	AutoMigrate  bool
}

// FromEnv reads HIVED_* environment variables, applying the defaults
// documented in README.md / CONTRIBUTING.md.
func FromEnv() (Config, error) {
	c := Config{
		PGDSN:       getenv("HIVED_PG_DSN", "postgres://hived:hived@localhost:5432/hived?sslmode=disable"),
		ListenAddr:  getenv("HIVED_LISTEN_ADDR", ":8080"),
		MetricsAddr: getenv("HIVED_METRICS_ADDR", ":9090"),
		LogLevel:    getenv("HIVED_LOG_LEVEL", "info"),
		LogFormat:   getenv("HIVED_LOG_FORMAT", "json"),
		AutoMigrate: true,
	}
	if v := os.Getenv("HIVED_AUTO_MIGRATE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: HIVED_AUTO_MIGRATE: %w", err)
		}
		c.AutoMigrate = b
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
