package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"nexus/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ─── Users ────────────────────────────────────────────────────────────────────

func (s *Store) CreateUser(ctx context.Context, u *domain.User) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, name, role)
		 VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Email, u.PasswordHash, u.Name, u.Role,
	)
	return err
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	u := &domain.User{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, name, role, created_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	u := &domain.User{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, name, role, created_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// ─── Projects ─────────────────────────────────────────────────────────────────

func (s *Store) ListProjects(ctx context.Context) ([]*domain.Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, slug, COALESCE(description,''), adapter_id, created_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []*domain.Project
	for rows.Next() {
		p := &domain.Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.AdapterID, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func (s *Store) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	p := &domain.Project{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, COALESCE(description,''), adapter_id, created_at FROM projects WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.AdapterID, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// UpsertProject inserts the project, or updates it if the slug exists. It
// returns the canonical project ID — the existing ID on conflict, the new one
// on insert — so callers always reference a row that actually exists.
func (s *Store) UpsertProject(ctx context.Context, p *domain.Project) (string, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO projects (id, name, slug, description, adapter_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (slug) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description
		 RETURNING id`,
		p.ID, p.Name, p.Slug, p.Description, p.AdapterID,
	).Scan(&p.ID)
	return p.ID, err
}

// ─── Agents ───────────────────────────────────────────────────────────────────

func (s *Store) ListAgents(ctx context.Context, projectID string) ([]*domain.Agent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, name, COALESCE(description,''), system_prompt,
		        default_mode, default_provider, default_model, allowed_tools, max_steps, created_at
		 FROM agents WHERE project_id = $1 ORDER BY name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []*domain.Agent
	for rows.Next() {
		a := &domain.Agent{}
		var toolsJSON []byte
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Description, &a.SystemPrompt,
			&a.DefaultMode, &a.DefaultProvider, &a.DefaultModel, &toolsJSON, &a.MaxSteps, &a.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(toolsJSON, &a.AllowedTools)
		agents = append(agents, a)
	}
	return agents, nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (*domain.Agent, error) {
	a := &domain.Agent{}
	var toolsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, COALESCE(description,''), system_prompt,
		        default_mode, default_provider, default_model, allowed_tools, max_steps, created_at
		 FROM agents WHERE id = $1`, id,
	).Scan(&a.ID, &a.ProjectID, &a.Name, &a.Description, &a.SystemPrompt,
		&a.DefaultMode, &a.DefaultProvider, &a.DefaultModel, &toolsJSON, &a.MaxSteps, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(toolsJSON, &a.AllowedTools)
	return a, nil
}

func (s *Store) UpsertAgent(ctx context.Context, a *domain.Agent) error {
	toolsJSON, _ := json.Marshal(a.AllowedTools)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agents (id, project_id, name, description, system_prompt, default_mode,
		                     default_provider, default_model, allowed_tools, max_steps)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (id) DO UPDATE SET
		   name=EXCLUDED.name, system_prompt=EXCLUDED.system_prompt,
		   default_provider=EXCLUDED.default_provider, default_model=EXCLUDED.default_model,
		   allowed_tools=EXCLUDED.allowed_tools`,
		a.ID, a.ProjectID, a.Name, a.Description, a.SystemPrompt, a.DefaultMode,
		a.DefaultProvider, a.DefaultModel, toolsJSON, a.MaxSteps,
	)
	return err
}

// UpsertAgentByName updates an existing agent for the project with the same
// name, or inserts a new one. Used by the seed so re-running it never creates
// duplicate agents.
func (s *Store) UpsertAgentByName(ctx context.Context, a *domain.Agent) error {
	var existingID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM agents WHERE project_id = $1 AND name = $2`,
		a.ProjectID, a.Name,
	).Scan(&existingID)
	if err == nil {
		a.ID = existingID
		return s.UpsertAgent(ctx, a)
	}
	if err != pgx.ErrNoRows {
		return err
	}
	return s.UpsertAgent(ctx, a)
}

// ─── Runs ─────────────────────────────────────────────────────────────────────

func (s *Store) CreateRun(ctx context.Context, r *domain.Run) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO runs (id, project_id, agent_id, user_id, title, status, mode, provider, model)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		r.ID, r.ProjectID, r.AgentID, r.UserID, r.Title, r.Status, r.Mode, r.Provider, r.Model,
	)
	return err
}

func (s *Store) UpdateRun(ctx context.Context, r *domain.Run) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE runs SET status=$2, step_count=$3, tokens_in=$4, tokens_out=$5,
		                 provider=$6, model=$7, updated_at=NOW()
		 WHERE id=$1`,
		r.ID, r.Status, r.StepCount, r.TokensIn, r.TokensOut, r.Provider, r.Model,
	)
	return err
}

func (s *Store) GetRun(ctx context.Context, id string) (*domain.Run, error) {
	r := &domain.Run{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, agent_id, user_id, title, status, mode, provider, model,
		        step_count, tokens_in, tokens_out, created_at, updated_at
		 FROM runs WHERE id = $1`, id,
	).Scan(&r.ID, &r.ProjectID, &r.AgentID, &r.UserID, &r.Title, &r.Status, &r.Mode,
		&r.Provider, &r.Model, &r.StepCount, &r.TokensIn, &r.TokensOut, &r.CreatedAt, &r.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return r, err
}

func (s *Store) ListRuns(ctx context.Context, userID string, limit int) ([]*domain.Run, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, agent_id, user_id, title, status, mode, provider, model,
		        step_count, tokens_in, tokens_out, created_at, updated_at
		 FROM runs WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []*domain.Run
	for rows.Next() {
		r := &domain.Run{}
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.AgentID, &r.UserID, &r.Title, &r.Status, &r.Mode,
			&r.Provider, &r.Model, &r.StepCount, &r.TokensIn, &r.TokensOut, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

// ─── Messages ─────────────────────────────────────────────────────────────────

func (s *Store) SaveMessage(ctx context.Context, m *domain.Message) error {
	toolCallsJSON, _ := json.Marshal(m.ToolCalls)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO messages (id, run_id, role, content, tool_calls, tool_call_id, tool_name)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		m.ID, m.RunID, m.Role, m.Content, toolCallsJSON, m.ToolCallID, m.ToolName,
	)
	return err
}

func (s *Store) GetMessages(ctx context.Context, runID string) ([]*domain.Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, run_id, role, content, tool_calls, COALESCE(tool_call_id,''), COALESCE(tool_name,''), created_at
		 FROM messages WHERE run_id = $1 ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []*domain.Message
	for rows.Next() {
		m := &domain.Message{}
		var toolCallsJSON []byte
		if err := rows.Scan(&m.ID, &m.RunID, &m.Role, &m.Content, &toolCallsJSON,
			&m.ToolCallID, &m.ToolName, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.ToolCalls = toolCallsJSON
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// ─── Tool Call Records ────────────────────────────────────────────────────────

func (s *Store) SaveToolCall(ctx context.Context, tc *domain.ToolCallRecord) error {
	inputJSON, _ := json.Marshal(tc.Input)
	outputJSON, _ := json.Marshal(tc.Output)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO tool_call_records (id, run_id, message_id, tool_name, input, output, duration_ms, error)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		tc.ID, tc.RunID, tc.MessageID, tc.ToolName, inputJSON, outputJSON, tc.DurationMs, tc.Error,
	)
	return err
}

// ─── Action Proposals ─────────────────────────────────────────────────────────

func (s *Store) CreateProposal(ctx context.Context, p *domain.ActionProposal) error {
	paramsJSON, _ := json.Marshal(p.Params)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO action_proposals (id, run_id, project_id, tool_name, description, params, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		p.ID, p.RunID, p.ProjectID, p.ToolName, p.Description, paramsJSON, p.Status,
	)
	return err
}

func (s *Store) GetProposal(ctx context.Context, id string) (*domain.ActionProposal, error) {
	p := &domain.ActionProposal{}
	var paramsJSON, resultJSON []byte
	var actedBy *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, run_id, project_id, tool_name, description, params, status,
		        acted_by, acted_at, result, created_at
		 FROM action_proposals WHERE id = $1`, id,
	).Scan(&p.ID, &p.RunID, &p.ProjectID, &p.ToolName, &p.Description, &paramsJSON, &p.Status,
		&actedBy, &p.ActedAt, &resultJSON, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if actedBy != nil {
		p.ActedBy = *actedBy
	}
	p.Params = paramsJSON
	p.Result = resultJSON
	return p, err
}

func (s *Store) ListPendingProposals(ctx context.Context) ([]*domain.ActionProposal, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, run_id, project_id, tool_name, description, params, status, created_at
		 FROM action_proposals WHERE status = 'pending' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var proposals []*domain.ActionProposal
	for rows.Next() {
		p := &domain.ActionProposal{}
		var paramsJSON []byte
		if err := rows.Scan(&p.ID, &p.RunID, &p.ProjectID, &p.ToolName, &p.Description,
			&paramsJSON, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Params = paramsJSON
		proposals = append(proposals, p)
	}
	return proposals, nil
}

func (s *Store) UpdateProposal(ctx context.Context, id string, status domain.ProposalStatus, actedBy string, result any) error {
	resultJSON, _ := json.Marshal(result)
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE action_proposals SET status=$2, acted_by=$3, acted_at=$4, result=$5
		 WHERE id=$1`,
		id, status, actedBy, now, resultJSON,
	)
	return err
}

// ClaimProposal atomically transitions a pending proposal to approved. It
// returns true only for the caller that wins the claim, so two concurrent
// approvals can never both execute the write tool.
func (s *Store) ClaimProposal(ctx context.Context, id, actedBy string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE action_proposals SET status='approved', acted_by=$2, acted_at=NOW()
		 WHERE id=$1 AND status='pending'`,
		id, actedBy,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ─── Audit ────────────────────────────────────────────────────────────────────

func (s *Store) Audit(ctx context.Context, userID, action, resource, resourceID string, payload any) {
	payloadJSON, _ := json.Marshal(payload)
	s.pool.Exec(ctx,
		`INSERT INTO audit_logs (id, user_id, action, resource, resource_id, payload)
		 VALUES (gen_random_uuid()::text, $1, $2, $3, $4, $5)`,
		userID, action, resource, resourceID, payloadJSON,
	)
}

// ─── Dashboard queries ────────────────────────────────────────────────────────

type BankPendingStat struct {
	BankName      string `json:"bank_name"`
	PendingCount  int    `json:"pending_count"`
	FailedCount   int    `json:"failed_count"`
	StuckCount    int    `json:"stuck_count"`
	OldestPending *int   `json:"oldest_pending_minutes,omitempty"`
}

func (s *Store) BankHealthStats(ctx context.Context, cashbackPool *pgxpool.Pool) ([]BankPendingStat, error) {
	if cashbackPool == nil {
		return nil, fmt.Errorf("cashback DB not connected")
	}
	rows, err := cashbackPool.Query(ctx, `
		SELECT
			bank_name,
			COUNT(*) FILTER (WHERE status = 'pending' AND add_in_progress = false)  AS pending_count,
			COUNT(*) FILTER (WHERE status = 'failed')                                AS failed_count,
			COUNT(*) FILTER (WHERE add_in_progress = true
			                   AND updated_at < NOW() - INTERVAL '30 minutes')       AS stuck_count,
			EXTRACT(EPOCH FROM (NOW() - MIN(created_at)
			    FILTER (WHERE status = 'pending' AND add_in_progress = false)))::int / 60 AS oldest_pending_minutes
		FROM users_card_bank
		WHERE status IN ('pending','failed') OR add_in_progress = true
		GROUP BY bank_name
		ORDER BY pending_count DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("bank health query: %w", err)
	}
	defer rows.Close()
	var stats []BankPendingStat
	for rows.Next() {
		var stat BankPendingStat
		var oldest *int
		if err := rows.Scan(&stat.BankName, &stat.PendingCount, &stat.FailedCount,
			&stat.StuckCount, &oldest); err != nil {
			return nil, err
		}
		stat.OldestPending = oldest
		stats = append(stats, stat)
	}
	return stats, nil
}
