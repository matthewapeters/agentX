package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

func runLogsWidgetCommand(coreHTTP string, out io.Writer) int {
	_ = strings.TrimSpace(coreHTTP)
	if err := runLogsWidgetLoop(context.Background(), out, 250*time.Millisecond); err != nil {
		fmt.Fprintf(out, "Logs widget failed: %v\n", err)
		return 1
	}
	return 0
}

func runLogsWidgetLoop(ctx context.Context, out io.Writer, idleInterval time.Duration) error {
	if idleInterval <= 0 {
		idleInterval = 250 * time.Millisecond
	}
	var previousLines []string
	render := renderLogsWidget(out)
	if err := writeFilesystemWidgetFrameDiff(out, previousLines, filesystemWidgetFrameLines(render)); err != nil {
		return err
	}
	previousLines = filesystemWidgetFrameLines(render)

	ticker := time.NewTicker(idleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			render := renderLogsWidget(out)
			currentLines := filesystemWidgetFrameLines(render)
			if err := writeFilesystemWidgetFrameDiff(out, previousLines, currentLines); err != nil {
				return err
			}
			previousLines = currentLines
		}
	}
}

func renderLogsWidget(out io.Writer) string {
	height, width := resolveWidgetPaneSizeForWriter(out)
	lines := []string{
		"[LOGS]",
		"Logs ready.",
		fmt.Sprintf("pane: %dx%d", height, width),
	}
	if width >= 100 {
		lines = append(lines, "mode: expanded", "Awaiting streamed diagnostics/events.")
	} else {
		lines = append(lines, "mode: compact")
	}
	lines = fitLinesToWidth(lines, width)
	lines = clipLinesForHeight(lines, height-1)
	return strings.Join(lines, "\n")
}
