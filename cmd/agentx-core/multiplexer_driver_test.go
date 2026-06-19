package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveMultiplexerBackend_DefaultsToTmuxWhenUnset(t *testing.T) {
	if got := resolveMultiplexerBackend(t.TempDir()); got != defaultMultiplexerBackend {
		t.Fatalf("resolveMultiplexerBackend() = %q, want %q", got, defaultMultiplexerBackend)
	}
}

func TestResolveMultiplexerBackend_UsesAgentxTomlValue(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nmultiplexer_backend = \"tmux\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if got := resolveMultiplexerBackend(projectDir); got != defaultMultiplexerBackend {
		t.Fatalf("resolveMultiplexerBackend() = %q, want %q", got, defaultMultiplexerBackend)
	}
}

func TestNewMultiplexerDriverFromConfig_ZellijBackendReturnsZellijDriver(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nmultiplexer_backend = \"zellij\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	driver, err := newMultiplexerDriverFromConfig(projectDir)
	if err != nil {
		t.Fatalf("expected zellij backend selection success, got %v", err)
	}
	if _, ok := driver.(*ZellijMultiplexerDriver); !ok {
		t.Fatalf("expected *ZellijMultiplexerDriver, got %T", driver)
	}
}

func TestNewMultiplexerDriverFromConfig_TmuxBackendReturnsDefaultForUnset(t *testing.T) {
	projectDir := t.TempDir()

	driver, err := newMultiplexerDriverFromConfig(projectDir)
	if err != nil {
		t.Fatalf("expected default backend selection success, got %v", err)
	}
	if _, ok := driver.(*TmuxMultiplexerDriver); !ok {
		t.Fatalf("expected *TmuxMultiplexerDriver, got %T", driver)
	}
}

func TestNewMultiplexerDriverFromConfig_InvalidBackendFailsDeterministically(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nmultiplexer_backend = \"invalid-backend\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := newMultiplexerDriverFromConfig(projectDir)
	if err == nil {
		t.Fatal("expected invalid backend to fail")
	}
	if !strings.Contains(err.Error(), "unsupported multiplexer backend: invalid-backend") {
		t.Fatalf("expected deterministic invalid backend error, got %q", err.Error())
	}
}

func TestTmuxMultiplexerDriverRunCombined_ReturnsOutputOnSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	createExecutable(t, filepath.Join(tmpDir, "tmux"), "#!/usr/bin/env bash\nset -euo pipefail\necho stdout-ok\necho stderr-ok >&2\n")

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
	})

	driver := NewTmuxMultiplexerDriver()
	output, err := driver.RunCombined(context.Background(), "display-message", "-p", "ok")
	if err != nil {
		t.Fatalf("expected RunCombined success, got %v", err)
	}
	if !strings.Contains(output, "stdout-ok") {
		t.Fatalf("expected stdout in combined output, got %q", output)
	}
	if !strings.Contains(output, "stderr-ok") {
		t.Fatalf("expected stderr in combined output, got %q", output)
	}
	if strings.HasPrefix(output, "\n") || strings.HasSuffix(output, "\n") {
		t.Fatalf("expected combined output to be trimmed, got %q", output)
	}
}

func TestTmuxMultiplexerDriverRunCombined_ReturnsOutputAndErrorOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	createExecutable(t, filepath.Join(tmpDir, "tmux"), "#!/usr/bin/env bash\nset -euo pipefail\necho stdout-fail\necho stderr-fail >&2\nexit 7\n")

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
	})

	driver := NewTmuxMultiplexerDriver()
	output, err := driver.RunCombined(context.Background(), "display-message", "-p", "fail")
	if err == nil {
		t.Fatal("expected RunCombined error")
	}
	if !strings.Contains(output, "stdout-fail") {
		t.Fatalf("expected stdout in combined output on error, got %q", output)
	}
	if !strings.Contains(output, "stderr-fail") {
		t.Fatalf("expected stderr in combined output on error, got %q", output)
	}
	if strings.HasPrefix(output, "\n") || strings.HasSuffix(output, "\n") {
		t.Fatalf("expected combined output on error to be trimmed, got %q", output)
	}
}
