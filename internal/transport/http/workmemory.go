package http

import (
	"net/http"
	"strings"

	"agentx/internal/session"
	"agentx/internal/surfaces"
)

// wmSetBody is the POST /working-memory/set payload (add or edit a fact).
type wmSetBody struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// wmKeyBody is the POST /working-memory/delete payload.
type wmKeyBody struct {
	Key string `json:"key"`
}

// wmEnabledBody is the POST /working-memory/enabled payload.
type wmEnabledBody struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

// wmLiveBody is the POST /working-memory/live payload.
type wmLiveBody struct {
	Key  string `json:"key"`
	Live bool   `json:"live"`
}

// handleWorkingMemory returns the session's working-memory facts. It is a loopback
// read, so it is not token-gated (consistent with the other GET endpoints).
func (s *Server) handleWorkingMemory(w http.ResponseWriter, r *http.Request) {
	facts, err := s.prov.WorkingMemory()
	if err != nil {
		writeError(w, http.StatusInternalServerError, surfaces.CategoryValidation, err.Error())
		return
	}
	if facts == nil {
		facts = []session.Fact{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"facts": facts})
}

// handleWMSet adds or edits a fact (upsert). Mutations are token-gated.
func (s *Server) handleWMSet(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	var body wmSetBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Key) == "" {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, "key is required")
		return
	}
	if err := s.prov.SetFact(body.Key, body.Value); err != nil {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "set", "key": body.Key})
}

// handleWMDelete removes a fact by key. An unknown key is a no-op success.
func (s *Server) handleWMDelete(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	var body wmKeyBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Key) == "" {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, "key is required")
		return
	}
	if err := s.prov.DeleteFact(body.Key); err != nil {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "key": body.Key})
}

// handleWMEnabled enables or disables a fact, controlling whether it folds into the
// assembled context.
func (s *Server) handleWMEnabled(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	var body wmEnabledBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Key) == "" {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, "key is required")
		return
	}
	if err := s.prov.SetFactEnabled(body.Key, body.Enabled); err != nil {
		writeError(w, http.StatusNotFound, surfaces.CategoryValidation, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "key": body.Key, "enabled": body.Enabled})
}

// handleWMLive toggles whether a pinned fact re-runs its source tool before
// every turn (live) or stays a frozen snapshot (static) — the working-memory
// surface's play/pause affordance (PD-WM-AF-008). Refused on a fact with no
// tool Source.
func (s *Server) handleWMLive(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		writeError(w, http.StatusUnauthorized, surfaces.CategoryAuth, "invalid or missing attach token")
		return
	}
	var body wmLiveBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Key) == "" {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, "key is required")
		return
	}
	if err := s.prov.SetFactLive(body.Key, body.Live); err != nil {
		writeError(w, http.StatusBadRequest, surfaces.CategoryValidation, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "key": body.Key, "live": body.Live})
}
