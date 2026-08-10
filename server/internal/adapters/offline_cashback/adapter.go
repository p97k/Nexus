package offline_cashback

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nexus/internal/cache"
	"nexus/internal/tools"

	"github.com/jackc/pgx/v5/pgxpool"
)

const AdapterID = "offline-cashback"

// Adapter provides read-only and write tools for the offline-cashback project.
// In "db" mode it uses a direct read-only Postgres connection.
// In "http" mode it calls the Laravel Ops API.
type Adapter struct {
	pool  *pgxpool.Pool // nil when using HTTP mode
	cache *cache.Cache  // nil when caching disabled
}

func New(pool *pgxpool.Pool, cacheTTL time.Duration) *Adapter {
	a := &Adapter{pool: pool}
	if cacheTTL > 0 {
		a.cache = cache.New(cacheTTL)
	}
	return a
}

func (a *Adapter) AdapterID() string { return AdapterID }

func (a *Adapter) Tools() []*tools.Tool {
	return []*tools.Tool{
		a.toolGetPendingSummary(),
		a.toolGetStuckInProgress(),
		a.toolGetCardBanks(),
		a.toolGetCardsByUser(),
		a.toolGetRecentResponseCodes(),
		a.toolGetSchema(),        // read-only: DB schema introspection
		a.toolQueryCashbackDB(),  // read-only: ad-hoc SELECT the model writes
		a.toolUnlockInProgress(), // write tool — requires approval
	}
}

// ─── helper ───────────────────────────────────────────────────────────────────

func (a *Adapter) requireDB() error {
	if a.pool == nil {
		return fmt.Errorf("cashback DB not configured (set CASHBACK_DB_URL)")
	}
	return nil
}

// cacheKey hashes the tool name + input so identical or repeated calls (e.g.
// the same diagnostic query across prompts) hit the in-memory cache.
func (a *Adapter) cacheKey(name string, input json.RawMessage) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte{0})
	h.Write(input)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// cached runs fn unless a live entry exists for key. Errors are never cached.
func (a *Adapter) cached(key string, fn func() (json.RawMessage, error)) (json.RawMessage, error) {
	if a.cache == nil {
		return fn()
	}
	if v, ok := a.cache.Get(key); ok {
		return v, nil
	}
	v, err := fn()
	if err != nil {
		return nil, err
	}
	a.cache.Set(key, v)
	return v, nil
}

func jsonSchema(s string) json.RawMessage { return json.RawMessage(s) }

// ─── Read Tools ───────────────────────────────────────────────────────────────

func (a *Adapter) toolGetPendingSummary() *tools.Tool {
	return &tools.Tool{
		Name:        "get_pending_summary",
		Description: "Returns pending and failed card-bank registration counts per bank. Optionally filter to a specific bank or cards stuck longer than N minutes.",
		ReadOnly:    true,
		Parameters: jsonSchema(`{
			"type": "object",
			"properties": {
				"bank_name":           {"type": "string", "description": "Filter to a specific bank (e.g. 'melli', 'mellat'). Leave empty for all."},
				"older_than_minutes":  {"type": "integer", "description": "Only include records older than this many minutes. Default 0 (all)."}
			}
		}`),
		Execute: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return a.cached(a.cacheKey("get_pending_summary", input), func() (json.RawMessage, error) {
				if err := a.requireDB(); err != nil {
					return nil, err
				}
				var args struct {
					BankName         string `json:"bank_name"`
					OlderThanMinutes int    `json:"older_than_minutes"`
				}
				json.Unmarshal(input, &args)

				query := `
				SELECT
					bank_name,
					COUNT(*) FILTER (WHERE status = 'pending' AND add_in_progress = false) AS pending,
					COUNT(*) FILTER (WHERE status = 'failed')                               AS failed,
					COUNT(*) FILTER (WHERE add_in_progress = true)                          AS in_progress
				FROM users_card_bank
				WHERE 1=1
			`
				var params []any
				if args.BankName != "" {
					params = append(params, args.BankName)
					query += fmt.Sprintf(" AND bank_name = $%d", len(params))
				}
				if args.OlderThanMinutes > 0 {
					params = append(params, args.OlderThanMinutes)
					query += fmt.Sprintf(" AND created_at < NOW() - ($%d * INTERVAL '1 minute')", len(params))
				}
				query += " GROUP BY bank_name ORDER BY pending DESC"

				rows, err := a.pool.Query(ctx, query, params...)
				if err != nil {
					return nil, err
				}
				defer rows.Close()

				type Row struct {
					BankName   string `json:"bank_name"`
					Pending    int    `json:"pending"`
					Failed     int    `json:"failed"`
					InProgress int    `json:"in_progress"`
				}
				result := []Row{}
				for rows.Next() {
					var r Row
					rows.Scan(&r.BankName, &r.Pending, &r.Failed, &r.InProgress)
					result = append(result, r)
				}
				return json.Marshal(result)
			})
		},
	}
}

func (a *Adapter) toolGetStuckInProgress() *tools.Tool {
	return &tools.Tool{
		Name:        "get_stuck_in_progress",
		Description: "Lists card-bank rows where add_in_progress=true for more than 30 minutes. These are candidates for unlocking.",
		ReadOnly:    true,
		Parameters: jsonSchema(`{
			"type": "object",
			"properties": {
				"bank_name":         {"type": "string", "description": "Filter to a specific bank."},
				"stuck_minutes":     {"type": "integer", "description": "Consider stuck if in_progress for longer than N minutes. Default 30."},
				"limit":             {"type": "integer", "description": "Max rows to return. Default 50."}
			}
		}`),
		Execute: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return a.cached(a.cacheKey("get_stuck_in_progress", input), func() (json.RawMessage, error) {
				if err := a.requireDB(); err != nil {
					return nil, err
				}
				var args struct {
					BankName     string `json:"bank_name"`
					StuckMinutes int    `json:"stuck_minutes"`
					Limit        int    `json:"limit"`
				}
				json.Unmarshal(input, &args)
				if args.StuckMinutes == 0 {
					args.StuckMinutes = 30
				}
				if args.Limit == 0 {
					args.Limit = 50
				}

				query := `
				SELECT id, card_id, bank_name, status, updated_at,
				       EXTRACT(EPOCH FROM (NOW() - updated_at))::int / 60 AS stuck_minutes
				FROM users_card_bank
				WHERE add_in_progress = true
				  AND updated_at < NOW() - ($1 * INTERVAL '1 minute')
			`
				params := []any{args.StuckMinutes}
				if args.BankName != "" {
					params = append(params, args.BankName)
					query += fmt.Sprintf(" AND bank_name = $%d", len(params))
				}
				query += fmt.Sprintf(" ORDER BY updated_at ASC LIMIT $%d", len(params)+1)
				params = append(params, args.Limit)

				rows, err := a.pool.Query(ctx, query, params...)
				if err != nil {
					return nil, err
				}
				defer rows.Close()

				type Row struct {
					ID           int       `json:"id"`
					CardID       int       `json:"card_id"`
					BankName     string    `json:"bank_name"`
					Status       string    `json:"status"`
					UpdatedAt    time.Time `json:"updated_at"`
					StuckMinutes int       `json:"stuck_minutes"`
				}
				result := []Row{}
				for rows.Next() {
					var r Row
					rows.Scan(&r.ID, &r.CardID, &r.BankName, &r.Status, &r.UpdatedAt, &r.StuckMinutes)
					result = append(result, r)
				}
				return json.Marshal(map[string]any{
					"stuck_rows": result,
					"count":      len(result),
				})
			})
		},
	}
}

func (a *Adapter) toolGetCardBanks() *tools.Tool {
	return &tools.Tool{
		Name:        "get_card_banks",
		Description: "Returns the bank registration status for a specific card ID.",
		ReadOnly:    true,
		Parameters: jsonSchema(`{
			"type": "object",
			"required": ["card_id"],
			"properties": {
				"card_id": {"type": "integer", "description": "The card ID from users_card table."}
			}
		}`),
		Execute: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return a.cached(a.cacheKey("get_card_banks", input), func() (json.RawMessage, error) {
				if err := a.requireDB(); err != nil {
					return nil, err
				}
				var args struct {
					CardID int `json:"card_id"`
				}
				json.Unmarshal(input, &args)

				rows, err := a.pool.Query(ctx, `
				SELECT id, bank_name, status, add_in_progress, response_code, response_desc,
				       created_at, updated_at
				FROM users_card_bank
				WHERE card_id = $1
				ORDER BY bank_name
			`, args.CardID)
				if err != nil {
					return nil, err
				}
				defer rows.Close()

				type Row struct {
					ID           int       `json:"id"`
					BankName     string    `json:"bank_name"`
					Status       string    `json:"status"`
					InProgress   bool      `json:"add_in_progress"`
					ResponseCode string    `json:"response_code"`
					ResponseDesc string    `json:"response_desc"`
					CreatedAt    time.Time `json:"created_at"`
					UpdatedAt    time.Time `json:"updated_at"`
				}
				result := []Row{}
				for rows.Next() {
					var r Row
					rows.Scan(&r.ID, &r.BankName, &r.Status, &r.InProgress,
						&r.ResponseCode, &r.ResponseDesc, &r.CreatedAt, &r.UpdatedAt)
					result = append(result, r)
				}
				return json.Marshal(map[string]any{
					"card_id": args.CardID,
					"banks":   result,
				})
			})
		},
	}
}

func (a *Adapter) toolGetCardsByUser() *tools.Tool {
	return &tools.Tool{
		Name:        "get_cards_by_user",
		Description: "Returns all cards registered by a specific user ID, including their overall status.",
		ReadOnly:    true,
		Parameters: jsonSchema(`{
			"type": "object",
			"required": ["user_id"],
			"properties": {
				"user_id": {"type": "integer", "description": "The Magento customer entity ID."}
			}
		}`),
		Execute: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return a.cached(a.cacheKey("get_cards_by_user", input), func() (json.RawMessage, error) {
				if err := a.requireDB(); err != nil {
					return nil, err
				}
				var args struct {
					UserID int `json:"user_id"`
				}
				json.Unmarshal(input, &args)

				rows, err := a.pool.Query(ctx, `
				SELECT c.id, LEFT(c.card_no::text, 6) AS prefix, c.status, c.created_at,
				       COUNT(cb.id)                                          AS total_banks,
				       COUNT(cb.id) FILTER (WHERE cb.status = 'confirmed')  AS confirmed_banks,
				       COUNT(cb.id) FILTER (WHERE cb.status = 'failed')     AS failed_banks
				FROM users_card c
				LEFT JOIN users_card_bank cb ON cb.card_id = c.id
				WHERE c.user_id = $1
				GROUP BY c.id, c.card_no, c.status, c.created_at
				ORDER BY c.created_at DESC
			`, args.UserID)
				if err != nil {
					return nil, err
				}
				defer rows.Close()

				type Row struct {
					ID             int       `json:"id"`
					CardNoPrefix   string    `json:"card_no_prefix"`
					Status         string    `json:"status"`
					CreatedAt      time.Time `json:"created_at"`
					TotalBanks     int       `json:"total_banks"`
					ConfirmedBanks int       `json:"confirmed_banks"`
					FailedBanks    int       `json:"failed_banks"`
				}
				result := []Row{}
				for rows.Next() {
					var r Row
					rows.Scan(&r.ID, &r.CardNoPrefix, &r.Status, &r.CreatedAt,
						&r.TotalBanks, &r.ConfirmedBanks, &r.FailedBanks)
					result = append(result, r)
				}
				return json.Marshal(map[string]any{
					"user_id": args.UserID,
					"cards":   result,
				})
			})
		},
	}
}

func (a *Adapter) toolGetRecentResponseCodes() *tools.Tool {
	return &tools.Tool{
		Name:        "get_recent_response_codes",
		Description: "Returns the distribution of PSP response codes for a bank over the last N hours. Useful for diagnosing systematic API errors.",
		ReadOnly:    true,
		Parameters: jsonSchema(`{
			"type": "object",
			"required": ["bank_name"],
			"properties": {
				"bank_name":  {"type": "string", "description": "The bank to query (e.g. 'melli', 'mellat')."},
				"hours":      {"type": "integer", "description": "Lookback window in hours. Default 24."},
				"limit":      {"type": "integer", "description": "Max distinct codes to return. Default 20."}
			}
		}`),
		Execute: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return a.cached(a.cacheKey("get_recent_response_codes", input), func() (json.RawMessage, error) {
				if err := a.requireDB(); err != nil {
					return nil, err
				}
				var args struct {
					BankName string `json:"bank_name"`
					Hours    int    `json:"hours"`
					Limit    int    `json:"limit"`
				}
				json.Unmarshal(input, &args)
				if args.Hours == 0 {
					args.Hours = 24
				}
				if args.Limit == 0 {
					args.Limit = 20
				}

				rows, err := a.pool.Query(ctx, `
				SELECT COALESCE(response_code,'(empty)') AS response_code,
				       COALESCE(response_desc,'')         AS response_desc,
				       COUNT(*)                            AS count
				FROM users_card_bank
				WHERE bank_name = $1
				  AND updated_at > NOW() - ($2 * INTERVAL '1 hour')
				  AND status != 'confirmed'
				GROUP BY response_code, response_desc
				ORDER BY count DESC
				LIMIT $3
			`, args.BankName, args.Hours, args.Limit)
				if err != nil {
					return nil, err
				}
				defer rows.Close()

				type Row struct {
					ResponseCode string `json:"response_code"`
					ResponseDesc string `json:"response_desc"`
					Count        int    `json:"count"`
				}
				result := []Row{}
				for rows.Next() {
					var r Row
					rows.Scan(&r.ResponseCode, &r.ResponseDesc, &r.Count)
					result = append(result, r)
				}
				return json.Marshal(map[string]any{
					"bank_name": args.BankName,
					"hours":     args.Hours,
					"codes":     result,
				})
			})
		},
	}
}

// ─── Dynamic SQL Tools ────────────────────────────────────────────────────────

// allowedTables whitelists the tables the model may query via
// query_cashback_db. Everything else is rejected.
var allowedTables = map[string]bool{
	"users_card":       true,
	"users_card_bank":  true,
	"users_card_log":   true,
	"users_card_event": true,
}

var (
	reForbidden = regexp.MustCompile(`(?i)\b(insert|update|delete|drop|alter|truncate|create|grant|revoke|copy|comment|vacuum|reindex|merge|call|do|prepare|execute)\b`)
	reFromJoin  = regexp.MustCompile(`(?i)\b(?:from|join)\s+(?:[a-z_][a-z0-9_]*\.)?([a-z_][a-z0-9_]*)`)
	reLimit     = regexp.MustCompile(`(?i)\blimit\s+\d+`)
)

// sanitizeSQL validates that a model-written query is a single read-only
// SELECT over an allowed table, and normalizes it (whitelisted unqualified
// names, enforced LIMIT). Returns the final SQL or an error.
func sanitizeSQL(raw string) (string, error) {
	sql := strings.TrimSpace(raw)
	if sql == "" {
		return "", fmt.Errorf("empty SQL")
	}
	if strings.Contains(sql, ";") {
		return "", fmt.Errorf("multiple statements are not allowed")
	}
	if !strings.HasPrefix(strings.ToLower(sql), "select") &&
		!strings.HasPrefix(strings.ToLower(sql), "with") {
		return "", fmt.Errorf("only SELECT queries are allowed")
	}
	if reForbidden.MatchString(sql) {
		return "", fmt.Errorf("query contains a forbidden keyword")
	}

	tables := reFromJoin.FindAllStringSubmatch(sql, -1)
	if len(tables) == 0 {
		return "", fmt.Errorf("no FROM/JOIN clause found")
	}
	for _, m := range tables {
		name := m[1]
		if !allowedTables[name] {
			return "", fmt.Errorf("table %q is not in the allowed set", name)
		}
	}

	// Quote-less identifiers are fine; also allow dotted public.table.
	sql = strings.ReplaceAll(sql, "public.", "")

	if !reLimit.MatchString(sql) {
		sql = strings.TrimRight(sql, "; \t\r\n") + "\nLIMIT 100"
	}
	return sql, nil
}

func (a *Adapter) toolGetSchema() *tools.Tool {
	return &tools.Tool{
		Name:        "get_schema",
		Description: "Returns the tables and columns (with data types) available in the cashback database. Call this before writing a query_cashback_db SQL so you reference real column names.",
		ReadOnly:    true,
		Parameters: jsonSchema(`{
			"type": "object",
			"properties": {}
		}`),
		Execute: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return a.cached(a.cacheKey("get_schema", input), func() (json.RawMessage, error) {
				if err := a.requireDB(); err != nil {
					return nil, err
				}
				rows, err := a.pool.Query(ctx, `
					SELECT table_name, column_name, data_type
					FROM information_schema.columns
					WHERE table_schema = 'public'
					ORDER BY table_name, ordinal_position
				`)
				if err != nil {
					return nil, err
				}
				defer rows.Close()

				type Column struct {
					Name string `json:"name"`
					Type string `json:"type"`
				}
				type Table struct {
					Table   string   `json:"table"`
					Columns []Column `json:"columns"`
				}
				order := []string{}
				byName := map[string]*Table{}
				for rows.Next() {
					var t, c, typ string
					if err := rows.Scan(&t, &c, &typ); err != nil {
						return nil, err
					}
					tt, ok := byName[t]
					if !ok {
						tt = &Table{Table: t}
						byName[t] = tt
						order = append(order, t)
					}
					tt.Columns = append(tt.Columns, Column{Name: c, Type: typ})
				}
				result := make([]Table, 0, len(order))
				for _, t := range order {
					result = append(result, *byName[t])
				}
				return json.Marshal(result)
			})
		},
	}
}

func (a *Adapter) toolQueryCashbackDB() *tools.Tool {
	return &tools.Tool{
		Name: "query_cashback_db",
		Description: `Runs a read-only SELECT you write against the cashback database and returns the rows.
Before calling, inspect get_schema for real table/column names.
Rules: single SELECT (or WITH) only; only tables users_card, users_card_bank, users_card_log, users_card_event;
no semicolons, no writes/DDL. A LIMIT is enforced automatically. Use aggregations to keep results small.`,
		ReadOnly: true,
		Parameters: jsonSchema(`{
			"type": "object",
			"required": ["sql"],
			"properties": {
				"sql": {"type": "string", "description": "The read-only SELECT to run, e.g. SELECT bank_name, COUNT(*) FROM users_card_bank WHERE status='failed' GROUP BY bank_name"}
			}
		}`),
		Execute: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			var args struct {
				SQL string `json:"sql"`
			}
			json.Unmarshal(input, &args)

			return a.cached(a.cacheKey("query_cashback_db", input), func() (json.RawMessage, error) {
				if err := a.requireDB(); err != nil {
					return nil, err
				}
				sql, err := sanitizeSQL(args.SQL)
				if err != nil {
					return nil, fmt.Errorf("query rejected: %w", err)
				}

				qctx, cancel := context.WithTimeout(ctx, 20*time.Second)
				defer cancel()
				rows, err := a.pool.Query(qctx, sql)
				if err != nil {
					return nil, fmt.Errorf("query failed: %w", err)
				}
				defer rows.Close()

				fields := rows.FieldDescriptions()
				cols := make([]string, len(fields))
				for i, f := range fields {
					cols[i] = string(f.Name)
				}

				const maxRows = 500
				var out []map[string]any
				for rows.Next() {
					vals, err := rows.Values()
					if err != nil {
						return nil, err
					}
					row := make(map[string]any, len(cols))
					for i, v := range vals {
						row[cols[i]] = v
					}
					out = append(out, row)
					if len(out) >= maxRows {
						break
					}
				}
				truncated := len(out) >= maxRows
				return json.Marshal(map[string]any{
					"sql":       sql,
					"columns":   cols,
					"rows":      out,
					"count":     len(out),
					"truncated": truncated,
				})
			})
		},
	}
}

// ─── Write Tools (require ActionProposal) ────────────────────────────────────

func (a *Adapter) toolUnlockInProgress() *tools.Tool {
	return &tools.Tool{
		Name: "unlock_in_progress",
		Description: `[WRITE — requires PM approval]
Resets add_in_progress=false for the given users_card_bank IDs.
Use this when cards are stuck and the PSP job has permanently failed.
Always call get_stuck_in_progress first to identify the IDs.`,
		ReadOnly: false,
		Parameters: jsonSchema(`{
			"type": "object",
			"required": ["ids"],
			"properties": {
				"ids": {
					"type": "array",
					"items": {"type": "integer"},
					"description": "List of users_card_bank IDs to unlock."
				},
				"reason": {"type": "string", "description": "Reason for unlock (for audit trail)."}
			}
		}`),
		Execute: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			if err := a.requireDB(); err != nil {
				return nil, err
			}
			var args struct {
				IDs    []int  `json:"ids"`
				Reason string `json:"reason"`
			}
			json.Unmarshal(input, &args)
			if len(args.IDs) == 0 {
				return nil, fmt.Errorf("ids array is empty")
			}

			result, err := a.pool.Exec(ctx, `
				UPDATE users_card_bank
				SET add_in_progress = false, updated_at = NOW()
				WHERE id = ANY($1) AND add_in_progress = true
			`, args.IDs)
			if err != nil {
				return nil, fmt.Errorf("unlock failed: %w", err)
			}
			return json.Marshal(map[string]any{
				"unlocked": result.RowsAffected(),
				"ids":      args.IDs,
				"reason":   args.Reason,
			})
		},
	}
}
