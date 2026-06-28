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
	Ollama         Ollama         `toml:"ollama"`
	Classification Classification `toml:"classification"`
	Output         Output         `toml:"output"`
	Theme          Theme          `toml:"theme"`
}

// Ollama is the [agentx.ollama] table: which local model the runtime drives.
type Ollama struct {
	Host  string `toml:"host"`
	Model string `toml:"model"`
}

// Classification is the [agentx.classification] table tuning the classify cycle.
type Classification struct {
	Retries              int `toml:"retries"`
	ClarificationOptions int `toml:"clarification_options"`
}

// Output is the [agentx.output] table tuning the output panel widgets.
type Output struct {
	MaxWidgetLines int `toml:"max_widget_lines"`
}

// Theme is the [agentx.theme] table styling the chat surface. Colors accept a
// CSS-ish name ("cyan", "dark gray"), an ANSI 256 index ("240"), or a hex value
// ("#00afaf"). The active color marks the focused panel (rendered bold) and the
// selected output widget; the inactive color marks everything else.
type Theme struct {
	ActiveBorder   string `toml:"active_border_color"`
	InactiveBorder string `toml:"inactive_border_color"`
}

// OllamaHost returns the configured Ollama host.
func (c Config) OllamaHost() string { return c.Agentx.Ollama.Host }

// OllamaModel returns the configured active model.
func (c Config) OllamaModel() string { return c.Agentx.Ollama.Model }

// ClassificationRetries returns the classify-cycle retry budget (>= 0).
func (c Config) ClassificationRetries() int {
	if c.Agentx.Classification.Retries < 0 {
		return defaultClassificationRetries
	}
	return c.Agentx.Classification.Retries
}

// ClarificationOptions returns the number of interpretations offered on ambiguity.
func (c Config) ClarificationOptions() int {
	if c.Agentx.Classification.ClarificationOptions <= 0 {
		return defaultClarificationOptions
	}
	return c.Agentx.Classification.ClarificationOptions
}

// MaxWidgetLines returns the max body rows before an output widget scrolls.
func (c Config) MaxWidgetLines() int {
	if c.Agentx.Output.MaxWidgetLines <= 0 {
		return defaultMaxWidgetLines
	}
	return c.Agentx.Output.MaxWidgetLines
}

// ActiveBorderColor returns the SGR foreground parameters for the focused-panel
// and selected-widget borders (e.g. "38;5;6"). Empty config falls back to cyan.
func (c Config) ActiveBorderColor() string {
	return resolveColor(c.Agentx.Theme.ActiveBorder, defaultActiveBorder)
}

// InactiveBorderColor returns the SGR foreground parameters for unfocused panels
// and unselected widgets. Empty config falls back to dark gray.
func (c Config) InactiveBorderColor() string {
	return resolveColor(c.Agentx.Theme.InactiveBorder, defaultInactiveBorder)
}

// resolveColor maps a configured color (name, ANSI index, or hex) to its SGR
// foreground parameter string. Unrecognized values fall back to def.
func resolveColor(name, def string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return namedColors[def]
	}
	if sgr, ok := namedColors[n]; ok {
		return sgr
	}
	if strings.HasPrefix(n, "#") && len(n) == 7 {
		var r, g, b int
		if _, err := fmt.Sscanf(n, "#%02x%02x%02x", &r, &g, &b); err == nil {
			return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
		}
	}
	if isAllDigits(n) {
		return "38;5;" + n
	}
	return namedColors[def]
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// namedColors maps friendly color names to SGR foreground parameters (ANSI 256).
var namedColors = map[string]string{
	"black":        "38;5;0",
	"red":          "38;5;1",
	"green":        "38;5;2",
	"yellow":       "38;5;3",
	"blue":         "38;5;4",
	"magenta":      "38;5;5",
	"cyan":         "38;5;6",
	"white":        "38;5;7",
	"bright cyan":  "38;5;14",
	"brightcyan":   "38;5;14",
	"gray":         "38;5;245",
	"grey":         "38;5;245",
	"dark gray":    "38;5;240",
	"darkgray":     "38;5;240",
	"dark grey":    "38;5;240",
	"darkgrey":     "38;5;240",
}

// Built-in defaults for the tuning tables.
const (
	defaultClassificationRetries = 2
	defaultClarificationOptions  = 3
	defaultMaxWidgetLines        = 20
	defaultActiveBorder          = "cyan"
	defaultInactiveBorder        = "dark gray"
)

// Default returns the built-in default configuration used to seed a deployment
// config on first launch.
func Default() Config {
	return Config{
		Agentx: Agentx{
			Ollama: Ollama{
				Host:  "localhost:11434",
				Model: "phi4-mini:3.8b",
			},
			Classification: Classification{
				Retries:              defaultClassificationRetries,
				ClarificationOptions: defaultClarificationOptions,
			},
			Output: Output{MaxWidgetLines: defaultMaxWidgetLines},
			Theme: Theme{
				ActiveBorder:   defaultActiveBorder,
				InactiveBorder: defaultInactiveBorder,
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

// ClassificationPath is the classification system-prompt file
// (~/.config/agentx/agentx-classification.md).
func (p Paths) ClassificationPath() string {
	return filepath.Join(p.configDir(), "agentx-classification.md")
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
