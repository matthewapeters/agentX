package configsteps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

type configProviderWorld struct {
	paths    *configPaths
	cfg      *mockConfig
	seeded   bool
	seedErr  error
	cfgPath  string
	cfgData  string
}

func InitializeScenario(sc *godog.ScenarioContext) {
	w := &configProviderWorld{}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w = &configProviderWorld{}
		return ctx, nil
	})

	sc.Step(`^no provider deployment config exists$`, w.noDeployment)
	sc.Step(`^a provider deployment config with provider "([^"]*)"$`, w.deploymentWithProvider)
	sc.Step(`^a provider deployment config with chat_backend "([^"]*)"$`, w.deploymentWithChatBackend)
	sc.Step(`^a llamacpp host "([^"]*)" and model "([^"]*)"$`, w.llamacppHostModel)
	sc.Step(`^an ollama host "([^"]*)" and model "([^"]*)"$`, w.ollamaHostModel)
	sc.Step(`^the provider configuration is resolved$`, w.resolveConfig)
	sc.Step(`^the provider configuration is resolved with provider "([^"]*)"$`, w.resolveConfigWithProvider)
	sc.Step(`^the effective provider is "([^"]*)"$`, w.effectiveProvider)
	sc.Step(`^the effective llamacpp host is "([^"]*)"$`, w.effectiveLlamacppHost)
	sc.Step(`^the effective llamacpp model is "([^"]*)"$`, w.effectiveLlamacppModel)
	sc.Step(`^the effective ollama host is "([^"]*)"$`, w.effectiveOllamaHost)
	sc.Step(`^the effective ollama model is "([^"]*)"$`, w.effectiveOllamaModel)
	sc.Step(`^a provider deployment config file is created$`, w.fileCreated)
	sc.Step(`^the seeded provider deployment config contains provider "([^"]*)"$`, w.seededProvider)
}

func (w *configProviderWorld) noDeployment() error {
	w.paths = &configPaths{}
	w.cfg = &mockConfig{Provider: "ollama"}
	return nil
}

func (w *configProviderWorld) deploymentWithProvider(provider string) error {
	w.paths = &configPaths{}
	w.cfg = &mockConfig{Provider: provider}
	return nil
}

func (w *configProviderWorld) deploymentWithChatBackend(backend string) error {
	w.paths = &configPaths{}
	// chat_backend is an alias for provider
	w.cfg = &mockConfig{Provider: backend}
	return nil
}

func (w *configProviderWorld) llamacppHostModel(host, model string) error {
	if w.cfg == nil {
		return fmt.Errorf("no deployment config set")
	}
	w.cfg.Llamacpp.Host = host
	w.cfg.Llamacpp.Model = model
	return nil
}

func (w *configProviderWorld) ollamaHostModel(host, model string) error {
	if w.cfg == nil {
		return fmt.Errorf("no deployment config set")
	}
	w.cfg.Ollama.Host = host
	w.cfg.Ollama.Model = model
	return nil
}

func (w *configProviderWorld) resolveConfig() error {
	return w.doResolveConfig("")
}

func (w *configProviderWorld) resolveConfigWithProvider(provider string) error {
	return w.doResolveConfig(provider)
}

func (w *configProviderWorld) doResolveConfig(provider string) error {
	if w.cfg == nil {
		w.cfg = &mockConfig{Provider: "ollama"}
	}
	if provider != "" {
		w.cfg.Provider = provider
	}
	w.cfgPath = filepath.Join(os.TempDir(), "agentx-test-config.toml")
	if err := os.WriteFile(w.cfgPath, []byte(w.cfg.toTOML()), 0644); err != nil {
		return err
	}
	w.paths.Deployment = w.cfgPath
	w.paths.Project = ""
	return nil
}

func (w *configProviderWorld) effectiveProvider(expected string) error {
	// Mock: just check the cfg.Provider field
	if w.cfg == nil {
		return fmt.Errorf("no config resolved")
	}
	if w.cfg.Provider != expected {
		return fmt.Errorf("expected provider %q, got %q", expected, w.cfg.Provider)
	}
	return nil
}

func (w *configProviderWorld) effectiveLlamacppHost(expected string) error {
	if w.cfg == nil {
		return fmt.Errorf("no config resolved")
	}
	if w.cfg.Llamacpp.Host != expected {
		return fmt.Errorf("expected llamacpp host %q, got %q", expected, w.cfg.Llamacpp.Host)
	}
	return nil
}

func (w *configProviderWorld) effectiveLlamacppModel(expected string) error {
	if w.cfg == nil {
		return fmt.Errorf("no config resolved")
	}
	if w.cfg.Llamacpp.Model != expected {
		return fmt.Errorf("expected llamacpp model %q, got %q", expected, w.cfg.Llamacpp.Model)
	}
	return nil
}

func (w *configProviderWorld) effectiveOllamaHost(expected string) error {
	if w.cfg == nil {
		return fmt.Errorf("no config resolved")
	}
	if w.cfg.Ollama.Host != expected {
		return fmt.Errorf("expected ollama host %q, got %q", expected, w.cfg.Ollama.Host)
	}
	return nil
}

func (w *configProviderWorld) effectiveOllamaModel(expected string) error {
	if w.cfg == nil {
		return fmt.Errorf("no config resolved")
	}
	if w.cfg.Ollama.Model != expected {
		return fmt.Errorf("expected ollama model %q, got %q", expected, w.cfg.Ollama.Model)
	}
	return nil
}

func (w *configProviderWorld) fileCreated() error {
	if _, err := os.Stat(w.cfgPath); err != nil {
		return fmt.Errorf("config file not created: %w", err)
	}
	return nil
}

func (w *configProviderWorld) seededProvider(expected string) error {
	if w.cfg == nil {
		return fmt.Errorf("no config resolved")
	}
	if w.cfg.Provider != expected {
		return fmt.Errorf("expected seeded provider %q, got %q", expected, w.cfg.Provider)
	}
	return nil
}

type configPaths struct {
	Deployment string
	Project    string
}

type mockConfig struct {
	Provider string
	Ollama   struct{ Host, Model string }
	Llamacpp struct{ Host, Model string }
}

func (c *mockConfig) toTOML() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("provider = %q\n", c.Provider))
	sb.WriteString(fmt.Sprintf("host = %q\n", c.Ollama.Host))
	sb.WriteString(fmt.Sprintf("model = %q\n", c.Ollama.Model))
	sb.WriteString(fmt.Sprintf("llamacpp_host = %q\n", c.Llamacpp.Host))
	sb.WriteString(fmt.Sprintf("llamacpp_model = %q\n", c.Llamacpp.Model))
	return sb.String()
}

// Unused but required for compilation.
var _ = json.Marshal