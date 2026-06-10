package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRenderLogsWidget_AdaptsToPaneWidth(t *testing.T) {
	t.Setenv("LINES", "20")
	t.Setenv("COLUMNS", "120")
	renderWide := renderLogsWidget(nil)
	if !strings.Contains(renderWide, "mode: expanded") {
		t.Fatalf("expected expanded mode on wide pane, got:\n%s", renderWide)
	}

	t.Setenv("COLUMNS", "60")
	renderNarrow := renderLogsWidget(nil)
	if !strings.Contains(renderNarrow, "mode: compact") {
		t.Fatalf("expected compact mode on narrow pane, got:\n%s", renderNarrow)
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

func TestRunLogsWidgetLoop_PrintsReadyBanner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	output := &bytes.Buffer{}
	if err := runLogsWidgetLoop(ctx, output, 10*time.Millisecond); err != nil {
		t.Fatalf("runLogsWidgetLoop returned error: %v", err)
	}

	if !strings.Contains(output.String(), "Logs ready.") {
		t.Fatalf("expected logs ready banner, got:\n%s", output.String())
	}
}
