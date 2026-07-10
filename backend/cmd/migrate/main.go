// Command migrate applies the embedded database migrations and exits.
// It reads FLEETDOCK_DATABASE_URL from the environment.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/TajBrains/fleetdock/backend/internal/config"
	"github.com/TajBrains/fleetdock/backend/internal/infra/postgres"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err.Error())
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect", "error", err.Error())
		os.Exit(1)
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool); err != nil {
		slog.Error("migrate", "error", err.Error())
		os.Exit(1)
	}
	slog.Info("migrations applied successfully")
}
