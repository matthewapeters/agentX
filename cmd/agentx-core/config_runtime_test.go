package main

import (
	"os"
	"path/filepath"
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
