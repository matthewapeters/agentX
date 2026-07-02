package http

import (
	"net/http"

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
