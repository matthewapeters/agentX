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
	if _, err := fmt.Fprintln(out, "Logs ready."); err != nil {
		return err
	}

	ticker := time.NewTicker(idleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
