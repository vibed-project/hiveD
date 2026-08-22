// Package obs wires up the Keeper's structured logging and metrics
// endpoint. OpenTelemetry tracing is not wired in M0 — there is nothing
// yet worth tracing beyond a single request/store round trip — but the
// package exists as the seam M1 hangs a tracer provider on.
package obs

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewLogger builds the process-wide slog.Logger. format is "json" or
// "text"; level is any slog.Level name ("debug", "info", "warn", "error").
func NewLogger(format, level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.New(handler)
}

// MetricsHandler serves Prometheus metrics. cmd/keeper mounts it on a
// separate listener/port from application traffic.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
