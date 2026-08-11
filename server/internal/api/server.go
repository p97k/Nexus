package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"nexus/internal/agent"
	"nexus/internal/auth"
	"nexus/internal/config"
	"nexus/internal/db"
	"nexus/internal/domain"
	"nexus/internal/llm"
	"nexus/internal/tools"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	cfg          *config.Config
	store        *db.Store
	authSvc      *auth.Service
	llmRouter    *llm.Router
	toolReg      *tools.Registry
	runner       *agent.Runner
	cashbackPool *pgxpool.Pool
}

func NewServer(
	cfg *config.Config,
	store *db.Store,
	authSvc *auth.Service,
	llmRouter *llm.Router,
	toolReg *tools.Registry,
	cashbackPool *pgxpool.Pool,
	fallbackModels []string,
) *Server {
	return &Server{
		cfg:          cfg,
		store:        store,
		authSvc:      authSvc,
		llmRouter:    llmRouter,
		toolReg:      toolReg,
		runner:       agent.NewRunner(store, llmRouter, toolReg, fallbackModels),
		cashbackPool: cashbackPool,
	}
}

func (s *Server) Handler() http.Handler {
	if s.cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = s.cfg.CORS
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	r.Use(cors.New(corsConfig))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api")

	// Public
	api.POST("/auth/login", s.login)

	// Protected
	protected := api.Group("")
	protected.Use(auth.Middleware(s.authSvc))

	protected.GET("/auth/me", s.me)

	protected.GET("/providers", s.listProviders)

	protected.GET("/projects", s.listProjects)
	protected.GET("/projects/:id/agents", s.listAgents)
	protected.GET("/agents/:id", s.getAgent)

	protected.POST("/runs", s.createRun)
	protected.GET("/runs", s.listRuns)
	protected.GET("/runs/:id", s.getRun)
	protected.GET("/runs/:id/messages", s.getRunMessages)
	protected.GET("/runs/:id/stream", s.streamRun)
	protected.POST("/runs/:id/messages", s.sendMessage)

	protected.GET("/proposals", s.listProposals)
	protected.POST("/proposals/:id/approve", s.approveProposal)
	protected.POST("/proposals/:id/reject", s.rejectProposal)

	protected.GET("/dashboard/cashback", s.cashbackDashboard)

	return r
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func userID(c *gin.Context) string {
	return c.GetString(auth.ContextUserID)
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

func (s *Server) login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request")
		return
	}
	user, err := s.store.GetUserByEmail(c, req.Email)
	if err != nil || user == nil || !s.authSvc.CheckPassword(user.PasswordHash, req.Password) {
		fail(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := s.authSvc.IssueToken(user)
	if err != nil {
		fail(c, http.StatusInternalServerError, "token error")
		return
	}
	ok(c, gin.H{"token": token, "user": user})
}

func (s *Server) me(c *gin.Context) {
	user, err := s.store.GetUserByID(c, userID(c))
	if err != nil || user == nil {
		fail(c, http.StatusNotFound, "user not found")
		return
	}
	ok(c, user)
}

// ─── Providers ────────────────────────────────────────────────────────────────

func (s *Server) listProviders(c *gin.Context) {
	ok(c, s.llmRouter.Providers())
}

// ─── Projects ─────────────────────────────────────────────────────────────────

func (s *Server) listProjects(c *gin.Context) {
	projects, err := s.store.ListProjects(c)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, projects)
}

func (s *Server) listAgents(c *gin.Context) {
	agents, err := s.store.ListAgents(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, agents)
}

func (s *Server) getAgent(c *gin.Context) {
	a, err := s.store.GetAgent(c, c.Param("id"))
	if err != nil || a == nil {
		fail(c, http.StatusNotFound, "agent not found")
		return
	}
	ok(c, a)
}

// ─── Runs ─────────────────────────────────────────────────────────────────────

func (s *Server) createRun(c *gin.Context) {
	var req struct {
		AgentID  string `json:"agent_id" binding:"required"`
		Title    string `json:"title"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	a, err := s.store.GetAgent(c, req.AgentID)
	if err != nil || a == nil {
		fail(c, http.StatusNotFound, "agent not found")
		return
	}
	title := req.Title
	if title == "" {
		title = "New investigation"
	}
	provider := req.Provider
	if provider == "" {
		provider = a.DefaultProvider
	}
	model := req.Model
	if model == "" {
		model = a.DefaultModel
	}

	run := &domain.Run{
		ID:        newID(),
		ProjectID: a.ProjectID,
		AgentID:   a.ID,
		UserID:    userID(c),
		Title:     title,
		Status:    domain.RunStatusPending,
		Mode:      string(a.DefaultMode),
		Provider:  provider,
		Model:     model,
	}
	if err := s.store.CreateRun(c, run); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, run)
}

func (s *Server) listRuns(c *gin.Context) {
	runs, err := s.store.ListRuns(c, userID(c), 50)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, runs)
}

func (s *Server) getRun(c *gin.Context) {
	run, err := s.store.GetRun(c, c.Param("id"))
	if err != nil || run == nil {
		fail(c, http.StatusNotFound, "run not found")
		return
	}
	ok(c, run)
}

func (s *Server) getRunMessages(c *gin.Context) {
	msgs, err := s.store.GetMessages(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, msgs)
}

func (s *Server) sendMessage(c *gin.Context) {
	runID := c.Param("id")
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	run, err := s.store.GetRun(c, runID)
	if err != nil || run == nil {
		fail(c, http.StatusNotFound, "run not found")
		return
	}
	a, err := s.store.GetAgent(c, run.AgentID)
	if err != nil || a == nil {
		fail(c, http.StatusInternalServerError, "agent not found")
		return
	}

	// Execute asynchronously — the client should be listening on /stream.
	// We detach from the request context so cancellation doesn't abort the agent,
	// but bound the whole turn so a hung provider can't leave the run running forever.
	// NOTE: no `defer cancel()` here — this handler returns immediately, and
	// cancelling here would kill the context before the runner goroutine starts.
	execCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	go func() {
		defer cancel()
		s.runner.Execute(execCtx, agent.RunOptions{
			Run:     run,
			Agent:   a,
			UserMsg: req.Content,
			Sink:    getOrCreateSink(runID),
		})
	}()

	c.JSON(http.StatusAccepted, gin.H{"status": "running"})
}

// ─── SSE Streaming ────────────────────────────────────────────────────────────

func (s *Server) streamRun(c *gin.Context) {
	runID := c.Param("id")

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	sink := getOrCreateSink(runID)
	ch := sink.Subscribe()
	defer sink.Unsubscribe(ch)

	ctx := c.Request.Context()
	// Keep the connection alive during long model calls so idle-timeout
	// proxies (e.g. Next dev) don't kill the stream mid-run.
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(c.Writer, ": ping\n\n")
			c.Writer.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(c, ev)
			c.Writer.Flush()
			if ev.Type == domain.StreamEventDone || ev.Type == domain.StreamEventError {
				return
			}
		}
	}
}

// ─── Proposals ────────────────────────────────────────────────────────────────

func (s *Server) listProposals(c *gin.Context) {
	proposals, err := s.store.ListPendingProposals(c)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, proposals)
}

func (s *Server) approveProposal(c *gin.Context) {
	id := c.Param("id")

	// Atomically claim the proposal. Only the winning caller proceeds to
	// execute, so concurrent approvals cannot double-run the write tool.
	claimed, err := s.store.ClaimProposal(c, id, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if !claimed {
		fail(c, http.StatusConflict, "proposal already acted on")
		return
	}

	proposal, err := s.store.GetProposal(c, id)
	if err != nil || proposal == nil {
		fail(c, http.StatusNotFound, "proposal not found")
		return
	}

	// Execute the tool
	tool, exists := s.toolReg.Get(proposal.ToolName)
	if !exists {
		s.store.UpdateProposal(c, id, domain.ProposalStatusFailed, userID(c), map[string]string{"error": "tool not found"})
		fail(c, http.StatusUnprocessableEntity, "tool not found")
		return
	}
	result, toolErr := tool.Execute(c, proposal.Params)
	if toolErr != nil {
		s.store.UpdateProposal(c, id, domain.ProposalStatusFailed, userID(c), map[string]string{"error": toolErr.Error()})
		fail(c, http.StatusInternalServerError, toolErr.Error())
		return
	}
	s.store.UpdateProposal(c, id, domain.ProposalStatusExecuted, userID(c), result)
	s.store.Audit(c, userID(c), "approve_proposal", "action_proposals", id, map[string]any{"tool": proposal.ToolName})
	ok(c, gin.H{"status": "executed", "result": json.RawMessage(result)})
}

func (s *Server) rejectProposal(c *gin.Context) {
	id := c.Param("id")
	s.store.UpdateProposal(c, id, domain.ProposalStatusRejected, userID(c), nil)
	s.store.Audit(c, userID(c), "reject_proposal", "action_proposals", id, nil)
	ok(c, gin.H{"status": "rejected"})
}

// ─── Dashboard ────────────────────────────────────────────────────────────────

func (s *Server) cashbackDashboard(c *gin.Context) {
	stats, err := s.store.BankHealthStats(c, s.cashbackPool)
	if err != nil {
		fail(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	ok(c, stats)
}
