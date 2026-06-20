package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// InputApplet wraps the existing input widget compose state as a testable
// Applet.  It embeds AppletBase to gain the HTTP render API and resize loop.
//
// Render is pure for a fixed (compose, activity) state: given (height, width)
// it seeds the viewport, calls compose.render(), and returns the lines.
// Tests inject a pre-configured compose state and a fixed activity label to
// get deterministic output without needing a live terminal.
type InputApplet struct {
	AppletBase
	compose  *inputWidgetComposeState
	activity *widgetActivityState
}

// NewInputApplet constructs an InputApplet with default compose and activity
// state.  Tests may further mutate compose and activity before calling Render.
func NewInputApplet() *InputApplet {
	return &InputApplet{
		compose:  newInputWidgetComposeState(),
		activity: newWidgetActivityState(),
	}
}

func (a *InputApplet) Name() string { return "input" }

// Render seeds the viewport from (height, width) then calls the existing
// compose.render path.  Returns the individual display lines.
// This is the single testable entry point for input widget rendering.
func (a *InputApplet) Render(height, width int) []string {
	if height < 1 {
		height = 1
	}
	if width < 1 {
		width = 1
	}
	a.compose.seedViewportFromStartup(height, width)
	frame := a.compose.render(a.activity.promptLabel())
	lines := strings.Split(frame, "\n")
	return clipLinesToHeight(lines, height)
}

// runInputWidgetCommandWithAPI is the entry point for --input-widget when
// --applet-api-addr is also supplied.  It:
//  1. Starts the AppletBase HTTP API server (GET /health, GET /render).
//  2. Registers an input-widget render observer that stores the latest
//     rendered frame in the applet snapshot for GET /render.
//  3. Runs the normal input widget loop unmodified.
//
// When apiAddr is empty the function falls back to plain runInputWidgetCommand.
func runInputWidgetCommandWithAPI(coreHTTP string, apiAddr string, in io.Reader, out io.Writer) int {
	if strings.TrimSpace(apiAddr) == "" {
		return runInputWidgetCommand(coreHTTP, in, out)
	}

	applet := NewInputApplet()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	boundAddr, err := applet.StartAPIServer(ctx, apiAddr, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[INPUT APPLET] failed to bind render API: %v\n", err)
		return 1
	}
	_ = boundAddr

	removeObserver := setInputWidgetRenderObserver(func(height int, width int, lines []string) {
		applet.StoreSnapshot(applet.Name(), height, width, clipLinesToHeight(lines, height))
	})
	defer removeObserver()

	return runInputWidgetCommand(coreHTTP, in, out)
}

func clipLinesToHeight(lines []string, height int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	clipped := make([]string, height)
	copy(clipped, lines[:height])
	return clipped
}


