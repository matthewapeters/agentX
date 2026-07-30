package config

import (
	"reflect"
	"testing"
)

func TestNormalize_ChatBackendToProvider(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			ChatBackend: "ollama",
		},
	}
	got := cfg.Normalize()
	want := []NormalizedKey{{Old: "chat_backend", New: "provider"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Normalize() = %+v, want %+v", got, want)
	}
	if cfg.Agentx.Provider != "ollama" {
		t.Errorf("Provider = %q, want %q", cfg.Agentx.Provider, "ollama")
	}
}

func TestNormalize_ProviderAlreadySet(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			Provider:    "ollama",
			ChatBackend: "llamacpp",
		},
	}
	got := cfg.Normalize()
	if len(got) != 0 {
		t.Errorf("Normalize() = %+v, want empty", got)
	}
	// ChatBackend should remain unchanged — we only copy if Provider is empty.
	if cfg.Agentx.ChatBackend != "llamacpp" {
		t.Errorf("ChatBackend = %q, want %q", cfg.Agentx.ChatBackend, "llamacpp")
	}
}

func TestNormalize_BothEmpty(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{},
	}
	got := cfg.Normalize()
	if len(got) != 0 {
		t.Errorf("Normalize() = %+v, want empty", got)
	}
}

func TestNormalize_WhitespaceOnly(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			ChatBackend: "  ",
		},
	}
	got := cfg.Normalize()
	if len(got) != 0 {
		t.Errorf("Normalize() = %+v, want empty", got)
	}
}

func TestIsProviderDeprecated(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			ChatBackend: "ollama",
		},
	}
	if !cfg.IsProviderDeprecated() {
		t.Error("expected deprecated provider")
	}

	cfg2 := Config{
		Agentx: Agentx{
			Provider: "ollama",
		},
	}
	if cfg2.IsProviderDeprecated() {
		t.Error("expected non-deprecated provider")
	}
}
