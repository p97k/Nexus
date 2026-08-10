package main

import (
	"context"
	"log/slog"
	"os"

	"nexus/internal/adapters/offline_cashback"
	"nexus/internal/config"
	"nexus/internal/db"
	"nexus/internal/domain"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
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

	store := db.NewStore(pool)

	// ── Admin user ────────────────────────────────────────────────────────────
	hash, _ := bcrypt.GenerateFromPassword([]byte("nexus_admin_2024"), bcrypt.DefaultCost)
	admin := &domain.User{
		ID:           uuid.NewString(),
		Email:        "admin@nexus.local",
		PasswordHash: string(hash),
		Name:         "Admin",
		Role:         domain.RoleAdmin,
	}
	if err := store.CreateUser(ctx, admin); err != nil {
		slog.Warn("admin user already exists or error", "err", err)
	} else {
		slog.Info("admin user created", "email", admin.Email)
	}

	// ── Offline Cashback project ───────────────────────────────────────────────
	project := &domain.Project{
		ID:          uuid.NewString(),
		Name:        "Offline Cashback",
		Slug:        "offline-cashback",
		Description: "Card registration, transaction ingestion and cashback settlement service",
		AdapterID:   offline_cashback.AdapterID,
	}
	// Resolve the canonical project ID (existing on conflict) so the agents
	// below always reference a row that exists.
	projectID, err := store.UpsertProject(ctx, project)
	if err != nil {
		slog.Warn("upsert project", "err", err)
	} else {
		slog.Info("project upserted", "slug", project.Slug, "id", projectID)
	}

	// ── Card Investigator agent ────────────────────────────────────────────────
	cardAgent := &domain.Agent{
		ID:          uuid.NewString(),
		ProjectID:   projectID,
		Name:        "Card Investigator",
		Description: "Diagnoses card registration failures across PSP banks",
		SystemPrompt: `You are an expert operations engineer for the Offline Cashback platform.
Your job is to investigate card registration problems across PSP banks (Melli, Mellat, Parsian, etc.).

How to get data:
1. The specialized read tools (get_pending_summary, get_stuck_in_progress, get_card_banks,
   get_cards_by_user, get_recent_response_codes) cover the common questions.
2. If the user asks something those tools cannot answer, DO NOT guess:
   call get_schema first to see the real tables/columns, then write your own
   read-only SELECT and run it with query_cashback_db.
   Only SELECT (or WITH) is allowed, single statement, no semicolons, and only tables
   listed in get_schema (users_card, users_card_bank, users_card_log, users_card_event).
   Use aggregations and LIMIT so results stay small.

When a user reports an issue:
1. Always start by querying the data before drawing conclusions.
2. Look at pending counts, failed counts, stuck-in-progress rows, response codes, timestamps.
3. Identify root causes: stuck locks, PSP API errors, batch size gates, etc.
4. Summarize findings clearly for non-technical product managers, citing the numbers you saw.
5. If a write action is needed (e.g., unlock stuck rows), propose it — it will go to a PM for approval.

Be precise, data-driven, and concise. Always show your reasoning.`,
		DefaultMode:     domain.AgentModeAuto,
		DefaultProvider: "google",
		DefaultModel:    "gemini-flash-latest",
		AllowedTools: []string{
			"get_pending_summary",
			"get_stuck_in_progress",
			"get_card_banks",
			"get_cards_by_user",
			"get_recent_response_codes",
			"get_schema",
			"query_cashback_db",
			"unlock_in_progress",
		},
		MaxSteps: 10,
	}
	if err := store.UpsertAgentByName(ctx, cardAgent); err != nil {
		slog.Warn("upsert card agent", "err", err)
	} else {
		slog.Info("agent upserted", "name", cardAgent.Name)
	}

	// ── Reliability Agent ──────────────────────────────────────────────────────
	relAgent := &domain.Agent{
		ID:          uuid.NewString(),
		ProjectID:   projectID,
		Name:        "Reliability Monitor",
		Description: "Proactive queue health and failure pattern detection",
		SystemPrompt: `You are a reliability engineer for the Offline Cashback platform.
Your role is to proactively monitor the health of the card registration pipeline.

Focus on:
- Banks with disproportionately high pending or failed counts
- Systematic PSP API error codes that indicate service-side issues
- Cards stuck in add_in_progress beyond SLA (30 minutes)
- Patterns that suggest batch processing failures

Give clear severity assessments (P0/P1/P2) and recommended actions.`,
		DefaultMode:     domain.AgentModeAuto,
		DefaultProvider: "google",
		DefaultModel:    "gemini-flash-latest",
		AllowedTools: []string{
			"get_pending_summary",
			"get_stuck_in_progress",
			"get_recent_response_codes",
			"get_schema",
			"query_cashback_db",
		},
		MaxSteps: 8,
	}
	if err := store.UpsertAgentByName(ctx, relAgent); err != nil {
		slog.Warn("upsert reliability agent", "err", err)
	} else {
		slog.Info("agent upserted", "name", relAgent.Name)
	}

	slog.Info("seed complete")
}
