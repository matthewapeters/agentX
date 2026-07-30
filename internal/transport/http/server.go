// Package http is the loopback HTTP/SSE transport that lets external surface
// processes attach to a running orchestrator session (TRN-2+). It is a thin
// adapter over the canonical state the orchestrator already publishes: read
// endpoints return snapshots and the SSE endpoint streams the event bus. The
// server owns no orchestration logic.
//
// Import direction: this package must not import internal/runtime. It depends on
// the local Provider interface, which *runtime.Orchestrator satisfies and
// internal/app injects (docs/implementation/08_go_module_layout.md).
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"agentx/internal/llm/provider"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/surfaces"
)

// ConfigWriteResult reports the outcome of a POST /config write, separating
// live-applied keys from those requiring restart and any validation errors.
type ConfigWriteResult struct {
	Status          string          `json:"status"`            // "applied", "error"
	LiveApplied     []string        `json:"live_applied"`      // keys applied immediately
	RestartRequired []string        `json:"restart_required"`  // keys requiring restart
	Errors          []string        `json:"errors"`            // validation errors
	NormalizedKeys  []NormalizedKey `json:"normalized_keys"`   // keys that were normalized
	Write           *WriteMetadata  `json:"write,omitempty"`   // write metadata for debugging
}

// NormalizedKey records a single key that was normalized from a deprecated form
// to its canonical name.
type NormalizedKey struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// WriteMetadata captures write-time details for debugging (path, lock, temp).
type WriteMetadata struct {
	Path      string `json:"path"`
	Semaphore string `json:"semaphore"`
	TempFile  string `json:"temp_file"`
}

// Provider is the orchestrator surface the transport adapts. The concrete
// *runtime.Orchestrator satisfies it.
type Provider interface {
	Bus() *state.Bus
	Processing() *state.ProcessingPublisher
	Session() session.Identity
	Registry() *surfaces.Registry

	// Write surface (TRN-3).
	Submit(ctx context.Context, text string) error
	Resolve(decision string)
	Accepting() bool

	// History returns the persisted session event log for seeding a surface (SS-1).
	History() ([]state.Event, error)

	// Working memory (surface CRUD). Mutations persist and take effect on the next
	// prompt's assembled context.
	WorkingMemory() ([]session.Fact, error)
	SetFact(key, value string) error
	DeleteFact(key string) error
	SetFactEnabled(key string, enabled bool) error
	// SetFactLive toggles a pinned fact's live/static state (PD-WM "play/pause"),
	// refusing on a fact with no tool Source.
	SetFactLive(key string, live bool) error

	// ContextBreakdown reports the assembled context window's composition by
	// content class for the read-only context-visualizer surface (SS-7).
	ContextBreakdown() (session.ContextReport, error)

	// SetEventEnabled toggles whether a conversation element (by ordinal) folds
	// into the agent's upcoming context — the context surface's management path.
	SetEventEnabled(ordinal uint64, enabled bool) error
	// PinToolEvent copies a tool_result conversation element (by ordinal) into
	// working memory as a durable, pin-owned fact and disables the source event's
	// context-surface participation (PD-CTX-AF-012 / PD-WM). It returns the new
	// fact's key.
	PinToolEvent(ordinal uint64, live bool) (string, error)
	// PinPlanNode copies a plan node's own resolved Value (no backing tool call —
	// a Step, e.g. a wavefront Know) into working memory as a durable, pin-owned
	// fact (ADR 0012 amendment). It returns the new fact's key.
	PinPlanNode(root, nodeID string) (string, error)

	// Config returns the current effective configuration for transport.
	Config() map[string]any
	// ConfigSchema returns the configuration schema for transport.
	ConfigSchema() map[string]provider.SchemaField
	// ListModels returns the list of models hosted on the active provider.
	ListModels() ([]string, error)
	// TestHost tests a host endpoint for the named provider and returns whether
	// it's reachable. The provider name ("ollama" or "llamacpp") selects the
	// probe endpoint (/api/tags for Ollama, /v1/models for llama.cpp).
	TestHost(provider, host string) error
	// SetConfig accepts a full config JSON payload, validates it, applies
	// live-reloadable keys immediately, writes to disk atomically, and queues
	// restart-required keys. Returns the write result with status, applied keys,
	// restart-required keys, errors, and normalized keys.
	SetConfig(payload map[string]any) (*ConfigWriteResult, error)
	// QueuedRestartKeys reports config keys pending a restart (Phase 1e).
	QueuedRestartKeys() []string
	// HasQueuedRestart reports whether a restart is pending (Phase 1e).
	HasQueuedRestart() bool
	// ExecuteRestart triggers the orchestrator to shut down and restart with
	// the queued config changes (Phase 1e, PD-CONFIG-AF-009). Returns an error
	// if no changes are queued or restart fails.
	ExecuteRestart() error
}

// Server is the loopback HTTP/SSE transport for external surfaces.
type Server struct {
	prov     Provider
	mux      *http.ServeMux
	srv      *http.Server
	done     chan struct{}
	doneOnce sync.Once
}

// NewServer returns a transport server adapting prov. Call Handler for in-process
// testing, or Serve to bind a listener.
func NewServer(prov Provider) *Server {
	s := &Server{prov: prov, mux: http.NewServeMux(), done: make(chan struct{})}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /processing-state", s.handleProcessingState)
	s.mux.HandleFunc("GET /surfaces", s.handleSurfaces)
	s.mux.HandleFunc("GET /sessions/current", s.handleCurrentSession)
	s.mux.HandleFunc("GET /sessions/current/events", s.handleSessionEvents)
	s.mux.HandleFunc("GET /events", s.handleEvents)

	s.mux.HandleFunc("POST /surface/register", s.handleRegister)
	s.mux.HandleFunc("POST /prompt", s.handlePrompt)
	s.mux.HandleFunc("POST /tool/approval", s.handleApproval)
	s.mux.HandleFunc("POST /surface/{id}/shutdown", s.handleShutdown)
	s.mux.HandleFunc("POST /surface/{id}/command", s.handleCommand)
	s.mux.HandleFunc("POST /model/switch", s.handleModelSwitch)

	// Working-memory CRUD (surface read-write).
	s.mux.HandleFunc("GET /working-memory", s.handleWorkingMemory)
	s.mux.HandleFunc("POST /working-memory/set", s.handleWMSet)
	s.mux.HandleFunc("POST /working-memory/delete", s.handleWMDelete)
	s.mux.HandleFunc("POST /working-memory/enabled", s.handleWMEnabled)
	s.mux.HandleFunc("POST /working-memory/live", s.handleWMLive)

	// Context composition (read-only visualizer, SS-7).
	s.mux.HandleFunc("GET /context", s.handleContext)

	// Enable/disable a conversation element's context participation (context surface).
	s.mux.HandleFunc("POST /events/{ordinal}/enabled", s.handleEventEnabled)
	// Pin a tool_result element into working memory (PD-CTX-AF-012 / PD-WM).
	s.mux.HandleFunc("POST /events/{ordinal}/pin", s.handleEventPin)
	// Pin a plan node's own resolved value into working memory (ADR 0012 amendment
	// — a Step/Know has no tool_result event to pin via the route above).
	s.mux.HandleFunc("POST /plans/{root}/nodes/{node}/pin", s.handlePlanNodePin)

	// Config surface (Phase 1a read-only + Phase 1d write endpoints + Phase 1e restart).
	s.mux.HandleFunc("GET /config", s.handleConfig)
	s.mux.HandleFunc("POST /config", s.handlePostConfig)
	s.mux.HandleFunc("GET /config/schema", s.handleConfigSchema)
	s.mux.HandleFunc("GET /provider/{name}/models", s.handleProviderModels)
	s.mux.HandleFunc("POST /test/host", s.handleTestHost)
	// Phase 1e: restart status and execution.
	s.mux.HandleFunc("GET /config/restart/status", s.handleConfigRestartStatus)
	s.mux.HandleFunc("POST /config/restart", s.handleConfigRestart)
}

// Handler exposes the routes for in-process testing (httptest).
func (s *Server) Handler() http.Handler { return s.mux }

// Serve runs the HTTP server on ln until the listener closes or Shutdown is
// called. It returns http.ErrServerClosed on a clean shutdown.
func (s *Server) Serve(ln net.Listener) error {
	s.srv = &http.Server{Handler: s.mux}
	return s.srv.Serve(ln)
}

// Shutdown gracefully stops the server. It first signals open SSE handlers to
// return (so they do not block the graceful drain), then shuts the server down.
// It is a no-op before Serve.
func (s *Server) Shutdown(ctx context.Context) error {
	s.doneOnce.Do(func() { close(s.done) })
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "ok",
		"session_id": s.prov.Session().ID,
	})
}

func (s *Server) handleProcessingState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.prov.Processing().Current())
}

func (s *Server) handleSurfaces(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.prov.Registry().List())
}

func (s *Server) handleCurrentSession(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.prov.Session())
}

// handleSessionEvents returns the persisted session event log — the durable seed
// an attaching surface renders before resuming the live stream (SS-1).
func (s *Server) handleSessionEvents(w http.ResponseWriter, _ *http.Request) {
	hist, err := s.prov.History()
	if err != nil {
		writeError(w, http.StatusInternalServerError, surfaces.CategoryValidation, "read session history")
		return
	}
	if hist == nil {
		hist = []state.Event{}
	}
	writeJSON(w, http.StatusOK, hist)
}

// seedBackfillTimeout bounds the wait for the recorder to persist events up to the
// subscribe-time boundary, so the disk seed and the live stream meet with no gap.
const seedBackfillTimeout = 2 * time.Second

// handleEvents streams events after the `after` cursor as SSE. It captures a
// boundary ordinal at subscribe time, serves (after, boundary] from the durable
// history and (boundary, ∞) from the live bus — so the seed→live handover has no
// gap and no duplicate, with no client-side de-dup. Each connection is an
// independent bus subscriber, so a slow consumer never blocks others.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	after := parseAfter(r)

	// Track stream liveness for connection status (SS-4): mark the surface live for
	// the duration of this stream. The defer fires on clean quit, ctx cancel, and
	// crash alike (the connection drops), so presence never goes stale.
	if surfaceID := r.URL.Query().Get("surface_id"); surfaceID != "" {
		s.prov.Registry().MarkLive(surfaceID)
		defer s.prov.Registry().MarkDead(surfaceID)
	}

	// Subscribe before flushing headers so that once the client's request returns,
	// the subscription is registered; capture the boundary immediately after, so
	// every ordinal <= boundary is served from history and every ordinal > boundary
	// from this subscription.
	sub := s.prov.Bus().Subscribe()
	defer sub.Close()
	boundary := s.prov.Bus().CurrentOrdinal()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()

	// Backfill (after, boundary] from the durable log, waiting briefly for the
	// recorder to persist up to the boundary so the handover has no gap.
	for _, ev := range s.historyThrough(ctx, boundary) {
		if ev.Ordinal <= after || ev.Ordinal > boundary {
			continue
		}
		writeEvent(w, flusher, ev)
	}

	// Live tail: everything past the boundary.
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			if ev.Ordinal <= boundary {
				continue
			}
			// agent_delta is a transient chunk for the in-process chat window's
			// live typing effect; external surfaces receive only the complete
			// agent_response, so they never see the streaming deltas.
			if ev.ContentType == state.ContentAgentDelta {
				continue
			}
			writeEvent(w, flusher, ev)
		}
	}
}

// historyThrough returns the persisted log, polling until it covers boundary (the
// recorder may lag the bus by a few events) or the backfill deadline elapses.
func (s *Server) historyThrough(ctx context.Context, boundary uint64) []state.Event {
	deadline := time.Now().Add(seedBackfillTimeout)
	for {
		hist, err := s.prov.History()
		if err == nil && (boundary == 0 || maxOrdinal(hist) >= boundary || time.Now().After(deadline)) {
			return hist
		}
		select {
		case <-ctx.Done():
			return nil
		case <-s.done:
			return nil
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func maxOrdinal(events []state.Event) uint64 {
	var max uint64
	for _, ev := range events {
		if ev.Ordinal > max {
			max = ev.Ordinal
		}
	}
	return max
}

// writeEvent sends one SSE frame, skipping an event that cannot be encoded.
func writeEvent(w http.ResponseWriter, flusher http.Flusher, ev state.Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.ContentType, data)
	flusher.Flush()
}

// parseAfter reads the optional `after` cursor (0 = from the beginning).
func parseAfter(r *http.Request) uint64 {
	n, err := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// writeJSON encodes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// handleConfig returns the current effective configuration for transport.
func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := s.prov.Config()
	writeJSON(w, http.StatusOK, cfg)
}

// handleConfigSchema returns the configuration schema for transport.
func (s *Server) handleConfigSchema(w http.ResponseWriter, _ *http.Request) {
	schema := s.prov.ConfigSchema()
	writeJSON(w, http.StatusOK, schema)
}

// handleProviderModels returns the list of models hosted on the active provider.
func (s *Server) handleProviderModels(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "config", "missing provider name")
		return
	}
	models, err := s.prov.ListModels()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config", "list models: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

// handleTestHost tests a host endpoint for the named provider and returns
// whether it's reachable. The provider ("ollama" or "llamacpp") selects the
// probe endpoint.
func (s *Server) handleTestHost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Host     string `json:"host"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "config", "invalid request body")
		return
	}
	if req.Host == "" {
		writeError(w, http.StatusBadRequest, "config", "host is required")
		return
	}
	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "config", "provider is required")
		return
	}
	err := s.prov.TestHost(req.Provider, req.Host)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"reachable": false,
			"error":     err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reachable": true,
	})
}

// handleConfigRestartStatus reports whether a restart is pending and, if so,
// which keys are queued. Phase 1e (PD-CONFIG-AF-009).
func (s *Server) handleConfigRestartStatus(w http.ResponseWriter, _ *http.Request) {
	keys := s.prov.QueuedRestartKeys()
	if keys == nil {
		keys = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"has_queued_restart": s.prov.HasQueuedRestart(),
		"keys":             keys,
	})
}

// handleConfigRestart triggers the orchestrator to restart with the queued
// config changes (Phase 1e, PD-CONFIG-AF-009).
func (s *Server) handleConfigRestart(w http.ResponseWriter, r *http.Request) {
	err := s.prov.ExecuteRestart()
	if err != nil {
		writeError(w, http.StatusConflict, surfaces.CategoryConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "restarted",
	})
}

// handlePostConfig accepts a full config JSON payload, validates it, applies
// live-reloadable keys, writes atomically to disk, and returns the write result.
// Phase 1d write endpoint.
func (s *Server) handlePostConfig(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "config", "invalid request body")
		return
	}
	if payload == nil {
		writeError(w, http.StatusBadRequest, "config", "empty config payload")
		return
	}
	result, err := s.prov.SetConfig(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config", err.Error())
		return
	}
	if result.Status == "error" {
		writeJSON(w, http.StatusBadRequest, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
