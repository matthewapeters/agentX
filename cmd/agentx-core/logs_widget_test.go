package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

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
