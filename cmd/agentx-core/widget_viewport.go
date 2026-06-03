package main

import (
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// resolveWidgetPaneSize returns pane dimensions with env fallback for non-TTY tests.
func resolveWidgetPaneSize() (height int, width int) {
	height = 40
	width = 100
	if raw := strings.TrimSpace(os.Getenv("LINES")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 4 {
			height = parsed
		}
	}
	if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 20 {
			width = parsed
		}
	}
	return height, width
}

// resolveWidgetPaneSizeForWriter prefers actual terminal size when available.
func resolveWidgetPaneSizeForWriter(out io.Writer) (height int, width int) {
	if file, ok := out.(*os.File); ok {
		fd := int(file.Fd())
		if term.IsTerminal(fd) {
			if w, h, err := term.GetSize(fd); err == nil && h > 0 && w > 0 {
				return h, w
			}
		}
	}
	return resolveWidgetPaneSize()
}

// resolveWidgetViewport centralizes viewport row/column sizing across applets.
func resolveWidgetViewport(out io.Writer, headerLines int, borderLines int, promptMode bool, defaultCols int, minCols int) (rows int, cols int) {
	height, width := resolveWidgetPaneSizeForWriter(out)

	rows = height - headerLines - borderLines
	if promptMode {
		rows--
	}
	if rows < 1 {
		rows = 1
	}

	cols = defaultCols
	if width > 4 {
		cols = width - 4
	}
	if cols < minCols {
		cols = minCols
	}
	return rows, cols
}
