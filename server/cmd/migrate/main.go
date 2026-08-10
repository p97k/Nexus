package main

import (
	"context"
	"log/slog"
	"os"

	"nexus/internal/config"
	"nexus/internal/db"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load("../.env")
	godotenv.Load(".env")

	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
	slog.Info("migration complete")
}
