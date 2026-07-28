package config

import (
	"testing"
)

func TestValidate_ValidOllama(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			Provider: "ollama",
			Ollama: Ollama{
				Host:  "localhost:11434",
				Model: "phi4-mini:3.8b",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidate_ValidLlamacpp(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			Provider: "llamacpp",
			Llamacpp: Llamacpp{
				Host:  "localhost:8080",
				Model: "phi4:latest",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidate_InvalidProvider(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			Provider: "llamaccp",
			Ollama: Ollama{
				Host:  "localhost:11434",
				Model: "phi4-mini:3.8b",
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid provider")
	}
	expected := `BAD CONFIGURATION FOR "provider". "llamaccp" is invalid. Must be one of "ollama", "llamacpp"`
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestValidate_OllamaNoModel(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			Provider: "ollama",
			Ollama: Ollama{
				Host:  "localhost:11434",
				Model: "",
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing ollama model")
	}
	expected := `BAD CONFIGURATION FOR "[agentx.ollama].model". Model name is required when provider = "ollama"`
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestValidate_LlamacppNoHost(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			Provider: "llamacpp",
			Llamacpp: Llamacpp{
				Host:  "",
				Model: "phi4:latest",
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing llamacpp host")
	}
	expected := `BAD CONFIGURATION FOR "[agentx.llamacpp].host". Host (host:port) is required when provider = "llamacpp"`
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestValidate_LlamacppNoModel(t *testing.T) {
	cfg := Config{
		Agentx: Agentx{
			Provider: "llamacpp",
			Llamacpp: Llamacpp{
				Host:  "localhost:8080",
				Model: "",
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing llamacpp model")
	}
	expected := `BAD CONFIGURATION FOR "[agentx.llamacpp].model". Model name is required when provider = "llamacpp"`
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}
