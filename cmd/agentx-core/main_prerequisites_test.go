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

	if err := validateRuntimePrerequisites(t.TempDir()); err != nil {
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

	if err := validateRuntimePrerequisites(t.TempDir()); err == nil {
		t.Fatalf("expected tmuxp probe failure")
	}
}

func TestValidateRuntimePrerequisites_ZellijBackendSkipsTmuxpProbe(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agentx.toml")
	if err := os.WriteFile(configPath, []byte("[agentx]\nmultiplexer_backend = \"zellij\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	createExecutable(t, filepath.Join(tmpDir, "zellij"), "#!/usr/bin/env bash\nset -euo pipefail\nwhile [[ $# -gt 0 ]]; do\n  case \"$1\" in\n    --config-dir)\n      shift 2\n      ;;\n    -V)\n      echo zellij 0.39.0\n      exit 0\n      ;;\n    *)\n      shift\n      ;;\n  esac\ndone\nexit 1\n")

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
	})

	if err := validateRuntimePrerequisites(tmpDir); err != nil {
		t.Fatalf("expected zellij prerequisites to pass without tmuxp, got: %v", err)
	}
}

func createExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("failed to write executable %s: %v", path, err)
	}
}
