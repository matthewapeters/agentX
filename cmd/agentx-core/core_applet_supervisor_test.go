package main

import (
	"context"
	"errors"
	"testing"
)

// TestStartAppletSupervisorTracksDefaultPanes validates that supervisor initialization
// seeds tracked applets for each default pane with ready status.
//
// GIVEN a new core with no tracked applets
// WHEN StartAppletSupervisor is invoked
// THEN each default pane applet is tracked with ready status.
func TestStartAppletSupervisorTracksDefaultPanes(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-app"}
	core := NewAgentXCore(cfg)

	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor returned error: %v", err)
	}

	snapshot := core.healthSnapshot()
	expected := len(DefaultPaneLayout())
	if len(snapshot.Applets) != expected {
		t.Fatalf("expected %d applets, got %d", expected, len(snapshot.Applets))
	}

	for _, applet := range snapshot.Applets {
		if applet.Status != string(AppletStatusReady) {
			t.Fatalf("expected applet %s to be ready, got %s", applet.Name, applet.Status)
		}
	}
}

// TestMarkAppletStatusTracksCrashLifecycle validates crash transitions and accounting.
//
// GIVEN a tracked applet
// WHEN it is marked crashed and then stopped
// THEN health snapshot reflects status transitions and crash count increment.
func TestMarkAppletStatusTracksCrashLifecycle(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-crash"}
	core := NewAgentXCore(cfg)

	core.mu.Lock()
	core.applets["chat"] = &AppletProcess{Name: "chat", PaneName: "chat", Status: AppletStatusReady}
	core.mu.Unlock()

	core.markAppletStatus("chat", AppletStatusCrashed, errors.New("boom"))
	snapshot := core.healthSnapshot()
	if len(snapshot.Applets) != 1 {
		t.Fatalf("expected 1 applet, got %d", len(snapshot.Applets))
	}
	if snapshot.Applets[0].Status != string(AppletStatusCrashed) {
		t.Fatalf("expected crashed status, got %s", snapshot.Applets[0].Status)
	}
	if snapshot.Applets[0].CrashCount != 1 {
		t.Fatalf("expected crash count 1, got %d", snapshot.Applets[0].CrashCount)
	}

	core.markAppletStatus("chat", AppletStatusStopped, nil)
	snapshot = core.healthSnapshot()
	if snapshot.Applets[0].Status != string(AppletStatusStopped) {
		t.Fatalf("expected stopped status, got %s", snapshot.Applets[0].Status)
	}
	if snapshot.Applets[0].CrashCount != 1 {
		t.Fatalf("expected crash count to stay 1, got %d", snapshot.Applets[0].CrashCount)
	}
}
