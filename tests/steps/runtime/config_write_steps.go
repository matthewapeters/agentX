// Package runtimesteps adds Godog step definitions for the config write
// domain (Phase 1d: POST /config and POST /test/host endpoints).
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

	"agentx/internal/llm/provider"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/surfaces"
	transporthttp "agentx/internal/transport/http"
)

// configWriteWorld carries per-scenario state for config-write scenarios.
type configWriteWorld struct {
	server   *httptest.Server
	prov     *configWriteProvider
	status   int
	body     []byte
	respJSON map[string]any
}

// configWriteProvider is a minimal transporthttp.Provider stand-in that only
// implements the config-write surface: SetConfig, Config, ConfigSchema,
// ListModels, TestHost. It validates payloads against the rules from Phase 1c.
type configWriteProvider struct {
	// writtenCfg holds the last successfully written config for assertions.
	writtenCfg map[string]any
}

// InitializeConfigWrite registers the config-write steps.
func InitializeConfigWrite(sc *godog.ScenarioContext) {
	w := &configWriteWorld{}

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
		*w = configWriteWorld{}
		return ctx, nil
	})

	// --- Setup ---
	sc.Step(`^a running transport server for config write tests$`, w.setupServer)

	// --- POST /config steps ---
	sc.Step(`^the client POSTs "/config" with payload$`, w.postConfigWithTable)
	sc.Step(`^the client POSTs "/config" with empty payload$`, w.postConfigEmpty)
	sc.Step(`^the client POSTs "/config" with malformed JSON$`, w.postConfigMalformed)

	// --- Response assertions (unique to config-write, prefixed "config write") ---
	sc.Step(`^the config write response status is (\d+)$`, w.responseStatus)
	sc.Step(`^the config write response field "([^"]*)" is "([^"]*)"$`, w.responseJSONField)
	sc.Step(`^the config write response field "([^"]*)" is true$`, w.responseJSONFieldTrue)
	sc.Step(`^the config write response field "([^"]*)" is false$`, w.responseJSONFieldFalse)
	sc.Step(`^the config write response includes key "([^"]*)"$`, w.responseIncludesKey)
	sc.Step(`^the restart-required keys include "([^"]*)"$`, w.restartKeyIncludes)
	sc.Step(`^the live-applied keys include "([^"]*)"$`, w.liveKeyIncludes)
	sc.Step(`^the config write normalized key "([^"]*)" → "([^"]*)"$`, w.normalizedKey)

	// --- POST /test/host steps (unique to config-write) ---
	sc.Step(`^the config write client tests host "([^"]*)" over the transport$`, w.testHost)
	sc.Step(`^the config write client tests host "([^"]*)" with provider "([^"]*)"$`, w.testHostWithProvider)
	sc.Step(`^the config write client tests host "([^"]*)" with empty provider$`, w.testHostEmptyProvider)
	sc.Step(`^the config write test response shows "([^"]*)" "([^"]*)"$`, w.testResponseShows)
	sc.Step(`^the config write test is rejected as "([^"]*)"$`, w.testRejected)
	sc.Step(`^the config write test returns status (\d+)$`, w.testReturnsStatus)
}

func (w *configWriteWorld) setupServer() error {
	prov := &configWriteProvider{}
	srv := transporthttp.NewServer(&configWriteFullProvider{configWriteProvider: prov})
	w.server = httptest.NewServer(srv.Handler())
	w.prov = prov
	return nil
}

// configWriteFullProvider satisfies transporthttp.Provider. Only the config-
// write methods carry real logic; the rest are no-ops for test purposes.
type configWriteFullProvider struct {
	*configWriteProvider
}

func (p *configWriteFullProvider) Config() map[string]any {
	if p.writtenCfg != nil {
		return p.writtenCfg
	}
	return map[string]any{
		"provider":      "ollama",
		"ollama_host":   "localhost:11434",
		"ollama_model":  "phi4-mini:3.8b",
		"llamacpp_host": "",
		"llamacpp_model": "",
	}
}

func (p *configWriteFullProvider) ConfigSchema() map[string]provider.SchemaField {
	return map[string]provider.SchemaField{
		"provider": {
			Name:            "Provider",
			Type:            "enum",
			Default:         "ollama",
			Required:        true,
			ReadOnly:        false,
			Description:     "The LLM backend to use.",
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
	}
}

func (p *configWriteFullProvider) ListModels() ([]string, error) {
	return []string{"phi4-mini:3.8b", "llama3.1:8b"}, nil
}

func (p *configWriteFullProvider) TestHost(provider, host string) error {
	if strings.Contains(host, "99999") {
		return fmt.Errorf("connection refused")
	}
	return nil
}

func (p *configWriteFullProvider) SetConfig(payload map[string]any) (*transporthttp.ConfigWriteResult, error) {
	return p.configWriteProvider.SetConfig(payload)
}

// Unused transporthttp.Provider methods — needed for interface conformance.
// The config-write tests never call these.

func (p *configWriteFullProvider) Bus() *state.Bus { return nil }
func (p *configWriteFullProvider) Processing() *state.ProcessingPublisher { return nil }
func (p *configWriteFullProvider) Session() session.Identity { return session.Identity{} }
func (p *configWriteFullProvider) Registry() *surfaces.Registry { return nil }
func (p *configWriteFullProvider) Submit(_ context.Context, text string) error { return nil }
func (p *configWriteFullProvider) Resolve(decision string)                  {}
func (p *configWriteFullProvider) Accepting() bool                         { return true }
func (p *configWriteFullProvider) History() ([]state.Event, error)          { return nil, nil }
func (p *configWriteFullProvider) WorkingMemory() ([]session.Fact, error)    { return nil, nil }
func (p *configWriteFullProvider) SetFact(key, value string) error         { return nil }
func (p *configWriteFullProvider) DeleteFact(key string) error             { return nil }
func (p *configWriteFullProvider) SetFactEnabled(key string, enabled bool) error { return nil }
func (p *configWriteFullProvider) SetFactLive(key string, live bool) error     { return nil }
func (p *configWriteFullProvider) ContextBreakdown() (session.ContextReport, error) { return session.ContextReport{}, nil }
func (p *configWriteFullProvider) SetEventEnabled(ordinal uint64, enabled bool) error { return nil }
func (p *configWriteFullProvider) PinToolEvent(ordinal uint64, live bool) (string, error) { return "", nil }
func (p *configWriteFullProvider) PinPlanNode(root, nodeID string) (string, error) { return "", nil }

// Phase 1e: restart stubs.
func (p *configWriteFullProvider) QueuedRestartKeys() []string { return nil }
func (p *configWriteFullProvider) HasQueuedRestart() bool      { return false }
func (p *configWriteFullProvider) ExecuteRestart() error       { return nil }

// --- POST /config with table ---

// postConfigWithTable sends POST /config with key-value pairs from the Godog
// table. Each row has a "key" column and a "value" column (literal JSON).
func (w *configWriteWorld) postConfigWithTable(table *godog.Table) error {
	payload := make(map[string]any)
	if table != nil {
		for _, row := range table.Rows[1:] {
			if len(row.Cells) >= 2 {
				key := row.Cells[0].Value
				val := row.Cells[1].Value
				// Parse the value as JSON (handles strings, ints, bools).
				var parsed any
				if err := json.Unmarshal([]byte(val), &parsed); err == nil {
					payload[key] = parsed
				} else {
					payload[key] = val
				}
			}
		}
	}
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

// --- POST /config with empty/malformed payload ---

func (w *configWriteWorld) postConfigEmpty() error {
	req, err := http.NewRequest(http.MethodPost, w.server.URL+"/config", nil)
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
	return nil
}

func (w *configWriteWorld) postConfigMalformed() error {
	req, err := http.NewRequest(http.MethodPost, w.server.URL+"/config", bytes.NewReader([]byte("{not valid json")))
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
	return nil
}

// --- POST /test/host ---

func (w *configWriteWorld) testHost(host string) error {
	return w.testHostWithProvider(host, "ollama")
}

func (w *configWriteWorld) testHostWithProvider(host, provider string) error {
	body, _ := json.Marshal(map[string]string{"host": host, "provider": provider})
	req, err := http.NewRequest(http.MethodPost, w.server.URL+"/test/host", bytes.NewReader(body))
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

func (w *configWriteWorld) testHostEmptyProvider(host string) error {
	body, _ := json.Marshal(map[string]string{"host": host, "provider": ""})
	req, err := http.NewRequest(http.MethodPost, w.server.URL+"/test/host", bytes.NewReader(body))
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
	return nil
}

// --- Response assertions ---

func (w *configWriteWorld) responseStatus(want int) error {
	if w.status != want {
		return fmt.Errorf("response status = %d, want %d (body: %s)", w.status, want, string(w.body))
	}
	return nil
}

func (w *configWriteWorld) responseJSONField(field, want string) error {
	if w.respJSON == nil {
		if err := json.Unmarshal(w.body, &w.respJSON); err != nil {
			return fmt.Errorf("body is not JSON: %s", string(w.body))
		}
	}
	v, ok := w.respJSON[field]
	if !ok {
		return fmt.Errorf("response missing field %q (body: %s)", field, string(w.body))
	}
	got, _ := json.Marshal(v)
	gotStr := strings.Trim(string(got), "\"")
	if gotStr != want {
		return fmt.Errorf("response field %q = %q, want %q", field, gotStr, want)
	}
	return nil
}

func (w *configWriteWorld) responseJSONFieldTrue(field string) error {
	return w.responseJSONFieldBool(field, true)
}

func (w *configWriteWorld) responseJSONFieldFalse(field string) error {
	return w.responseJSONFieldBool(field, false)
}

func (w *configWriteWorld) responseJSONFieldBool(field string, want bool) error {
	if w.respJSON == nil {
		_ = json.Unmarshal(w.body, &w.respJSON)
	}
	v, ok := w.respJSON[field]
	if !ok {
		return fmt.Errorf("response missing field %q", field)
	}
	got, ok := v.(bool)
	if !ok || got != want {
		return fmt.Errorf("response field %q = %v, want %v", field, v, want)
	}
	return nil
}

func (w *configWriteWorld) responseIncludesKey(key string) error {
	if w.respJSON == nil {
		_ = json.Unmarshal(w.body, &w.respJSON)
	}
	if _, ok := w.respJSON[key]; !ok {
		return fmt.Errorf("response missing field %q", key)
	}
	return nil
}

func (w *configWriteWorld) restartKeyIncludes(key string) error {
	if w.respJSON == nil {
		_ = json.Unmarshal(w.body, &w.respJSON)
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

func (w *configWriteWorld) liveKeyIncludes(key string) error {
	if w.respJSON == nil {
		_ = json.Unmarshal(w.body, &w.respJSON)
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

func (w *configWriteWorld) normalizedKey(oldKey, newKey string) error {
	if w.respJSON == nil {
		_ = json.Unmarshal(w.body, &w.respJSON)
	}
	nk, ok := w.respJSON["normalized_keys"]
	if !ok {
		return fmt.Errorf("response missing normalized_keys")
	}
	arr, ok := nk.([]any)
	if !ok {
		return fmt.Errorf("normalized_keys is not an array")
	}
	for _, v := range arr {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if m["old"] == oldKey && m["new"] == newKey {
			return nil
		}
	}
	return fmt.Errorf("normalized_keys does not include %s -> %s", oldKey, newKey)
}

func (w *configWriteWorld) testResponseShows(key, expected string) error {
	if w.respJSON == nil {
		return fmt.Errorf("no test response JSON")
	}
	v, ok := w.respJSON[key]
	if !ok {
		return fmt.Errorf("test response missing key %q", key)
	}
	switch val := v.(type) {
	case bool:
		expectedBool := expected == "true"
		if val != expectedBool {
			return fmt.Errorf("test response key %q = %v, want %v", key, val, expectedBool)
		}
	case string:
		if val != expected {
			return fmt.Errorf("test response key %q = %q, want %q", key, val, expected)
		}
	default:
		return fmt.Errorf("test response key %q = %v (unexpected type %T)", key, val, val)
	}
	return nil
}

func (w *configWriteWorld) testRejected(reason string) error {
	if w.respJSON == nil {
		return fmt.Errorf("no test response JSON")
	}
	errField, ok := w.respJSON["error"]
	if !ok {
		return fmt.Errorf("test response missing error field")
	}
	errStr, ok := errField.(string)
	if !ok {
		return fmt.Errorf("error field is not a string: %v", errField)
	}
	if !strings.Contains(errStr, reason) {
		return fmt.Errorf("error = %q, want to contain %q", errStr, reason)
	}
	return nil
}

func (w *configWriteWorld) testReturnsStatus(expectedStatus int) error {
	if w.status == 0 {
		return fmt.Errorf("no HTTP status recorded")
	}
	if w.status != expectedStatus {
		return fmt.Errorf("HTTP status = %d, want %d (body: %s)", w.status, expectedStatus, string(w.body))
	}
	return nil
}

// --- configWriteProvider SetConfig ---

func (p *configWriteProvider) SetConfig(payload map[string]any) (*transporthttp.ConfigWriteResult, error) {
	// Validate provider if present.
	if v, ok := payload["provider"]; ok {
		if s, ok := v.(string); ok {
			if s != "ollama" && s != "llamacpp" {
				return &transporthttp.ConfigWriteResult{
					Status: "error",
					Errors: []string{"provider: must be one of: ollama, llamacpp"},
				}, nil
			}
		}
	}

	// Classify keys as restart-required or live.
	restartKeys := []string{}
	liveKeys := []string{}
	for key := range payload {
		switch key {
		case "provider", "ollama_host", "ollama_model", "llamacpp_host", "llamacpp_model",
			"transport.host", "transport.port_start", "transport.port_end":
			restartKeys = append(restartKeys, key)
		default:
			liveKeys = append(liveKeys, key)
		}
	}

	// Normalize deprecated chat_backend → provider.
	var normKeys []transporthttp.NormalizedKey
	if cb, ok := payload["chat_backend"]; ok {
		if s, ok := cb.(string); ok && s != "" {
			if _, hasProvider := payload["provider"]; !hasProvider {
				payload["provider"] = s
				normKeys = append(normKeys, transporthttp.NormalizedKey{Old: "chat_backend", New: "provider"})
			}
		}
	}

	p.writtenCfg = payload

	return &transporthttp.ConfigWriteResult{
		Status:          "applied",
		LiveApplied:     liveKeys,
		RestartRequired: restartKeys,
		NormalizedKeys:  normKeys,
	}, nil
}
