package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// logsWidgetEventsResponse mirrors the JSON shape of GET /events.
type logsWidgetEventsResponse struct {
	Events []LogEvent `json:"events"`
}

func fetchLogsWidgetEvents(ctx context.Context, baseURL string) ([]LogEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/events", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload logsWidgetEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Events, nil
}

// runLogsWidgetCommand starts the logs widget loop against the core HTTP API.
func runLogsWidgetCommand(coreHTTP string, out io.Writer) int {
	baseURL := strings.TrimRight(strings.TrimSpace(coreHTTP), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("AGENTX_CORE_HTTP")), "/")
	}
	if baseURL == "" {
		fmt.Fprintln(out, "Logs widget failed: missing core HTTP base URL")
		return 1
	}
	ctx, cancel := widgetCommandContext()
	defer cancel()
	stopWatchdog := startWidgetCoreWatchdog(resolveWidgetCorePIDFromEnv(), 500*time.Millisecond, os.Stderr, cancel)
	defer stopWatchdog()
	if err := runLogsWidgetLoop(ctx, baseURL, out, 250*time.Millisecond); err != nil {
		fmt.Fprintf(out, "Logs widget failed: %v\n", err)
		return 1
	}
	return 0
}

// runLogsWidgetLoop polls /events on an interval and redraws the pane when
// the rendered log lines change.
func runLogsWidgetLoop(ctx context.Context, baseURL string, out io.Writer, idleInterval time.Duration) error {
	hideTerminalCursor(out)
	defer showTerminalCursor(out)

	if idleInterval <= 0 {
		idleInterval = 250 * time.Millisecond
	}

	var previousLines []string
	var lastEvents []LogEvent

	ticker := time.NewTicker(idleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			fetchCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
			events, err := fetchLogsWidgetEvents(fetchCtx, baseURL)
			cancel()
			if err == nil {
				lastEvents = events
			}
			height, width := resolveWidgetPaneSizeForWriter(out)
			currentLines := renderLogsWidgetLines(lastEvents, height, width, err != nil)
			if strings.Join(previousLines, "\n") != strings.Join(currentLines, "\n") {
				if writeErr := writeFilesystemWidgetFrameDiff(out, previousLines, currentLines); writeErr != nil {
					return writeErr
				}
				previousLines = currentLines
			}
		}
	}
}

// renderLogsWidgetLines renders a fixed-height tail view where the newest
// events stay visible and the frame is padded to fill the pane.
func renderLogsWidgetLines(events []LogEvent, height, width int, fetchErr bool) []string {
	if height < 1 {
		height = 1
	}
	if width < 1 {
		width = 1
	}

	header := "[LOGS]"
	if fetchErr {
		header = "[LOGS] (connecting…)"
	}

	// Reserve 1 row for the header; show the most-recent events that fit.
	maxRows := height - 1
	if maxRows < 1 {
		maxRows = 1
	}

	// Format each event as "HH:MM:SS message", trimmed to pane width.
	formatted := make([]string, 0, len(events))
	for _, ev := range events {
		ts := ev.At.Local().Format("15:04:05")
		line := ts + " " + ev.Message
		if width > 0 && len(line) > width {
			line = line[:width]
		}
		formatted = append(formatted, line)
	}

	// Take the tail so the newest events are always visible.
	if len(formatted) > maxRows {
		formatted = formatted[len(formatted)-maxRows:]
	}

	// Pad with blank lines so the widget always fills the pane height.
	for len(formatted) < maxRows {
		formatted = append(formatted, "")
	}

	lines := append([]string{header}, formatted...)
	return clipLinesForHeight(lines, height)
}
