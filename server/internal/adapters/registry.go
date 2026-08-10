package adapters

import (
	"time"

	"nexus/internal/adapters/offline_cashback"
	"nexus/internal/tools"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterAll registers all project adapters into the tool registry.
// cashbackPool may be nil if CASHBACK_DB_URL is not configured.
// cacheTTLSeconds enables in-memory tool-result caching (0 = disabled).
func RegisterAll(reg *tools.Registry, cashbackPool *pgxpool.Pool, cacheTTLSeconds int) {
	reg.Register(offline_cashback.New(cashbackPool, time.Duration(cacheTTLSeconds)*time.Second))
}
