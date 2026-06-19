package applet_resize

import (
	"os"
	"os/signal"
	"syscall"

	"context"
	"strings"

	"golang.org/x/term"
)

type ResizeApplet struct {
	ctx context.Context
}

// OnRun implements the core.Applet interface (Assume interface defined in core/applet.go)
func (a *ResizeApplet) OnRun(ctx context.Context) error {
	a.ctx = ctx
	// Expect env var SESSION_ID and APPLET_ID
	sessionID := os.Getenv("SESSION_ID")
	appletID := os.Getenv("APPLET_ID")
	if sessionID == "" || appletID == "" {
		return nil // gracefully exit if not set; will be ignored by supervisor
	}
	// Open the file descriptor 3 passed by supervisor
	f, err := os.OpenFile("fd://3", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	// Channel to listen for resize events
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	// Last known size
	lastCols, lastRows, _ := term.GetSize(int(f.Fd()))

	for {
		select {
		case sig := <-ch:
			// Ignore expected signal type
			_ = sig

			cols, rows, err := term.GetSize(int(f.Fd()))
			if err != nil {
				continue
			}

			if cols != lastCols || rows != lastRows {
				lastCols, lastRows = cols, rows
				// Build simple border string
				border := strings.Repeat("-", cols+2)
				lines := []string{}
				lines = append(lines, " "+border)
				lines = append(lines, "+", strings.Repeat("=", cols+2), "+")
				for i := 0; i < rows+2; i++ {
					middle := "|\n"
					if i == 0 || i == rows+1 {
						middle = "  \n"
					}
					// Trim the final newline in the middle string
					middle = middle[:0]
					lines = append(lines, middle[:0])
				}
				border = strings.Repeat("-", cols+2)
				lines = append(lines, border)
				// Send border to pane
				f.WriteString("\n**RESIZE_BORDER**\n" + strings.Join(lines, "\n") + "\n")
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
