package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setWidgetTestEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func createWidgetTestProjectDir(t *testing.T, files []string, dirs []string) string {
	t.Helper()

	projectDir := t.TempDir()
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(projectDir, dir), 0o755); err != nil {
			t.Fatalf("MkdirAll failed for %q: %v", dir, err)
		}
	}
	for _, file := range files {
		fullPath := filepath.Join(projectDir, file)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll failed for file parent %q: %v", file, err)
		}
		if err := os.WriteFile(fullPath, []byte("test"), 0o644); err != nil {
			t.Fatalf("WriteFile failed for %q: %v", file, err)
		}
	}
	return projectDir
}

func createWidgetTestConfigProject(t *testing.T, configContents string) (string, string) {
	t.Helper()

	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	if err := os.WriteFile(configPath, []byte(configContents), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", configPath, err)
	}
	return projectDir, configPath
}

func runHeadlessCommandScript(
	t *testing.T,
	script string,
	readerFactory func(context.Context, io.Reader) <-chan string,
) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	commands := readerFactory(ctx, strings.NewReader(script))
	captured := make([]string, 0)

	for {
		select {
		case cmd, ok := <-commands:
			if !ok {
				return captured
			}
			captured = append(captured, cmd)
		case <-ctx.Done():
			t.Fatalf("timed out collecting command script output")
		}
	}
}

func runHeadlessWidgetLoopScript(
	t *testing.T,
	script string,
	runLoop func(context.Context, io.Reader, io.Writer) error,
) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out := &bytes.Buffer{}
	if err := runLoop(ctx, strings.NewReader(script), out); err != nil {
		t.Fatalf("widget loop returned error: %v", err)
	}

	return out.String()
}

func runHeadlessWidgetCommandScript(
	t *testing.T,
	script string,
	runCommand func(io.Reader, io.Writer) int,
) (int, string) {
	t.Helper()

	out := &bytes.Buffer{}
	exitCode := runCommand(strings.NewReader(script), out)
	return exitCode, out.String()
}
