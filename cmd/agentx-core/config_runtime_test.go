package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveCoreRuntimeConfig_UsesTomlValues validates TOML runtime config loading.
//
// GIVEN agentx.toml defines ollama host/model
// WHEN resolveCoreRuntimeConfig is called
// THEN runtime config uses values from agentx.toml.
func TestResolveCoreRuntimeConfig_UsesTomlValues(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nollama_host = \"example.local:11434\"\nollama_model = \"model-from-toml\"\nchat_backend = \"ollama\"\n"
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
	if runtimeConfig.OllamaHost != defaultOllamaHost {
		t.Fatalf("expected default ollama host %q, got %q", defaultOllamaHost, runtimeConfig.OllamaHost)
	}
	if runtimeConfig.OllamaModel != defaultOllamaModel {
		t.Fatalf("expected default ollama model %q, got %q", defaultOllamaModel, runtimeConfig.OllamaModel)
	}
}
