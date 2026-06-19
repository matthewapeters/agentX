package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestResolveCoreRuntimeConfig_UsesTomlValues validates TOML runtime config loading.
//
// GIVEN agentx.toml defines ollama host/model
// WHEN resolveCoreRuntimeConfig is called
// THEN runtime config uses values from agentx.toml.
func TestResolveCoreRuntimeConfig_UsesTomlValues(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nollama_host = \"example.local:11434\"\nollama_model = \"model-from-toml\"\nchat_backend = \"ollama\"\nchat_runtime = \"go\"\nchat_bridge_response_timeout_seconds = 150\nsubmit_execution_timeout_seconds = 160\nsubmit_timeout_seconds = 170\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	runtimeConfig := resolveCoreRuntimeConfig(projectDir)

	if runtimeConfig.OllamaHost != "example.local:11434" {
		t.Fatalf("expected ollama host from toml, got %q", runtimeConfig.OllamaHost)
	}
	if runtimeConfig.OllamaModel != "model-from-toml" {
		t.Fatalf("expected ollama model from toml, got %q", runtimeConfig.OllamaModel)
	}
	if runtimeConfig.ChatBackend != "ollama" {
		t.Fatalf("expected chat backend from toml, got %q", runtimeConfig.ChatBackend)
	}
	if runtimeConfig.ChatRuntime != "go" {
		t.Fatalf("expected chat runtime from toml, got %q", runtimeConfig.ChatRuntime)
	}
	if runtimeConfig.MultiplexerBackend != defaultMultiplexerBackend {
		t.Fatalf("expected default multiplexer backend %q, got %q", defaultMultiplexerBackend, runtimeConfig.MultiplexerBackend)
	}
	if runtimeConfig.ChatBridgeResponseTimeout != 150*time.Second {
		t.Fatalf("expected chat bridge timeout 150s from toml, got %s", runtimeConfig.ChatBridgeResponseTimeout)
	}
	if runtimeConfig.SubmitExecutionTimeout != 160*time.Second {
		t.Fatalf("expected submit exec timeout 160s from toml, got %s", runtimeConfig.SubmitExecutionTimeout)
	}
	if runtimeConfig.SubmitTimeout != 170*time.Second {
		t.Fatalf("expected submit timeout 170s from toml, got %s", runtimeConfig.SubmitTimeout)
	}
}

// TestResolveCoreRuntimeConfig_EnvOverridesToml validates env precedence over TOML values.
//
// GIVEN agentx.toml defines ollama host/model and AGENTX_OLLAMA_* env vars are set
// WHEN resolveCoreRuntimeConfig is called
// THEN env values override agentx.toml.
func TestResolveCoreRuntimeConfig_EnvOverridesToml(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nollama_host = \"example.local:11434\"\nollama_model = \"model-from-toml\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Setenv("AGENTX_OLLAMA_HOST", "env-host:11434")
	t.Setenv("AGENTX_OLLAMA_MODEL", "env-model")
	t.Setenv("AGENTX_CHAT_BACKEND", "ollama")
	t.Setenv("AGENTX_CHAT_RUNTIME", "go")
	t.Setenv("AGENTX_CHAT_BRIDGE_RESPONSE_TIMEOUT_SEC", "180")
	t.Setenv("AGENTX_SUBMIT_EXEC_TIMEOUT_SEC", "181")
	t.Setenv("AGENTX_SUBMIT_TIMEOUT_SEC", "182")

	runtimeConfig := resolveCoreRuntimeConfig(projectDir)

	if runtimeConfig.OllamaHost != "env-host:11434" {
		t.Fatalf("expected env ollama host override, got %q", runtimeConfig.OllamaHost)
	}
	if runtimeConfig.OllamaModel != "env-model" {
		t.Fatalf("expected env ollama model override, got %q", runtimeConfig.OllamaModel)
	}
	if runtimeConfig.ChatBackend != "ollama" {
		t.Fatalf("expected env chat backend override, got %q", runtimeConfig.ChatBackend)
	}
	if runtimeConfig.ChatRuntime != "go" {
		t.Fatalf("expected env chat runtime override, got %q", runtimeConfig.ChatRuntime)
	}
	if runtimeConfig.ChatBridgeResponseTimeout != 180*time.Second {
		t.Fatalf("expected env chat bridge timeout override, got %s", runtimeConfig.ChatBridgeResponseTimeout)
	}
	if runtimeConfig.SubmitExecutionTimeout != 181*time.Second {
		t.Fatalf("expected env submit exec timeout override, got %s", runtimeConfig.SubmitExecutionTimeout)
	}
	if runtimeConfig.SubmitTimeout != 182*time.Second {
		t.Fatalf("expected env submit timeout override, got %s", runtimeConfig.SubmitTimeout)
	}
}

// TestResolveCoreRuntimeConfig_DefaultsWhenNoConfig validates fallback defaults.
//
// GIVEN no agentx.toml and no AGENTX_* overrides
// WHEN resolveCoreRuntimeConfig is called
// THEN runtime config falls back to built-in defaults.
func TestResolveCoreRuntimeConfig_DefaultsWhenNoConfig(t *testing.T) {
	runtimeConfig := resolveCoreRuntimeConfig(t.TempDir())

	if runtimeConfig.ChatBackend != defaultChatBackend {
		t.Fatalf("expected default chat backend %q, got %q", defaultChatBackend, runtimeConfig.ChatBackend)
	}
	if runtimeConfig.ChatRuntime != defaultChatRuntime {
		t.Fatalf("expected default chat runtime %q, got %q", defaultChatRuntime, runtimeConfig.ChatRuntime)
	}
	if runtimeConfig.MultiplexerBackend != defaultMultiplexerBackend {
		t.Fatalf("expected default multiplexer backend %q, got %q", defaultMultiplexerBackend, runtimeConfig.MultiplexerBackend)
	}
	if runtimeConfig.OllamaHost != defaultOllamaHost {
		t.Fatalf("expected default ollama host %q, got %q", defaultOllamaHost, runtimeConfig.OllamaHost)
	}
	if runtimeConfig.OllamaModel != defaultOllamaModel {
		t.Fatalf("expected default ollama model %q, got %q", defaultOllamaModel, runtimeConfig.OllamaModel)
	}
	if runtimeConfig.ChatBridgeResponseTimeout != time.Duration(defaultChatBridgeResponseTimeoutSeconds)*time.Second {
		t.Fatalf("expected default chat bridge timeout, got %s", runtimeConfig.ChatBridgeResponseTimeout)
	}
	if runtimeConfig.SubmitExecutionTimeout != time.Duration(defaultSubmitExecutionTimeoutSeconds)*time.Second {
		t.Fatalf("expected default submit execution timeout, got %s", runtimeConfig.SubmitExecutionTimeout)
	}
	if runtimeConfig.SubmitTimeout != time.Duration(defaultSubmitTimeoutSeconds)*time.Second {
		t.Fatalf("expected default submit timeout, got %s", runtimeConfig.SubmitTimeout)
	}
}

func TestResolveCoreRuntimeConfig_NormalizesPythonOverrideToGo(t *testing.T) {
	t.Setenv("AGENTX_CHAT_RUNTIME", "python")

	runtimeConfig := resolveCoreRuntimeConfig(t.TempDir())
	if runtimeConfig.ChatRuntime != "go" {
		t.Fatalf("expected python override to normalize to go, got %q", runtimeConfig.ChatRuntime)
	}
}

func TestResolveCoreRuntimeConfig_UsesTomlMultiplexerBackend(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nmultiplexer_backend = \"tmux\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	runtimeConfig := resolveCoreRuntimeConfig(projectDir)
	if runtimeConfig.MultiplexerBackend != defaultMultiplexerBackend {
		t.Fatalf("expected multiplexer backend from toml to resolve as %q, got %q", defaultMultiplexerBackend, runtimeConfig.MultiplexerBackend)
	}
}

func TestResolveCoreRuntimeConfig_ZellijBackendParsed(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nmultiplexer_backend = \"zellij\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	runtimeConfig := resolveCoreRuntimeConfig(projectDir)
	if runtimeConfig.MultiplexerBackend != "zellij" {
		t.Fatalf("expected zellij multiplexer backend to be preserved, got %q", runtimeConfig.MultiplexerBackend)
	}
	if got := resolveMultiplexerBackend(projectDir); got != "zellij" {
		t.Fatalf("expected resolved backend %q, got %q", "zellij", got)
	}
}

func TestResolveCoreRuntimeConfig_PreservesInvalidTomlMultiplexerBackend(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nmultiplexer_backend = \"invalid-backend\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	runtimeConfig := resolveCoreRuntimeConfig(projectDir)
	if runtimeConfig.MultiplexerBackend != "invalid-backend" {
		t.Fatalf("expected invalid multiplexer backend to be preserved for explicit driver erroring, got %q", runtimeConfig.MultiplexerBackend)
	}
}

func TestConfigPrecedence_ExplicitZellijBackend(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	if err := os.WriteFile(configPath, []byte("[agentx]\nmultiplexer_backend = \"zellij\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	driver, err := newMultiplexerDriverFromConfig(projectDir)
	if err != nil {
		t.Fatalf("expected zellij backend selection success, got %v", err)
	}
	if _, ok := driver.(*ZellijMultiplexerDriver); !ok {
		t.Fatalf("expected *ZellijMultiplexerDriver, got %T", driver)
	}
	if got := driver.BackendName(); got != "zellij" {
		t.Fatalf("BackendName() = %q, want %q", got, "zellij")
	}
}

func TestConfigPrecedence_ExplicitTmuxBackend(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	if err := os.WriteFile(configPath, []byte("[agentx]\nmultiplexer_backend = \"tmux\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	driver, err := newMultiplexerDriverFromConfig(projectDir)
	if err != nil {
		t.Fatalf("expected tmux backend selection success, got %v", err)
	}
	if _, ok := driver.(*TmuxMultiplexerDriver); !ok {
		t.Fatalf("expected *TmuxMultiplexerDriver, got %T", driver)
	}
}

func TestConfigPrecedence_UnsetBackendDefaultsToTmux(t *testing.T) {
	driver, err := newMultiplexerDriverFromConfig(t.TempDir())
	if err != nil {
		t.Fatalf("expected default backend selection success, got %v", err)
	}
	if _, ok := driver.(*TmuxMultiplexerDriver); !ok {
		t.Fatalf("expected *TmuxMultiplexerDriver, got %T", driver)
	}
}

func TestConfigPrecedence_InvalidBackendFailsDeterministically(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	if err := os.WriteFile(configPath, []byte("[agentx]\nmultiplexer_backend = \"zelij\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := newMultiplexerDriverFromConfig(projectDir)
	if err == nil {
		t.Fatal("expected invalid backend to fail")
	}
	if got := err.Error(); got != "unsupported multiplexer backend: zelij" {
		t.Fatalf("unexpected invalid backend error: got %q", got)
	}
}

func TestConfigPrecedence_LayoutResolutionRespectBackend(t *testing.T) {
	projectDir := t.TempDir()

	zellijLayoutPath, ok := backendLayoutFilePath(projectDir, "zellij")
	if !ok {
		t.Fatal("expected zellij backend layout path")
	}
	if err := os.MkdirAll(filepath.Dir(zellijLayoutPath), 0o755); err != nil {
		t.Fatalf("failed to create zellij layout dir: %v", err)
	}
	if err := os.WriteFile(zellijLayoutPath, []byte("layout {}\n"), 0o644); err != nil {
		t.Fatalf("failed to write zellij layout file: %v", err)
	}
	resolvedZellij, err := resolveImplicitLayoutFile(projectDir, "zellij")
	if err != nil {
		t.Fatalf("expected zellij layout resolution success, got %v", err)
	}
	if resolvedZellij != zellijLayoutPath {
		t.Fatalf("resolved zellij layout mismatch: got %q want %q", resolvedZellij, zellijLayoutPath)
	}

	tmuxLegacyPath := defaultLayoutFilePath(projectDir)
	if err := os.MkdirAll(filepath.Dir(tmuxLegacyPath), 0o755); err != nil {
		t.Fatalf("failed to create tmux layout dir: %v", err)
	}
	if err := os.WriteFile(tmuxLegacyPath, []byte("session_name: ${SESSION}\n"), 0o644); err != nil {
		t.Fatalf("failed to write legacy tmux layout file: %v", err)
	}
	resolvedTmux, err := resolveImplicitLayoutFile(projectDir, "tmux")
	if err != nil {
		t.Fatalf("expected tmux layout resolution success, got %v", err)
	}
	if resolvedTmux != tmuxLegacyPath {
		t.Fatalf("resolved tmux layout mismatch: got %q want %q", resolvedTmux, tmuxLegacyPath)
	}
}

func TestConfigPrecedence_StartupUsesConfiguredBackend(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	if err := os.WriteFile(configPath, []byte("[agentx]\nmultiplexer_backend = \"zellij\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	layoutPath, ok := backendLayoutFilePath(projectDir, "zellij")
	if !ok {
		t.Fatal("expected zellij backend layout path")
	}
	if err := os.MkdirAll(filepath.Dir(layoutPath), 0o755); err != nil {
		t.Fatalf("failed to create zellij layout dir: %v", err)
	}
	if err := os.WriteFile(layoutPath, []byte("layout {}\n"), 0o644); err != nil {
		t.Fatalf("failed to write zellij layout file: %v", err)
	}

	driver, err := newMultiplexerDriverFromConfig(projectDir)
	if err != nil {
		t.Fatalf("expected zellij backend selection success, got %v", err)
	}
	command := buildNewSessionCommand(driver.BackendName(), "agentx_test", 120, 40, layoutPath)
	// zellij 0.40+: background session creation via attach --create-background; layout not supported here
	wantCommand := []string{"attach", "--create-background", "agentx_test"}
	if len(command) != len(wantCommand) {
		t.Fatalf("startup command length mismatch: got %v want %v", command, wantCommand)
	}
	for idx := range wantCommand {
		if command[idx] != wantCommand[idx] {
			t.Fatalf("startup command mismatch at %d: got %q want %q", idx, command[idx], wantCommand[idx])
		}
	}
	startupLog := fmt.Sprintf("[AgentX Core] ✓ %s session initialized", driver.BackendName())
	attachLog := fmt.Sprintf("[AgentX Core] Attaching to %s session '%s'...", driver.BackendName(), "agentx_test")
	if !strings.Contains(startupLog, "zellij session") {
		t.Fatalf("expected startup log to mention zellij session, got %q", startupLog)
	}
	if !strings.Contains(attachLog, "Attaching to zellij session") {
		t.Fatalf("expected attach log to mention zellij session, got %q", attachLog)
	}
}
