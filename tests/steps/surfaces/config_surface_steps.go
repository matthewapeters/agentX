// Package surfacesteps implements Godog step definitions for the config
// surface (PD-CONFIG). These steps exercise the read-only scaffold (Phase 2a),
// interactive editing (Phase 2b), and dialogs/pickers/restart flow (Phase 2c).
package surfacesteps

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/llm/provider"
	"agentx/internal/surfaces/config"
)

// configWorld holds state for config surface Godog steps.
type configWorld struct {
	model  *config.ConfigModel
	width  int
	height int
}

// registerConfigSteps wires the config surface Godog steps.
func registerConfigSteps(sc *godog.ScenarioContext) {
	w := &configWorld{}

	sc.Step(`^a config surface sized (\d+) by (\d+)$`, w.sized)
	sc.Step(`^the config surface fetches config with provider "([^"]*)"$`, w.withProvider)
	sc.Step(`^the config surface fetches config with provider "([^"]*)" and ollama_host "([^"]*)"$`, w.withProviderAndOllamaHost)
	sc.Step(`^the config surface fetches config with provider "([^"]*)" and ollama_host "([^"]*)" and llamacpp_host "([^"]*)"$`, w.withProviderAndHosts)
	sc.Step(`^the config surface fails to fetch config$`, w.failFetch)
	sc.Step(`^the config surface receives key "([^"]*)"$`, w.receiveKey)
	sc.Step(`^the config view contains "([^"]*)"$`, w.viewContains)
	sc.Step(`^the config view does not contain "([^"]*)"$`, w.viewNotContains)
}

func (w *configWorld) sized(width, height int) error {
	w.model = config.New(nil, "")
	w.width = width
	w.height = height
	w.model.SetSize(width, height)
	return nil
}

// defaultSchema returns a minimal schema matching the default config keys.
// Phase 2c adds host, model, and color types to the schema.
func defaultSchema() map[string]provider.SchemaField {
	return map[string]provider.SchemaField{
		"agentx.provider": {
			Name:            "Provider",
			Type:            "enum",
			Default:         "ollama",
			ReadOnly:        false,
			Description:     "The LLM backend to use.",
			EnumValues:      []string{"ollama", "llamacpp"},
			RestartRequired: true,
		},
		"agentx.ollama.host": {
			Name:            "Ollama Host",
			Type:            "host",
			Default:         "localhost:11434",
			ReadOnly:        false,
			Description:     "The Ollama host address.",
			RestartRequired: true,
		},
		"agentx.ollama.model": {
			Name:            "Ollama Model",
			Type:            "model",
			Default:         "",
			ReadOnly:        false,
			Description:     "The Ollama model name.",
			RestartRequired: true,
		},
		"agentx.llamacpp.host": {
			Name:            "llama.cpp Host",
			Type:            "host",
			Default:         "localhost:8080",
			ReadOnly:        false,
			Description:     "The llama.cpp host address.",
			RestartRequired: true,
		},
		"agentx.llamacpp.model": {
			Name:            "llama.cpp Model",
			Type:            "model",
			Default:         "",
			ReadOnly:        false,
			Description:     "The llama.cpp model name.",
			RestartRequired: true,
		},
		"agentx.theme.active_border_color": {
			Name:            "Active Border Color",
			Type:            "color",
			Default:         "cyan",
			ReadOnly:        false,
			Description:     "Border color when the surface is active.",
			RestartRequired: false,
		},
		"agentx.theme.inactive_border_color": {
			Name:            "Inactive Border Color",
			Type:            "color",
			Default:         "dark gray",
			ReadOnly:        false,
			Description:     "Border color when the surface is inactive.",
			RestartRequired: false,
		},
	}
}

// seedConfig returns a default config map and schema for test use.
// The config is nested: top-level "agentx" maps to sub-sections
// (ollama, llamacpp, …) which contain the actual keys.
func seedConfig() (map[string]any, map[string]provider.SchemaField) {
	cfg := map[string]any{
		"agentx": map[string]any{
			"provider": "ollama",
			"ollama": map[string]any{
				"host":   "localhost:11434",
				"model":  "llama3.1",
			},
			"llamacpp": map[string]any{
				"host":  "localhost:8080",
				"model": "",
			},
			"theme": map[string]any{
				"active_border_color":   "cyan",
				"inactive_border_color": "dark gray",
			},
		},
	}
	return cfg, defaultSchema()
}

// loadConfig builds a ConfigModel pre-loaded with the given config and schema.
func (w *configWorld) loadConfig(cfg map[string]any, schema map[string]provider.SchemaField) {
	w.model = config.NewFromConfig(cfg, schema, w.width, w.height, nil, "")
}

// withProvider sets up a config with just the given provider value.
func (w *configWorld) withProvider(provider string) error {
	cfg, schema := seedConfig()
	cfg["provider"] = provider
	w.loadConfig(cfg, schema)
	return nil
}

// withProviderAndOllamaHost adds the ollama_host field.
func (w *configWorld) withProviderAndOllamaHost(provider, host string) error {
	cfg, schema := seedConfig()
	cfg["provider"] = provider
	cfg["ollama_host"] = host
	w.loadConfig(cfg, schema)
	return nil
}

// withProviderAndHosts adds both ollama and llamacpp hosts.
func (w *configWorld) withProviderAndHosts(provider, ollamaHost, llamacppHost string) error {
	cfg, schema := seedConfig()
	cfg["provider"] = provider
	cfg["ollama_host"] = ollamaHost
	cfg["llamacpp_host"] = llamacppHost
	w.loadConfig(cfg, schema)
	return nil
}

// failFetch simulates a transport failure: the model has an error set.
func (w *configWorld) failFetch() error {
	m := config.New(nil, "")
	m.Err = fmt.Errorf("connection refused")
	w.model = m
	return nil
}

func (w *configWorld) receiveKey(name string) error {
	if name == "space" {
		w.model.Key(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		return nil
	}
	r := []rune(name)[0]
	w.model.Key(tea.KeyPressMsg{Code: r, Text: name})
	return nil
}

func (w *configWorld) viewContains(want string) error {
	if !strings.Contains(w.model.View(), want) {
		return fmt.Errorf("config view does not contain %q (got: %q)", want, w.model.View())
	}
	return nil
}

func (w *configWorld) viewNotContains(unwanted string) error {
	if strings.Contains(w.model.View(), unwanted) {
		return fmt.Errorf("config view unexpectedly contains %q", unwanted)
	}
	return nil
}
