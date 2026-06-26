package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the effective AgentX runtime configuration. Fields mirror the keys in
// agentx.toml (docs/implementation/03_configuration_and_storage.md).
type Config struct {
	OllamaHost  string `toml:"ollama_host"`
	OllamaModel string `toml:"ollama_model"`
}

// Default returns the built-in default configuration used to seed a deployment
// config on first launch.
func Default() Config {
	return Config{
		OllamaHost:  "localhost:11434",
		OllamaModel: "phi4-mini:3.8b",
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
