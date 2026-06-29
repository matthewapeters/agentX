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
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/surfaces"
)

// Provider is the orchestrator surface the transport adapts. The concrete
// *runtime.Orchestrator satisfies it.
type Provider interface {
	Bus() *state.Bus
	Processing() *state.ProcessingPublisher
	Session() session.Identity
	Registry() *surfaces.Registry
}

// Server is the loopback HTTP/SSE transport for external surfaces.
type Server struct {
	prov Provider
	mux  *http.ServeMux
	srv  *http.Server
}

// NewServer returns a transport server adapting prov. Call Handler for in-process
// testing, or Serve to bind a listener.
func NewServer(prov Provider) *Server {
	s := &Server{prov: prov, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /processing-state", s.handleProcessingState)
	s.mux.HandleFunc("GET /surfaces", s.handleSurfaces)
	s.mux.HandleFunc("GET /sessions/current", s.handleCurrentSession)
	s.mux.HandleFunc("GET /events", s.handleEvents)
}

// Handler exposes the routes for in-process testing (httptest).
func (s *Server) Handler() http.Handler { return s.mux }

// Serve runs the HTTP server on ln until the listener closes or Shutdown is
// called. It returns http.ErrServerClosed on a clean shutdown.
func (s *Server) Serve(ln net.Listener) error {
	s.srv = &http.Server{Handler: s.mux}
	return s.srv.Serve(ln)
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

// handleEvents streams the event bus to one SSE client. Each connection is an
// independent bus subscriber, so a slow consumer never blocks the publisher or
// other surfaces.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Subscribe before flushing headers so that once the client's request
	// returns, the subscription is guaranteed registered and no event published
	// after that point can be missed.
	sub := s.prov.Bus().Subscribe()
	defer sub.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.ContentType, data)
			flusher.Flush()
		}
	}
}

// writeJSON encodes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
