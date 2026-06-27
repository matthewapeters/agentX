package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the effective AgentX runtime configuration. It mirrors the nested
// table layout of agentx.toml (docs/implementation/03_configuration_and_storage.md):
//
//	[agentx.ollama]
//	host  = "localhost:11434"
//	model = "phi4-mini:3.8b"
//
// Only the keys the Go runtime consumes are bound; unknown keys in the file
// (timeouts, theme, applet ports, the [tui] section, ...) are ignored by the
// decoder so the config can carry settings for other tools without error.
type Config struct {
	Agentx Agentx `toml:"agentx"`
}

// Agentx is the [agentx] table.
type Agentx struct {
	Ollama Ollama `toml:"ollama"`
}

// Ollama is the [agentx.ollama] table: which local model the runtime drives.
type Ollama struct {
	Host  string `toml:"host"`
	Model string `toml:"model"`
}

// OllamaHost returns the configured Ollama host.
func (c Config) OllamaHost() string { return c.Agentx.Ollama.Host }

// OllamaModel returns the configured active model.
func (c Config) OllamaModel() string { return c.Agentx.Ollama.Model }

// Default returns the built-in default configuration used to seed a deployment
// config on first launch.
func Default() Config {
	return Config{
		Agentx: Agentx{
			Ollama: Ollama{
				Host:  "localhost:11434",
				Model: "phi4-mini:3.8b",
			},
		},
	}
}

// Source identifies where the effective configuration came from.
type Source string

const (
	// SourceDeployment means an existing deployment config was loaded.
	SourceDeployment Source = "deployment"
	// SourceSeeded means no deployment config existed and one was seeded this run.
	SourceSeeded Source = "seeded"
)

// Paths holds the candidate configuration file locations, in precedence order.
type Paths struct {
	// Deployment is the authoritative runtime config (~/.config/agentx/agentx.toml).
	Deployment string
	// Project is the optional project-local default (<cwd>/.agentx/.agentx.toml).
	Project string
}

// SessionRoot returns the session storage root alongside the deployment config
// (conventionally ~/.config/agentx/sessions).
func (p Paths) SessionRoot() string {
	return filepath.Join(filepath.Dir(p.Deployment), "sessions")
}

// configDir is the deployment config directory that holds the user prompt files.
func (p Paths) configDir() string { return filepath.Dir(p.Deployment) }

// InstructionsPath is the standing user-instructions file prefixed to every LLM
// context (~/.config/agentx/agentx-instructions.md).
func (p Paths) InstructionsPath() string {
	return filepath.Join(p.configDir(), "agentx-instructions.md")
}

// BootstrapPath is the startup auto-submit prompt file
// (~/.config/agentx/bootstrap-prompt.md).
func (p Paths) BootstrapPath() string {
	return filepath.Join(p.configDir(), "bootstrap-prompt.md")
}

// ReadPromptFile reads an optional Markdown prompt file, returning its trimmed
// contents, or "" when the file does not exist. See
// docs/implementation/04_llm_prompt_tooling_runtime.md (Instructions and
// Bootstrap Prompts).
func ReadPromptFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read prompt file %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// DefaultPaths derives the conventional configuration locations, honoring
// XDG_CONFIG_HOME for the deployment config.
func DefaultPaths() (Paths, error) {
	cfgHome := os.Getenv("XDG_CONFIG_HOME")
	if cfgHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve home dir: %w", err)
		}
		cfgHome = filepath.Join(home, ".config")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve working dir: %w", err)
	}
	return Paths{
		Deployment: filepath.Join(cfgHome, "agentx", "agentx.toml"),
		Project:    filepath.Join(cwd, ".agentx", ".agentx.toml"),
	}, nil
}

// Resolve returns the effective configuration and its source. The deployment
// config wins when present; otherwise built-in defaults are overlaid with any
// project-local config, seeded to the deployment path, and returned.
func Resolve(p Paths) (Config, Source, error) {
	if fileExists(p.Deployment) {
		cfg := Default()
		if _, err := toml.DecodeFile(p.Deployment, &cfg); err != nil {
			return Config{}, "", fmt.Errorf("load deployment config %s: %w", p.Deployment, err)
		}
		return cfg, SourceDeployment, nil
	}

	cfg := Default()
	if p.Project != "" && fileExists(p.Project) {
		if _, err := toml.DecodeFile(p.Project, &cfg); err != nil {
			return Config{}, "", fmt.Errorf("load project config %s: %w", p.Project, err)
		}
	}
	if err := seed(p.Deployment, cfg); err != nil {
		return Config{}, "", err
	}
	return cfg, SourceSeeded, nil
}

// seed writes cfg to path, creating parent directories as needed.
func seed(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir for %s: %w", path, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config %s: %w", path, err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
