package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRuntimePrerequisites_SucceedsWhenBinariesHealthy(t *testing.T) {
	tmpDir := t.TempDir()
	createExecutable(t, filepath.Join(tmpDir, "tmux"), "#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"${1:-}\" == \"-V\" ]]; then\n  echo tmux 3.4\n  exit 0\nfi\nexit 1\n")
	createExecutable(t, filepath.Join(tmpDir, "tmuxp"), "#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"${1:-}\" == \"--version\" ]]; then\n  echo tmuxp 1.50\n  exit 0\nfi\nexit 1\n")

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
	})

	if err := validateRuntimePrerequisites(); err != nil {
		t.Fatalf("expected prerequisites to pass, got: %v", err)
	}
}

func TestValidateRuntimePrerequisites_FailsWhenTmuxpProbeFails(t *testing.T) {
	tmpDir := t.TempDir()
	createExecutable(t, filepath.Join(tmpDir, "tmux"), "#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"${1:-}\" == \"-V\" ]]; then\n  echo tmux 3.4\n  exit 0\nfi\nexit 1\n")
	createExecutable(t, filepath.Join(tmpDir, "tmuxp"), "#!/usr/bin/env bash\nset -euo pipefail\necho incompatible tmuxp >&2\nexit 2\n")

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
	})

	if err := validateRuntimePrerequisites(); err == nil {
		t.Fatalf("expected tmuxp probe failure")
	}
}

func createExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("failed to write executable %s: %v", path, err)
	}
}
