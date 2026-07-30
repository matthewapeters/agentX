// Package runtimesteps adds Godog step definitions for the config live-reload
// and restart-queue domain (Phase 1e: PD-CONFIG-AF-007, AF-009).
package runtimesteps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/config"
	"agentx/internal/llm/provider"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/surfaces"
	transporthttp "agentx/internal/transport/http"
)

// liveReloadWorld carries per-scenario state for the live-reload / restart
// queue scenarios (Phase 1e).
type liveReloadWorld struct {
	server   *httptest.Server
	prov     *liveReloadProvider
	status   int
	body     []byte
	respJSON map[string]any
}

// InitializeLiveReload registers the live-reload steps.
func InitializeLiveReload(sc *godog.ScenarioContext) {
	w := &liveReloadWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w.server = nil
		w.prov = nil
		w.status = 0
		w.body = nil
		w.respJSON = nil
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.server != nil {
			w.server.Close()
		}
		w = &liveReloadWorld{}
		return ctx, nil
	})

	// --- Setup ---
	sc.Step(`^a running transport server for live-reload tests$`, w.setupServer)

	// --- POST /config: classify live vs restart keys ---
	sc.Step(`^the client POSTs "/config" with a live-reloadable change$`, w.postLiveReloadChange)
	sc.Step(`^the client POSTs "/config" with a restart-required change$`, w.postRestartChange)
	sc.Step(`^the client POSTs "/config" with both live and restart changes$`, w.postMixedChange)

	// --- Response assertions for live-reload ---
	sc.Step(`^the live-reload response lists "([^"]*)" as live-applied$`, w.liveAppliedKey)
	sc.Step(`^the live-reload response lists "([^"]*)" as restart-required$`, w.restartRequiredKey)
	sc.Step(`^the live-reload response has no live-applied keys$`, w.noLiveApplied)
	sc.Step(`^the live-reload response has no restart-required keys$`, w.noRestartRequired)
	sc.Step(`^the live-reload response status is "([^"]*)"`, w.liveReloadResponseStatus)

	// --- Restart status ---
	sc.Step(`^the client GETs "/config/restart/status"$`, w.getRestartStatus)
	sc.Step(`^the restart status response shows "([^"]*)" "([^"]*)"`, w.restartStatusShows)

	// --- Execute restart ---
	sc.Step(`^the client POSTs "/config/restart"$`, w.postRestart)
	sc.Step(`^the restart response status is "([^"]*)"`, w.restartResponseStatus)
	sc.Step(`^the restart response status is (\d+)$`, w.restartResponseHTTPStatus)

	// --- Restart queue after restart ---
	sc.Step(`^the restart queue is cleared after a successful restart$`, w.queueClearedAfterRestart)
}

func (w *liveReloadWorld) setupServer() error {
	w.prov = &liveReloadProvider{
		cfg: config.Default(),
	}
	w.server = httptest.NewServer(transporthttp.NewServer(w.prov).Handler())
	return nil
}

// --- POST /config helpers ---

func (w *liveReloadWorld) postLiveReloadChange() error {
	payload := map[string]any{
		"tools.timeout_seconds": float64(60),
	}
	return w.postConfig(payload)
}

func (w *liveReloadWorld) postRestartChange() error {
	payload := map[string]any{
		"provider":      "llamacpp",
		"llamacpp_host": "localhost:8080",
	}
	return w.postConfig(payload)
}

func (w *liveReloadWorld) postMixedChange() error {
	payload := map[string]any{
		"tools.timeout_seconds": float64(45),
		"provider":            "ollama",
	}
	return w.postConfig(payload)
}

func (w *liveReloadWorld) postConfig(payload map[string]any) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, w.server.URL+"/config", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	w.status = resp.StatusCode
	data, _ := io.ReadAll(resp.Body)
	w.body = data
	if w.status == http.StatusOK {
		_ = json.Unmarshal(data, &w.respJSON)
	}
	return nil
}

// --- Restart status ---

func (w *liveReloadWorld) getRestartStatus() error {
	resp, err := http.Get(w.server.URL + "/config/restart/status")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	w.status = resp.StatusCode
	data, _ := io.ReadAll(resp.Body)
	w.body = data
	if w.status == http.StatusOK {
		_ = json.Unmarshal(data, &w.respJSON)
	}
	return nil
}

// --- Execute restart ---

func (w *liveReloadWorld) postRestart() error {
	req, err := http.NewRequest(http.MethodPost, w.server.URL+"/config/restart", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	w.status = resp.StatusCode
	data, _ := io.ReadAll(resp.Body)
	w.body = data
	if w.status == http.StatusOK {
		_ = json.Unmarshal(data, &w.respJSON)
	}
	return nil
}

// --- Assertions ---

func (w *liveReloadWorld) liveAppliedKey(key string) error {
	if w.respJSON == nil {
		return fmt.Errorf("no response JSON")
	}
	la, ok := w.respJSON["live_applied"]
	if !ok {
		return fmt.Errorf("response missing live_applied")
	}
	arr, ok := la.([]any)
	if !ok {
		return fmt.Errorf("live_applied is not an array")
	}
	for _, v := range arr {
		if s, ok := v.(string); ok && s == key {
			return nil
		}
	}
	return fmt.Errorf("live_applied does not include %q", key)
}

func (w *liveReloadWorld) restartRequiredKey(key string) error {
	if w.respJSON == nil {
		return fmt.Errorf("no response JSON")
	}
	rr, ok := w.respJSON["restart_required"]
	if !ok {
		return fmt.Errorf("response missing restart_required")
	}
	arr, ok := rr.([]any)
	if !ok {
		return fmt.Errorf("restart_required is not an array")
	}
	for _, v := range arr {
		if s, ok := v.(string); ok && s == key {
			return nil
		}
	}
	return fmt.Errorf("restart_required does not include %q", key)
}

func (w *liveReloadWorld) noLiveApplied() error {
	if w.respJSON == nil {
		return fmt.Errorf("no response JSON")
	}
	la, ok := w.respJSON["live_applied"]
	if !ok {
		return fmt.Errorf("response missing live_applied")
	}
	arr, ok := la.([]any)
	if !ok {
		return fmt.Errorf("live_applied is not an array")
	}
	if len(arr) != 0 {
		return fmt.Errorf("live_applied has %d keys, want 0", len(arr))
	}
	return nil
}

func (w *liveReloadWorld) noRestartRequired() error {
	if w.respJSON == nil {
		return fmt.Errorf("no response JSON")
	}
	rr, ok := w.respJSON["restart_required"]
	if !ok {
		return fmt.Errorf("response missing restart_required")
	}
	arr, ok := rr.([]any)
	if !ok {
		return fmt.Errorf("restart_required is not an array")
	}
	if len(arr) != 0 {
		return fmt.Errorf("restart_required has %d keys, want 0", len(arr))
	}
	return nil
}

func (w *liveReloadWorld) liveReloadResponseStatus(want string) error {
	if w.respJSON == nil {
		return fmt.Errorf("no response JSON")
	}
	v, ok := w.respJSON["status"]
	if !ok {
		return fmt.Errorf("response missing status")
	}
	s, ok := v.(string)
	if !ok || s != want {
		return fmt.Errorf("response status = %v, want %q", v, want)
	}
	return nil
}

func (w *liveReloadWorld) restartStatusShows(key, expected string) error {
	if w.respJSON == nil {
		return fmt.Errorf("no response JSON")
	}
	v, ok := w.respJSON[key]
	if !ok {
		return fmt.Errorf("response missing key %q", key)
	}
	switch val := v.(type) {
	case bool:
		expectedBool := expected == "true"
		if val != expectedBool {
			return fmt.Errorf("response key %q = %v, want %v", key, val, expectedBool)
		}
	case string:
		if val != expected {
			return fmt.Errorf("response key %q = %q, want %q", key, val, expected)
		}
	default:
		return fmt.Errorf("response key %q = %v (unexpected type %T)", key, val, val)
	}
	return nil
}

func (w *liveReloadWorld) restartResponseStatus(want string) error {
	if w.respJSON == nil {
		return fmt.Errorf("no response JSON")
	}
	v, ok := w.respJSON["status"]
	if !ok {
		return fmt.Errorf("response missing status")
	}
	s, ok := v.(string)
	if !ok || s != want {
		return fmt.Errorf("response status = %v, want %q", v, want)
	}
	return nil
}

func (w *liveReloadWorld) restartResponseHTTPStatus(want int) error {
	if w.status == 0 {
		return fmt.Errorf("no HTTP status recorded")
	}
	if w.status != want {
		return fmt.Errorf("HTTP status = %d, want %d (body: %s)", w.status, want, string(w.body))
	}
	return nil
}

func (w *liveReloadWorld) queueClearedAfterRestart() error {
	// After restart, the queue should be empty. We check via the provider.
	if w.prov == nil {
		return fmt.Errorf("no provider")
	}
	keys := w.prov.QueuedRestartKeys()
	if keys != nil && len(keys) > 0 {
		return fmt.Errorf("restart queue has %d keys after restart, want 0", len(keys))
	}
	return nil
}

// --- liveReloadProvider satisfies transporthttp.Provider for Phase 1e tests. ---

// liveReloadProvider is a minimal transporthttp.Provider stand-in that implements
// the live-reload and restart flow. The in-memory config is mutated by SetConfig,
// the restart queue is held in memory, and ExecuteRestart clears the queue.
type liveReloadProvider struct {
	cfg          config.Config
	restartQueue map[string]any
}

func (p *liveReloadProvider) Bus() *state.Bus                        { return nil }
func (p *liveReloadProvider) Processing() *state.ProcessingPublisher { return nil }
func (p *liveReloadProvider) Session() session.Identity              { return session.Identity{} }
func (p *liveReloadProvider) Registry() *surfaces.Registry           { return nil }
func (p *liveReloadProvider) Submit(_ context.Context, text string) error   { return nil }
func (p *liveReloadProvider) Resolve(decision string)                 {}
func (p *liveReloadProvider) Accepting() bool                       { return true }
func (p *liveReloadProvider) History() ([]state.Event, error)       { return nil, nil }
func (p *liveReloadProvider) WorkingMemory() ([]session.Fact, error) { return nil, nil }
func (p *liveReloadProvider) SetFact(key, value string) error      { return nil }
func (p *liveReloadProvider) DeleteFact(key string) error          { return nil }
func (p *liveReloadProvider) SetFactEnabled(key string, enabled bool) error { return nil }
func (p *liveReloadProvider) SetFactLive(key string, live bool) error     { return nil }
func (p *liveReloadProvider) ContextBreakdown() (session.ContextReport, error) { return session.ContextReport{}, nil }
func (p *liveReloadProvider) SetEventEnabled(ordinal uint64, enabled bool) error { return nil }
func (p *liveReloadProvider) PinToolEvent(ordinal uint64, live bool) (string, error) { return "", nil }
func (p *liveReloadProvider) PinPlanNode(root, nodeID string) (string, error) { return "", nil }

func (p *liveReloadProvider) Config() map[string]any {
	out := map[string]any{
		"provider":             p.cfg.Provider(),
		"ollama_host":          p.cfg.OllamaHost(),
		"ollama_model":         p.cfg.OllamaModel(),
		"llamacpp_host":        p.cfg.LlamacppHost(),
		"llamacpp_model":       p.cfg.LlamacppModel(),
		"tools.timeout_seconds": p.cfg.Agentx.Tools.TimeoutSeconds,
		"tools.output_max_bytes": p.cfg.Agentx.Tools.OutputMaxBytes,
		"tools.absolute_max_bytes": p.cfg.Agentx.Tools.AbsoluteMaxBytes,
		"tools.enabled":          p.cfg.ToolsEnabled(),
		"tools.read_only":        p.cfg.ToolReadOnly(),
	}
	return out
}

func (p *liveReloadProvider) ConfigSchema() map[string]provider.SchemaField {
	return map[string]provider.SchemaField{
		"provider": {
			Name:            "Provider",
			Type:            "enum",
			Default:         "ollama",
			Required:        true,
			ReadOnly:        false,
			Description:     "The LLM backend to use: 'ollama' or 'llamacpp'.",
			EnumValues:      []string{"ollama", "llamacpp"},
			RestartRequired: true,
		},
		"ollama_host": {
			Name:            "Ollama Host",
			Type:            "host",
			Default:         "localhost:11434",
			Required:        true,
			ReadOnly:        false,
			Description:     "The Ollama host address.",
			RestartRequired: true,
		},
		"tools.timeout_seconds": {
			Name:            "Tool Timeout",
			Type:            "int",
			Default:         "30",
			Required:        false,
			ReadOnly:        false,
			Description:     "Tool execution timeout in seconds.",
			RestartRequired: false,
		},
	}
}

func (p *liveReloadProvider) ListModels() ([]string, error) {
	return []string{"phi4-mini:3.8b", "llama3.1:8b"}, nil
}

func (p *liveReloadProvider) TestHost(provider, host string) error {
	if strings.Contains(host, "99999") {
		return fmt.Errorf("connection refused")
	}
	return nil
}

func (p *liveReloadProvider) SetConfig(payload map[string]any) (*transporthttp.ConfigWriteResult, error) {
	// Decode payload into config.
	var cfg config.Config
	if err := decodeConfigPayloadTransport(payload, &cfg); err != nil {
		return &transporthttp.ConfigWriteResult{
			Status: "error",
			Errors: []string{"invalid config payload: " + err.Error()},
		}, nil
	}

	// Classify keys into live vs restart (Phase 1e).
	var restartKeys, liveKeys []string
	for key := range payload {
		switch key {
		case "provider", "ollama_host", "ollama_model",
			"llamacpp_host", "llamacpp_model",
			"transport.enabled", "transport.host",
			"transport.port_start", "transport.port_end":
			restartKeys = append(restartKeys, key)
		default:
			liveKeys = append(liveKeys, key)
		}
	}

	// Queue restart-required keys.
	if len(restartKeys) > 0 {
		p.restartQueue = payload
	}

	// Apply live keys to the in-memory config.
	p.cfg = cfg

	normalizedKeys := make([]transporthttp.NormalizedKey, 0)
	return &transporthttp.ConfigWriteResult{
		Status:          "applied",
		LiveApplied:     liveKeys,
		RestartRequired: restartKeys,
		NormalizedKeys:  normalizedKeys,
	}, nil
}

func (p *liveReloadProvider) QueuedRestartKeys() []string {
	if p.restartQueue == nil {
		return nil
	}
	keys := make([]string, 0)
	for k := range p.restartQueue {
		keys = append(keys, k)
	}
	return keys
}

func (p *liveReloadProvider) HasQueuedRestart() bool {
	return p.restartQueue != nil
}

func (p *liveReloadProvider) ExecuteRestart() error {
	if p.restartQueue == nil {
		return fmt.Errorf("no restart-queued config changes")
	}
	// Apply queued config and clear queue.
	var cfg config.Config
	if err := decodeConfigPayloadTransport(p.restartQueue, &cfg); err == nil {
		p.cfg = cfg
	}
	p.restartQueue = nil
	return nil
}

// intFromAny extracts an int from a JSON number (float64) or string.
func intFromAny(v any) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case string:
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err != nil {
			return 0, err
		}
		return n, nil
	default:
		return 0, fmt.Errorf("not an int: %T", v)
	}
}

// boolFromAny extracts a bool from a JSON bool, string "true"/"false", or numeric 0/1.
func boolFromAny(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		s := strings.TrimSpace(strings.ToLower(val))
		switch s {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("not a bool: %q", val)
		}
	case float64:
		switch int(val) {
		case 0:
			return false, nil
		case 1:
			return true, nil
		default:
			return false, fmt.Errorf("not a bool: %v", val)
		}
	default:
		return false, fmt.Errorf("not a bool: %T", v)
	}
}

// decodeConfigPayloadTransport decodes a flat JSON map into a typed config.Config
// for use by the live-reload test provider.
func decodeConfigPayloadTransport(payload map[string]any, cfg *config.Config) error {
	for k, v := range payload {
		s, ok := v.(string)
		if !ok {
			continue
		}
		switch k {
		case "provider", "chat_backend":
			cfg.Agentx.Provider = s
			cfg.Agentx.ChatBackend = s
		case "ollama_host":
			cfg.Agentx.Ollama.Host = s
		case "ollama_model":
			cfg.Agentx.Ollama.Model = s
		case "llamacpp_host":
			cfg.Agentx.Llamacpp.Host = s
		case "llamacpp_model":
			cfg.Agentx.Llamacpp.Model = s
		case "tools.timeout_seconds":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Tools.TimeoutSeconds = n
			}
		case "tools.output_max_bytes":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Tools.OutputMaxBytes = n
			}
		case "tools.absolute_max_bytes":
			if n, err := intFromAny(v); err == nil {
				cfg.Agentx.Tools.AbsoluteMaxBytes = n
			}
		case "tools.enabled":
			if b, err := boolFromAny(v); err == nil {
				cfg.Agentx.Tools.Enabled = &b
			}
		case "tools.read_only":
			if b, err := boolFromAny(v); err == nil {
				cfg.Agentx.Tools.ReadOnly = &b
			}
		}
	}
	return nil
}
