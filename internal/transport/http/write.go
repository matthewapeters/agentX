package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"agentx/internal/surfaces"
)

// registerBody is the POST /surface/register payload.
type registerBody struct {
	SurfaceID        string   `json:"surface_id"`
	SurfaceKind      string   `json:"surface_kind"`
	TransportAddress string   `json:"transport_address"`
	Capabilities     []string `json:"capabilities"`
	Token            string   `json:"token"`
}

type promptBody struct {
	Text string `json:"text"`
}

type approvalBody struct {
	Decision string `json:"decision"`
}

// handleRegister attaches a surface. It authorizes via the token in the body.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body registerBody
	if !decodeJSON(w, r, &body) {
		return
	}
	reg, err := s.prov.Registry().Register(surfaces.RegisterRequest{
		SurfaceID:        body.SurfaceID,
		SurfaceKind:      body.SurfaceKind,
		TransportAddress: body.TransportAddress,
		Capabilities:     body.Capabilities,
		Token:            body.Token,
	})
	if err != nil {
		var re *surfaces.RegisterError
		if errors.As(err, &re) {
			writeError(w, statusForCategory(re.Category), re.Category, re.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, surfaces.CategoryValidation, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, reg)
}

// handlePrompt hands a prompt to the orchestrator and returns immediately; the
// cycle's events flow back over GET /events.
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	var body promptBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, "text is required")
		return
	}
	if !s.prov.Accepting() {
		writeError(w, http.StatusConflict, surfaces.CategoryConflict, "not accepting prompts")
		return
	}
	text := body.Text
	go func() { _ = s.prov.Submit(context.Background(), text) }()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// handleApproval forwards a tool-approval decision to the orchestrator gate.
func (s *Server) handleApproval(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	var body approvalBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Decision) == "" {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, "decision is required")
		return
	}
	s.prov.Resolve(body.Decision)
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

// handleVerbApproval forwards a verb-continuation decision to the orchestrator's
// verb-approval gate — the same shape as handleApproval, for the distinct
// stated-continuation-verb decision (see internal/runtime's verbApprovalGate).
func (s *Server) handleVerbApproval(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	var body approvalBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Decision) == "" {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, "decision is required")
		return
	}
	s.prov.ResolveVerb(body.Decision)
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

// handleShutdown transitions a registered surface to stopped.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	id := r.PathValue("id")
	if err := s.prov.Registry().Shutdown(id); err != nil {
		writeError(w, http.StatusNotFound, surfaces.CategoryValidation, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "surface_id": id})
}

// handleCommand is reserved: there is no inbound channel to an external surface
// in v1, so it validates registration then reports not-implemented.
func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	id := r.PathValue("id")
	if _, ok := s.prov.Registry().Get(id); !ok {
		writeError(w, http.StatusNotFound, surfaces.CategoryValidation, "unknown surface_id")
		return
	}
	writeNotImplemented(w, "surface command relay not implemented in v1")
}

// handleModelSwitch is deferred in v1.
func (s *Server) handleModelSwitch(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	writeNotImplemented(w, "live model switch not implemented in v1")
}

// authorize validates the Authorization: Bearer <attach-token> header.
func (s *Server) authorize(r *http.Request) bool {
	return s.prov.Registry().ValidateToken(bearerToken(r))
}

func bearerToken(r *http.Request) string {
	if t, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return t
	}
	return ""
}

// decodeJSON decodes the request body into v, writing a 400 and returning false
// on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, "invalid JSON body")
		return false
	}
	return true
}

// statusForCategory maps a registry reason category to an HTTP status.
func statusForCategory(category string) int {
	switch category {
	case surfaces.CategoryAuth:
		return http.StatusUnauthorized
	case surfaces.CategoryConflict:
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

func writeError(w http.ResponseWriter, status int, category, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "category": category})
}

func writeNotImplemented(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": msg, "status": "not_implemented"})
}
