package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRenderLogsWidgetLines_ShowsHeader(t *testing.T) {
	lines := renderLogsWidgetLines(nil, 10, 80, false)
	if len(lines) == 0 || lines[0] != "[LOGS]" {
		t.Fatalf("expected [LOGS] header, got: %v", lines)
	}
}

func TestRenderLogsWidgetLines_ShowsConnectingOnFetchError(t *testing.T) {
	lines := renderLogsWidgetLines(nil, 10, 80, true)
	if len(lines) == 0 || !strings.Contains(lines[0], "connecting") {
		t.Fatalf("expected connecting header on fetch error, got: %v", lines)
	}
}

func TestRenderLogsWidgetLines_RendersEvents(t *testing.T) {
	events := []LogEvent{
		{At: time.Now(), Message: "test event one"},
		{At: time.Now(), Message: "test event two"},
	}
	lines := renderLogsWidgetLines(events, 10, 80, false)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "test event one") {
		t.Fatalf("expected event one in output, got:\n%s", joined)
	}
	if !strings.Contains(joined, "test event two") {
		t.Fatalf("expected event two in output, got:\n%s", joined)
	}
}

func TestRenderLogsWidgetLines_CapsToHeight(t *testing.T) {
	events := make([]LogEvent, 50)
	for i := range events {
		events[i] = LogEvent{At: time.Now(), Message: "line"}
	}
	lines := renderLogsWidgetLines(events, 8, 80, false)
	if len(lines) > 8 {
		t.Fatalf("expected at most 8 lines for height=8, got %d", len(lines))
	}
}

func TestResolveWidgetPaneSizeAtStartup_UsesSeededPaneDimensionsOnce(t *testing.T) {
	t.Setenv("AGENTX_WIDGET_PANE_HEIGHT", "37")
	t.Setenv("AGENTX_WIDGET_PANE_WIDTH", "111")
	t.Setenv("LINES", "20")
	t.Setenv("COLUMNS", "60")

	height, width := resolveWidgetPaneSizeAtStartup(&bytes.Buffer{})
	if height != 37 || width != 111 {
		t.Fatalf("expected seeded startup dimensions 37x111, got %dx%d", height, width)
	}

	height, width = resolveWidgetPaneSizeAtStartup(&bytes.Buffer{})
	if height != 20 || width != 60 {
		t.Fatalf("expected second startup resolution to fall back to terminal/env dimensions, got %dx%d", height, width)
	}
}

func TestResolveWidgetPaneSizeForWriter_DoesNotConsumeStartupSeed(t *testing.T) {
	t.Setenv("AGENTX_WIDGET_PANE_HEIGHT", "37")
	t.Setenv("AGENTX_WIDGET_PANE_WIDTH", "111")
	t.Setenv("LINES", "20")
	t.Setenv("COLUMNS", "60")

	height, width := resolveWidgetPaneSizeForWriter(nil)
	if height != 20 || width != 60 {
		t.Fatalf("expected writer resolution to use terminal/env dimensions, got %dx%d", height, width)
	}

	if got := strings.TrimSpace(os.Getenv("AGENTX_WIDGET_PANE_HEIGHT")); got != "37" {
		t.Fatalf("expected startup seed height to remain available, got %q", got)
	}
	if got := strings.TrimSpace(os.Getenv("AGENTX_WIDGET_PANE_WIDTH")); got != "111" {
		t.Fatalf("expected startup seed width to remain available, got %q", got)
	}

	height, width = resolveWidgetPaneSizeAtStartup(&bytes.Buffer{})
	if height != 37 || width != 111 {
		t.Fatalf("expected startup helper to consume seed after writer resolution, got %dx%d", height, width)
	}
	if got := strings.TrimSpace(os.Getenv("AGENTX_WIDGET_PANE_HEIGHT")); got != "" {
		t.Fatalf("expected startup helper to clear seed height, got %q", got)
	}
	if got := strings.TrimSpace(os.Getenv("AGENTX_WIDGET_PANE_WIDTH")); got != "" {
		t.Fatalf("expected startup helper to clear seed width, got %q", got)
	}
}

func TestRunLogsWidgetLoop_FetchesAndRendersEvents(t *testing.T) {
	events := []LogEvent{
		{At: time.Now(), Message: "[bridge] event=go_chat_route_start details=prompt_chars=12"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"events": events})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	output := &bytes.Buffer{}
	if err := runLogsWidgetLoop(ctx, server.URL, output, 10*time.Millisecond); err != nil {
		t.Fatalf("runLogsWidgetLoop returned error: %v", err)
	}
	if !strings.Contains(output.String(), "go_chat_route_start") {
		t.Fatalf("expected event in logs widget output, got:\n%s", output.String())
	}
}

func TestRunLogsWidgetLoop_ToleratesFetchError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	output := &bytes.Buffer{}
	// Point at a guaranteed-unused port so every fetch errors out.
	if err := runLogsWidgetLoop(ctx, "http://127.0.0.1:1", output, 10*time.Millisecond); err != nil {
		t.Fatalf("runLogsWidgetLoop should tolerate fetch errors, got: %v", err)
	}
	if !strings.Contains(output.String(), "[LOGS]") {
		t.Fatalf("expected [LOGS] header even on fetch failure, got:\n%s", output.String())
	}
}
