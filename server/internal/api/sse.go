package api

import (
	"encoding/json"
	"fmt"
	"sync"

	"nexus/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ─── SSE Sink ─────────────────────────────────────────────────────────────────

// Sink is a per-run fan-out broadcaster for SSE clients.
type Sink struct {
	mu          sync.RWMutex
	subscribers map[string]chan domain.StreamEvent
}

func newSink() *Sink {
	return &Sink{subscribers: make(map[string]chan domain.StreamEvent)}
}

func (s *Sink) Subscribe() chan domain.StreamEvent {
	ch := make(chan domain.StreamEvent, 32)
	id := uuid.NewString()
	s.mu.Lock()
	s.subscribers[id] = ch
	s.mu.Unlock()
	return ch
}

func (s *Sink) Unsubscribe(ch chan domain.StreamEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.subscribers {
		if c == ch {
			delete(s.subscribers, id)
			close(ch)
			return
		}
	}
}

func (s *Sink) Send(event domain.StreamEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			// drop if slow consumer
		}
	}
}

// ─── Global sink registry ─────────────────────────────────────────────────────

var (
	sinkMu   sync.Mutex
	sinkMap  = make(map[string]*Sink)
)

func getOrCreateSink(runID string) *Sink {
	sinkMu.Lock()
	defer sinkMu.Unlock()
	if s, ok := sinkMap[runID]; ok {
		return s
	}
	s := newSink()
	sinkMap[runID] = s
	return s
}

// ─── SSE helpers ──────────────────────────────────────────────────────────────

func writeSSE(c *gin.Context, ev domain.StreamEvent) {
	data, _ := json.Marshal(ev)
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
}

// ─── Other helpers ────────────────────────────────────────────────────────────

func newID() string {
	return uuid.NewString()
}
