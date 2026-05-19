package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// GIVEN a context manager configured with runtime snapshot metadata
// WHEN health endpoints are queried
// THEN deterministic JSON payloads should expose session, pane, and applet state.
func TestContextManagerHealthHandler_ReportsRuntimeState(t *testing.T) {
	cm := NewContextManager(t.TempDir())
	cm.SetSessionMetadata("sess-health", time.Now().Add(-10*time.Second))
	cm.SetSnapshotProvider(func() HealthSnapshot {
		return HealthSnapshot{
			Status:        "ok",
			SessionID:     "sess-health",
			UptimeSeconds: 10,
			Panes: []PaneStatus{
				{Name: "chat", Applet: "chat", Status: "ready"},
				{Name: "logs", Applet: "logs", Status: "ready"},
			},
			Applets: []AppletStatus{
				{Name: "chat", Pane: "chat", Status: "running", CrashCount: 0},
			},
		}
	})

	server := httptest.NewServer(cm.HealthHandler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("failed to query /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /health status 200, got %d", resp.StatusCode)
	}

	var health map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode /health payload: %v", err)
	}

	if got := health["session_id"]; got != "sess-health" {
		t.Fatalf("expected /health session_id sess-health, got %v", got)
	}
	if got := int(health["pane_count"].(float64)); got != 2 {
		t.Fatalf("expected /health pane_count 2, got %d", got)
	}
	if got := int(health["applet_count"].(float64)); got != 1 {
		t.Fatalf("expected /health applet_count 1, got %d", got)
	}

	panesResp, err := http.Get(server.URL + "/panes")
	if err != nil {
		t.Fatalf("failed to query /panes: %v", err)
	}
	defer panesResp.Body.Close()

	if panesResp.StatusCode != http.StatusOK {
		t.Fatalf("expected /panes status 200, got %d", panesResp.StatusCode)
	}

	var panesPayload struct {
		SessionID string       `json:"session_id"`
		Panes     []PaneStatus `json:"panes"`
	}
	if err := json.NewDecoder(panesResp.Body).Decode(&panesPayload); err != nil {
		t.Fatalf("failed to decode /panes payload: %v", err)
	}
	if panesPayload.SessionID != "sess-health" {
		t.Fatalf("expected /panes session_id sess-health, got %q", panesPayload.SessionID)
	}
	if len(panesPayload.Panes) != 2 {
		t.Fatalf("expected /panes length 2, got %d", len(panesPayload.Panes))
	}

	appletsResp, err := http.Get(server.URL + "/applets")
	if err != nil {
		t.Fatalf("failed to query /applets: %v", err)
	}
	defer appletsResp.Body.Close()

	if appletsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected /applets status 200, got %d", appletsResp.StatusCode)
	}

	var appletsPayload struct {
		SessionID string         `json:"session_id"`
		Applets   []AppletStatus `json:"applets"`
	}
	if err := json.NewDecoder(appletsResp.Body).Decode(&appletsPayload); err != nil {
		t.Fatalf("failed to decode /applets payload: %v", err)
	}
	if appletsPayload.SessionID != "sess-health" {
		t.Fatalf("expected /applets session_id sess-health, got %q", appletsPayload.SessionID)
	}
	if len(appletsPayload.Applets) != 1 {
		t.Fatalf("expected /applets length 1, got %d", len(appletsPayload.Applets))
	}
	if appletsPayload.Applets[0].Name != "chat" {
		t.Fatalf("expected first applet name chat, got %q", appletsPayload.Applets[0].Name)
	}
}

// GIVEN endpoint method constraints for runtime health handlers
// WHEN a non-GET request is sent
// THEN the handler should return method-not-allowed.
func TestContextManagerHealthHandler_RejectsNonGetMethods(t *testing.T) {
	cm := NewContextManager(t.TempDir())
	server := httptest.NewServer(cm.HealthHandler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/health", nil)
	if err != nil {
		t.Fatalf("failed to build POST /health request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to execute POST /health request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected /health status 405 for POST, got %d", resp.StatusCode)
	}
}
