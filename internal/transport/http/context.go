package http

import (
	"net/http"
	"strconv"

	"agentx/internal/surfaces"
)

// handleContext returns the assembled context window's composition by content
// class for the read-only context-visualizer surface (SS-7). It is a loopback
// read, so it is not token-gated (consistent with the other GET endpoints).
func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	report, err := s.prov.ContextBreakdown()
	if err != nil {
		writeError(w, http.StatusInternalServerError, surfaces.CategoryValidation, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// eventEnabledBody is the POST /events/{ordinal}/enabled payload.
type eventEnabledBody struct {
	Enabled bool `json:"enabled"`
}

// handleEventEnabled toggles whether a conversation element folds into the agent's
// upcoming context (the context surface's management path). Mutations are token-gated.
func (s *Server) handleEventEnabled(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	ordinal, err := strconv.ParseUint(r.PathValue("ordinal"), 10, 64)
	if err != nil || ordinal == 0 {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, "invalid ordinal")
		return
	}
	var body eventEnabledBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.prov.SetEventEnabled(ordinal, body.Enabled); err != nil {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "set", "ordinal": ordinal, "enabled": body.Enabled})
}
