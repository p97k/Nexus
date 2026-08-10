package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nexus/internal/adapters"
	"nexus/internal/api"
	"nexus/internal/auth"
	"nexus/internal/config"
	"nexus/internal/db"
	"nexus/internal/llm"
	"nexus/internal/tools"

	"github.com/joho/godotenv"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	godotenv.Load("../.env") // Nexus repo root .env
	godotenv.Load(".env")    // local override

	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ── Nexus DB ──────────────────────────────────────────────────────────────
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("nexus db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(context.Background(), pool); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}

	store := db.NewStore(pool)

	// ── Cashback DB (optional) ────────────────────────────────────────────────
	var cashbackPool *pgxpool.Pool
	if cfg.CashbackAdapterMode == "db" && cfg.CashbackDBURL != "" {
		cashbackPool, err = db.Connect(context.Background(), cfg.CashbackDBURL)
		if err != nil {
			slog.Warn("cashback db connect failed — adapter tools will be unavailable", "err", err)
		} else {
			slog.Info("cashback DB connected")
			defer cashbackPool.Close()
		}
	}

	// ── LLM Router ────────────────────────────────────────────────────────────
	router := llm.NewRouter()
	if cfg.OpenAIKey != "" {
		router.Register(llm.NewOpenAI(cfg.OpenAIKey, cfg.OpenAIDefaultModel))
		slog.Info("provider registered", "name", "openai")
	}
	if cfg.AnthropicKey != "" {
		router.Register(llm.NewAnthropic(cfg.AnthropicKey, cfg.AnthropicDefaultModel))
		slog.Info("provider registered", "name", "anthropic")
	}
	if cfg.GoogleKey != "" {
		router.Register(llm.NewGoogle(cfg.GoogleKey, cfg.GoogleDefaultModel))
		slog.Info("provider registered", "name", "google")
	}
	if cfg.GroqKey != "" {
		router.Register(llm.NewOpenAICompatible("groq", cfg.GroqKey, cfg.GroqBaseURL, cfg.GroqDefaultModel))
		slog.Info("provider registered", "name", "groq", "base", cfg.GroqBaseURL)
	}
	if len(router.Providers()) == 0 {
		slog.Warn("no LLM providers configured — set OPENAI/ANTHROPIC/GOOGLE/GROQ API keys")
	}

	// ── Tool Registry ─────────────────────────────────────────────────────────
	toolReg := tools.NewRegistry()
	adapters.RegisterAll(toolReg, cashbackPool, cfg.CashbackCacheTTL)

	// ── Auth ──────────────────────────────────────────────────────────────────
	authSvc := auth.NewService(cfg.JWTSecret, cfg.JWTExpiryHours)

	// ── HTTP Server ───────────────────────────────────────────────────────────
	srv := api.NewServer(cfg, store, authSvc, router, toolReg, cashbackPool, cfg.LLMFallbackModels)
	httpSrv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("nexus server starting", "port", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	httpSrv.Shutdown(shutCtx)
	slog.Info("nexus server stopped")
}
