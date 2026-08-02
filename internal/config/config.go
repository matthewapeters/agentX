package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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
	Provider       string       `toml:"provider"`
	ChatBackend    string       `toml:"chat_backend"` // deprecated alias for Provider
	Ollama         Ollama       `toml:"ollama"`
	Llamacpp       Llamacpp     `toml:"llamacpp"`
	Classification Classification `toml:"classification"`
	Output         Output         `toml:"output"`
	Theme          Theme          `toml:"theme"`
	Thinking       Thinking       `toml:"thinking"`
	Tools          Tools          `toml:"tools"`
	Transport      Transport      `toml:"transport"`
	Wavefront      Wavefront      `toml:"wavefront"`
}

// Wavefront is the [agentx.wavefront] table (ADR 0012). Enabled is a pointer so an
// absent key defaults off — an experimental second decomposition engine should
// never activate silently for an existing deployment.
type Wavefront struct {
	Enabled *bool `toml:"enabled"`
}

// Transport is the [agentx.transport] table configuring the HTTP/SSE endpoint
// external surfaces attach to. Enabled is a pointer so an absent key defaults on.
// Host is loopback-only in v1; [PortStart, PortEnd] is the inclusive candidate
// range the allocator binds the first free port from.
type Transport struct {
	Enabled   *bool  `toml:"enabled"`
	Host      string `toml:"host"`
	PortStart int    `toml:"port_start"`
	PortEnd   int    `toml:"port_end"`
}

// Tools is the [agentx.tools] table gating the single_tool execution cycle.
// Enabled is a pointer so an absent key takes its default.
type Tools struct {
	Enabled        *bool `toml:"enabled"`
	TimeoutSeconds int   `toml:"timeout_seconds"`
	OutputMaxBytes int   `toml:"output_max_bytes"`
	// AbsoluteMaxBytes bounds the oversized-output recovery gate's "capture more"
	// choice (TOOL-6): no interactive choice or remembered per-tool preference can
	// ask for more than this, however large the tool's real output turns out to be.
	AbsoluteMaxBytes int `toml:"absolute_max_bytes"`
}

// Thinking is the [agentx.thinking] table. Enabled requests model reasoning
// during the respond phase (pointer so an absent key defaults to on);
// TimeBudgetSeconds bounds the thinking phase before the runtime falls back to a
// direct answer; Routes enables thinking per classification route.
type Thinking struct {
	Enabled           *bool          `toml:"enabled"`
	TimeBudgetSeconds int            `toml:"time_budget_seconds"`
	Routes            ThinkingRoutes `toml:"routes"`
	// PlannerTimeBudgetSeconds bounds the decomposition planner's own Complete-based
	// reasoning phase (ADR 0012 Phase 1), independent of TimeBudgetSeconds above (which
	// governs the streaming respond path only). <= 0 disables thinking for planner
	// Complete calls entirely — the default, so existing behavior is unchanged unless
	// explicitly configured.
	PlannerTimeBudgetSeconds int `toml:"planner_time_budget_seconds"`
}

// ThinkingRoutes enables/disables thinking per classification route (pointers so
// absent keys take the per-route defaults: respond_directly off, others on).
type ThinkingRoutes struct {
	RespondDirectly *bool `toml:"respond_directly"`
	SingleTool      *bool `toml:"single_tool"`
	InvokePlanner   *bool `toml:"invoke_planner"`
}

// Ollama is the [agentx.ollama] table: which local model the runtime drives.
type Ollama struct {
	Host  string `toml:"host"`
	Model string `toml:"model"`
}

// Llamacpp is the [agentx.llamacpp] table: which llama.cpp server instance to use.
type Llamacpp struct {
	Host  string `toml:"host"`
	Model string `toml:"model"`
}

// Classification is the [agentx.classification] table tuning the classify cycle.
type Classification struct {
	Retries              int `toml:"retries"`
	ClarificationOptions int `toml:"clarification_options"`
}

// Output is the [agentx.output] table tuning the output panel widgets.
// MarkdownRenderer selects how a finalized assistant answer is styled: "native" (the
// default — scanner prose plus lipgloss/table tables and chroma-highlighted code) or
// "scanner" (the lightweight per-line scanner alone, also used live while streaming).
// See ADR 0007.
type Output struct {
	MaxWidgetLines   int    `toml:"max_widget_lines"`
	InputMaxLines    int    `toml:"input_max_lines"`
	MarkdownRenderer string `toml:"markdown_renderer"`
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

// Provider returns the configured provider name: "ollama", "llamacpp", or
// "" if unset (caller should default to "ollama").
func (c Config) Provider() string {
	// chat_backend is a deprecated alias for backward compatibility.
	if p := strings.TrimSpace(c.Agentx.Provider); p != "" {
		return strings.ToLower(p)
	}
	if b := strings.TrimSpace(c.Agentx.ChatBackend); b != "" {
		return strings.ToLower(b)
	}
	return ""
}

// EffectiveProvider returns the provider name with the default ("ollama") applied.
func (c Config) EffectiveProvider() string {
	if p := c.Provider(); p != "" {
		return p
	}
	return "ollama"
}

// NormalizedKey records a single key that was normalized from a deprecated form
// to its canonical name. Returned by Normalize so callers (the transport layer,
// the config surface) can surface which keys were rewritten and warn the user.
type NormalizedKey struct {
	// Old is the deprecated key name (e.g. "chat_backend").
	Old string
	// New is the canonical key name (e.g. "provider").
	New string
}

// Normalize mutates cfg in place, replacing deprecated key aliases with their
// canonical equivalents. It returns the list of normalizations applied so the
// caller can report them back to the user (the PD-CONFIG spec requires a
// warning when the user is editing a deprecated key).
//
// Current rules:
//   - chat_backend → provider: if Provider is empty and ChatBackend is set,
//     copy ChatBackend into Provider.
//
// The function does NOT write to disk; callers should call WriteConfig after
// Normalize if the change should persist.
func (c *Config) Normalize() []NormalizedKey {
	var normalized []NormalizedKey

	// chat_backend → provider.
	if c.Agentx.Provider == "" && strings.TrimSpace(c.Agentx.ChatBackend) != "" {
		c.Agentx.Provider = strings.TrimSpace(c.Agentx.ChatBackend)
		normalized = append(normalized, NormalizedKey{Old: "chat_backend", New: "provider"})
	}

	return normalized
}

// HasDeprecatedKeys reports whether the config still contains a deprecated key
// that has not yet been normalized. Used by the config surface to warn the
// user when they are editing a deprecated key (PD-CONFIG-AF-011).
func (c *Config) HasDeprecatedKeys() bool {
	return c.Agentx.Provider == "" && strings.TrimSpace(c.Agentx.ChatBackend) != ""
}

// Validate checks the configuration for logical errors that the TOML decoder
// cannot catch: unknown provider names, missing model/host for the chosen
// backend, and so on. It returns a human-readable error that names the
// offending key and, where feasible, lists the valid choices.
//
// A nil return means the config is ready to use; callers should call this
// immediately after Resolve before starting the orchestrator.
func (c Config) Validate() error {
	switch c.EffectiveProvider() {
	case "ollama":
		if strings.TrimSpace(c.Agentx.Ollama.Model) == "" {
			return fmt.Errorf(`BAD CONFIGURATION FOR "[agentx.ollama].model". Model name is required when provider = "ollama"`)
		}
	case "llamacpp":
		if strings.TrimSpace(c.Agentx.Llamacpp.Host) == "" {
			return fmt.Errorf(`BAD CONFIGURATION FOR "[agentx.llamacpp].host". Host (host:port) is required when provider = "llamacpp"`)
		}
		if strings.TrimSpace(c.Agentx.Llamacpp.Model) == "" {
			return fmt.Errorf(`BAD CONFIGURATION FOR "[agentx.llamacpp].model". Model name is required when provider = "llamacpp"`)
		}
	default:
		p := c.Provider()
		if p == "" {
			p = "(unset)"
		}
		return fmt.Errorf(`BAD CONFIGURATION FOR "provider". %q is invalid. Must be one of "ollama", "llamacpp"`, p)
	}
	return nil
}

// LlamacppHost returns the configured llama.cpp server host.
func (c Config) LlamacppHost() string { return c.Agentx.Llamacpp.Host }

// LlamacppModel returns the configured llama.cpp model name.
func (c Config) LlamacppModel() string { return c.Agentx.Llamacpp.Model }

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

// InputMaxLines returns the max rows the input panel grows to before it scrolls.
func (c Config) InputMaxLines() int {
	if c.Agentx.Output.InputMaxLines <= 0 {
		return defaultInputMaxLines
	}
	return c.Agentx.Output.InputMaxLines
}

// MarkdownRenderer returns the assistant-markdown rendering mode: "native" (the
// default) renders prose with the per-line scanner plus GFM tables (lipgloss/table,
// bordered + zebra) and chroma-highlighted code; "scanner" is the lightweight per-line
// scanner alone. An explicit "scanner" opts out; every other value (empty, the retired
// "glamour", or unrecognized) resolves to the native default. See ADR 0007.
func (c Config) MarkdownRenderer() string {
	if strings.EqualFold(strings.TrimSpace(c.Agentx.Output.MarkdownRenderer), "scanner") {
		return "scanner"
	}
	return defaultMarkdownRenderer
}

// ToolsEnabled reports whether the single_tool execution cycle is on (default on).
func (c Config) ToolsEnabled() bool { return boolOr(c.Agentx.Tools.Enabled, true) }

// WavefrontEnabled reports whether ADR 0012's round-free decomposition engine
// drains invoke_planner plans instead of the continuous engine (default off).
func (c Config) WavefrontEnabled() bool { return boolOr(c.Agentx.Wavefront.Enabled, false) }

// ToolOutputMaxBytes is the captured-output cap before truncation (default 65536).
func (c Config) ToolOutputMaxBytes() int {
	if c.Agentx.Tools.OutputMaxBytes > 0 {
		return c.Agentx.Tools.OutputMaxBytes
	}
	return 65536
}

// ToolOutputAbsoluteMaxBytes is the hard ceiling the oversized-output recovery
// gate's "capture more" choice (once or remembered) is clamped to (default 2 MiB —
// large enough for almost any legitimate command's real output, small enough to
// bound worst-case memory/context impact). See TOOL-6.
func (c Config) ToolOutputAbsoluteMaxBytes() int {
	if c.Agentx.Tools.AbsoluteMaxBytes > 0 {
		return c.Agentx.Tools.AbsoluteMaxBytes
	}
	return 2097152
}

// ThinkingEnabled reports whether the respond phase requests model reasoning.
// Absent config defaults to true.
func (c Config) ThinkingEnabled() bool {
	if c.Agentx.Thinking.Enabled == nil {
		return true
	}
	return *c.Agentx.Thinking.Enabled
}

// ThinkingTimeBudgetSeconds returns the wall-clock cap on the thinking phase
// before the runtime falls back to a direct answer (default 180s; <=0 disables).
func (c Config) ThinkingTimeBudgetSeconds() int {
	if c.Agentx.Thinking.TimeBudgetSeconds == 0 {
		return defaultThinkingBudgetSeconds
	}
	return c.Agentx.Thinking.TimeBudgetSeconds
}

// PlannerThinkingBudgetSeconds bounds the decomposition planner's own Complete-based
// reasoning phase (ADR 0012 Phase 1), independent of ThinkingTimeBudgetSeconds. Unlike
// that method, there is no positive default here: <= 0 (including an absent config key)
// disables thinking for planner Complete calls entirely, so existing behavior is
// byte-identical until an operator opts in explicitly.
func (c Config) PlannerThinkingBudgetSeconds() int {
	return c.Agentx.Thinking.PlannerTimeBudgetSeconds
}

// ThinkingRoutes returns the per-route thinking enables, resolved against the
// defaults (respond_directly off; single_tool and invoke_planner on).
func (c Config) ThinkingRoutes() map[string]bool {
	r := c.Agentx.Thinking.Routes
	return map[string]bool{
		"respond_directly": boolOr(r.RespondDirectly, false),
		"single_tool":      boolOr(r.SingleTool, true),
		"invoke_planner":   boolOr(r.InvokePlanner, true),
	}
}

// TransportEnabled reports whether the HTTP/SSE transport server is served
// alongside the in-process chat surface (default on).
func (c Config) TransportEnabled() bool { return boolOr(c.Agentx.Transport.Enabled, true) }

// TransportHost returns the loopback host the transport binds (default 127.0.0.1).
func (c Config) TransportHost() string {
	if h := strings.TrimSpace(c.Agentx.Transport.Host); h != "" {
		return h
	}
	return defaultTransportHost
}

// TransportPortRange returns the inclusive [start, end] candidate port range,
// falling back to the built-in range when unset or invalid.
func (c Config) TransportPortRange() (int, int) {
	start, end := c.Agentx.Transport.PortStart, c.Agentx.Transport.PortEnd
	if start <= 0 || end <= 0 || end < start {
		return defaultTransportPortStart, defaultTransportPortEnd
	}
	return start, end
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
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

func boolPtr(b bool) *bool { return &b }

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
	"black":       "38;5;0",
	"red":         "38;5;1",
	"green":       "38;5;2",
	"yellow":      "38;5;3",
	"blue":        "38;5;4",
	"magenta":     "38;5;5",
	"cyan":        "38;5;6",
	"white":       "38;5;7",
	"bright cyan": "38;5;14",
	"brightcyan":  "38;5;14",
	"gray":        "38;5;245",
	"grey":        "38;5;245",
	"dark gray":   "38;5;240",
	"darkgray":    "38;5;240",
	"dark grey":   "38;5;240",
	"darkgrey":    "38;5;240",
}

// IsProviderDeprecated reports whether the config still contains a deprecated
// key (chat_backend). A non-empty chat_backend is deprecated regardless of
// whether provider has been set — Normalize() copies it to provider but does
// not clear the source field (PD-CONFIG-AF-011).
func (c Config) IsProviderDeprecated() bool {
	return strings.TrimSpace(c.Agentx.ChatBackend) != ""
}
const (
	defaultClassificationRetries = 2
	defaultClarificationOptions  = 3
	defaultMaxWidgetLines        = 20
	defaultInputMaxLines         = 8
	defaultMarkdownRenderer      = "native"
	defaultActiveBorder          = "cyan"
	defaultInactiveBorder        = "dark gray"
	defaultThinkingBudgetSeconds = 180
	defaultTransportHost         = "127.0.0.1"
	defaultTransportPortStart    = 8420
	defaultTransportPortEnd      = 8460
)

// Default returns the built-in default configuration used to seed a deployment
// config on first launch.
func Default() Config {
	return Config{
		Agentx: Agentx{
			Provider: "ollama",
			Ollama: Ollama{
				Host:  "localhost:11434",
				Model: "phi4-mini:3.8b",
			},
			Classification: Classification{
				Retries:              defaultClassificationRetries,
				ClarificationOptions: defaultClarificationOptions,
			},
			Output: Output{MaxWidgetLines: defaultMaxWidgetLines, InputMaxLines: defaultInputMaxLines, MarkdownRenderer: defaultMarkdownRenderer},
			Thinking: Thinking{
				Enabled:           boolPtr(true),
				TimeBudgetSeconds: defaultThinkingBudgetSeconds,
				Routes: ThinkingRoutes{
					RespondDirectly: boolPtr(false),
					SingleTool:      boolPtr(true),
					InvokePlanner:   boolPtr(true),
				},
			},
			Theme: Theme{
				ActiveBorder:   defaultActiveBorder,
				InactiveBorder: defaultInactiveBorder,
			},
			Tools: Tools{
				Enabled:          boolPtr(true),
				TimeoutSeconds:   30,
				OutputMaxBytes:   65536,
				AbsoluteMaxBytes: 2097152,
			},
			Transport: Transport{
				Enabled:   boolPtr(true),
				Host:      defaultTransportHost,
				PortStart: defaultTransportPortStart,
				PortEnd:   defaultTransportPortEnd,
			},
			Wavefront: Wavefront{
				Enabled: boolPtr(false),
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

// CachePaths holds the cache-location metadata for config-write coordination
// (Phase 1b). The lock file guards concurrent writes so two orchestrator
// instances (or two processes racing on seed) never interleave their partial
// bytes onto the deployment config.
//
// Convention:
//
	// CacheDir   = $XDG_CACHE_HOME/agentx/  (default ~/.cache/agentx/)
	// LockFile   = $CacheDir/config.lock
	// TempPattern= $CacheDir/config_*.tmp    (stale temps cleaned at startup)
type CachePaths struct {
	// CacheDir is the cache directory used for the config semaphore and temp
	// file staging area. Honors XDG_CACHE_HOME, falling back to ~/.cache.
	CacheDir string
}

// LockFile returns the path of the config-write semaphore file.
func (c CachePaths) LockFile() string {
	return filepath.Join(c.CacheDir, "config.lock")
}

// TempPattern returns the glob pattern used to find stale temp files at cleanup
// time (e.g. "config_*.tmp").
func (c CachePaths) TempPattern() string {
	return filepath.Join(c.CacheDir, "config_*.tmp")
}

// DefaultCachePaths resolves the conventional cache paths, honoring
// XDG_CACHE_HOME.
func DefaultCachePaths() (CachePaths, error) {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return CachePaths{}, fmt.Errorf("resolve home dir: %w", err)
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	return CachePaths{CacheDir: filepath.Join(cacheHome, "agentx")}, nil
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

// ThinkingPath is the thinking-guidance system-prompt file
// (~/.config/agentx/agentx-thinking.md).
func (p Paths) ThinkingPath() string {
	return filepath.Join(p.configDir(), "agentx-thinking.md")
}

// ShellCommandsPath is the LLM-facing tool catalog file
// (~/.config/agentx/agentx-shell-commands.md).
func (p Paths) ShellCommandsPath() string {
	return filepath.Join(p.configDir(), "agentx-shell-commands.md")
}

// PlannerPath is the decomposition-planner system-prompt file
// (~/.config/agentx/agentx-planner.md) — externalized so the Step/Task decomposition
// rules can be tuned without a rebuild (ADR 0008 amendment).
func (p Paths) PlannerPath() string {
	return filepath.Join(p.configDir(), "agentx-planner.md")
}

// WavefrontClassifyPath is the wavefront classify system-prompt file
// (~/.config/agentx/agentx-wavefront-classify.md) — ADR 0012 Phase 2.
func (p Paths) WavefrontClassifyPath() string {
	return filepath.Join(p.configDir(), "agentx-wavefront-classify.md")
}

// WavefrontSynthesisPath is the wavefront synthesis system-prompt file
// (~/.config/agentx/agentx-wavefront-synthesis.md) — ADR 0012 Phase 2.
func (p Paths) WavefrontSynthesisPath() string {
	return filepath.Join(p.configDir(), "agentx-wavefront-synthesis.md")
}

// WavefrontSummaryPath is the wavefront output-summarization system-prompt file
// (~/.config/agentx/agentx-wavefront-summary.md) — ADR 0012 Phase 2.
func (p Paths) WavefrontSummaryPath() string {
	return filepath.Join(p.configDir(), "agentx-wavefront-summary.md")
}

// ToolBlacklistPath is the persisted command-policy blacklist
// (~/.config/agentx/agentx-tool-blacklist.toml).
func (p Paths) ToolBlacklistPath() string {
	return filepath.Join(p.configDir(), "agentx-tool-blacklist.toml")
}

// ToolApprovalsPath is the persisted global approval whitelist, written when a
// command is approved globally (~/.config/agentx/agentx-tool-approvals.toml).
func (p Paths) ToolApprovalsPath() string {
	return filepath.Join(p.configDir(), "agentx-tool-approvals.toml")
}

// ContinuationVerbsAllowedPath is the allow-list of verbs that, ending an agent
// response as "Let me <verb> ..." / "Should I <verb> ...?" / "Shall I <verb> ...?",
// trigger one more bounded, grounded investigation round instead of silently ending
// the turn on a stated intent (~/.config/agentx/agentx-continuation-verbs-allowed.md).
// Approving an unrecognized verb "always" appends it here.
func (p Paths) ContinuationVerbsAllowedPath() string {
	return filepath.Join(p.configDir(), "agentx-continuation-verbs-allowed.md")
}

// ContinuationVerbsDeniedPath is the deny-list counterpart: a verb here is
// recognized but deliberately not treated as a continuation trigger, and the
// surface does not ask about it again
// (~/.config/agentx/agentx-continuation-verbs-denied.md). Starts empty.
func (p Paths) ContinuationVerbsDeniedPath() string {
	return filepath.Join(p.configDir(), "agentx-continuation-verbs-denied.md")
}

// ToolOutputOverridesPath is the persisted per-tool oversized-output recovery
// decisions (TOOL-6): an "always" choice on the output-size prompt is remembered
// here so future truncations from that tool skip the prompt
// (~/.config/agentx/agentx-tool-output-overrides.toml).
func (p Paths) ToolOutputOverridesPath() string {
	return filepath.Join(p.configDir(), "agentx-tool-output-overrides.toml")
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
	cp, err := DefaultCachePaths()
	if err != nil {
		return Config{}, "", err
	}
	if err := seed(p.Deployment, cfg, cp); err != nil {
		return Config{}, "", err
	}
	return cfg, SourceSeeded, nil
}

// ResolveWithCache is the Resolve variant that takes an explicit CachePaths so
// callers and tests can point the semaphore at a known temp location instead of
// the real user cache dir.
func ResolveWithCache(p Paths, cp CachePaths) (Config, Source, error) {
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
	if err := seed(p.Deployment, cfg, cp); err != nil {
		return Config{}, "", err
	}
	return cfg, SourceSeeded, nil
}

// seed writes cfg to path, creating parent directories and the config-cache
// semaphore directory as needed, then performs an atomic temp+rename write
// guarded by the semaphore lock.
func seed(path string, cfg Config, cp CachePaths) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir for %s: %w", path, err)
	}
	if err := os.MkdirAll(cp.CacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir for %s: %w", cp.CacheDir, err)
	}
	if err := writeConfigAtomic(path, cfg, cp); err != nil {
		return err
	}
	return nil
}

// writeConfigAtomic encodes cfg to TOML, writes to a timestamped temp file
// in the cache dir, then atomically renames it over dst. The caller must hold
// the semaphore lock (see LockConfig/UnlockConfig).
//
// If the cache dir and dst are on different filesystems (os.Rename reports
// "invalid cross-device link"), we fall back to copying the temp into dst's
// parent dir and then renaming — the copy+rename keeps the same atomic
// guarantee on the local filesystem.
func writeConfigAtomic(dst string, cfg Config, cp CachePaths) error {
	buf, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config %s: %w", dst, err)
	}
	// Stamp the temp name with a monotonic timestamp so stale temps sort by
	// age and cleanup can purge them in one glob pass. The cache dir is the
	// staging area — it lives on the same filesystem as the deployment config
	// when XDG_CACHE_HOME is on the user's home mount, so os.Rename is atomic.
	tmp := filepath.Join(cp.CacheDir, "config_"+timestampSuffix()+".tmp")
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return fmt.Errorf("write temp config %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		// Cross-device fallback: copy into dst's parent, then rename locally.
		if isCrossDevice(err) {
			if cerr := copyTempToLocal(tmp, dst, cp); cerr != nil {
				// If copy failed, best-effort cleanup and return the original
				// rename error so the root cause is surfaced.
				_ = os.Remove(tmp)
				return fmt.Errorf("replace config %s: %w", dst, err)
			}
			return nil
		}
		// Non-cross-device error — clean up and return.
		_ = os.Remove(tmp)
		return fmt.Errorf("replace config %s: %w", dst, err)
	}
	return nil
}

// copyTempToLocal copies the staged temp file into dst's parent directory and
// atomically renames it over dst. The temp is removed on success.
func copyTempToLocal(tmp, dst string, cp CachePaths) error {
	localTmp := filepath.Join(filepath.Dir(dst), filepath.Base(tmp))
	if err := copyFile(tmp, localTmp); err != nil {
		return fmt.Errorf("copy temp to local %s: %w", localTmp, err)
	}
	if err := os.Rename(localTmp, dst); err != nil {
		_ = os.Remove(localTmp)
		return fmt.Errorf("replace local %s: %w", dst, err)
	}
	// Best-effort cleanup of the cross-device staging copy.
	_ = os.Remove(tmp)
	return nil
}

// copyFile copies src to dst byte-for-byte (no rename semantics — callers that
// need atomicity use copyTempToLocal).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// isCrossDevice reports whether err is an "invalid cross-device link" error.
// The exact sentinel varies by OS — Linux uses syscall.EXDEV, Darwin uses
// errno 18. We inspect both.
func isCrossDevice(err error) bool {
	if err == nil {
		return false
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EXDEV
	}
	return strings.Contains(err.Error(), "cross-device link")
}

// timestampSuffix returns a short monotonic timestamp suitable for temp-file
// naming. Millisecond precision is plenty to order writes within a single
// session and keeps the filename short.
func timestampSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixMilli())
}

// LockConfig acquires the config-write semaphore. The lock is a best-effort
// advisory lock using a single-file "mutex" in the cache dir: only one process
// can write config.lock at a time, so a second concurrent write blocks until the
// first completes. We use file locking (syscall.Flock) for portability across
// POSIX systems; the lock file lives at ~/.cache/agentx/config.lock.
//
// LockConfig returns an *Unlocker that must be called exactly once to release
// the lock. Callers should defer unlock() immediately after a successful lock.
func LockConfig(cp CachePaths) (*Unlocker, error) {
	if err := os.MkdirAll(cp.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir %s: %w", cp.CacheDir, err)
	}
	f, err := os.OpenFile(cp.LockFile(), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", cp.LockFile(), err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock %s: %w", cp.LockFile(), err)
	}
	// Write the PID so stale locks can be diagnosed.
	_, _ = f.Write([]byte(fmt.Sprintf("%d %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))))
	_, _ = f.Seek(0, 0)
	return &Unlocker{f: f}, nil
}

// Unlocker releases a config-write lock.
type Unlocker struct {
	f *os.File
}

// Unlock releases the lock and closes the lock file.
func (u *Unlocker) Unlock() error {
	if u.f == nil {
		return nil
	}
	err := syscall.Flock(int(u.f.Fd()), syscall.LOCK_UN)
	closeErr := u.f.Close()
	u.f = nil
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close lock file: %w", closeErr)
	}
	return nil
}

// UnlockConfig is the legacy unlocked release form kept for callers that do not
// need the Unlocker return value. Prefer LockConfig's returned Unlocker.
func UnlockConfig(u *Unlocker) error {
	if u == nil {
		return nil
	}
	return u.Unlock()
}

// WriteConfig writes cfg to dst atomically under the config semaphore. It is the
// public write entry point used by the config surface (Phase 1b).
//
// WriteConfig:
//   1. Creates the cache dir (~/.cache/agentx/) if absent.
//   2. Acquires the semaphore lock.
//   3. Writes cfg to dst via temp file + atomic rename in the cache staging dir.
//   4. Releases the lock.
//
// If the lock cannot be acquired (another writer is in progress), WriteConfig
// returns an error so the caller can retry or abort.
func WriteConfig(cp CachePaths, dst string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create config dir for %s: %w", dst, err)
	}
	lock, err := LockConfig(cp)
	if err != nil {
		return err
	}
	defer lock.Unlock()
	return writeConfigAtomic(dst, cfg, cp)
}

// CleanupStaleTemps removes any temp files matching the cache-dir glob
// (config_*.tmp). Called at orchestrator startup so a crash that left a temp
// behind does not leave orphan files accumulating. It returns the number of
// files removed so callers can log or assert.
//
// The cleanup is best-effort: if a glob fails (e.g. the cache dir does not
// exist), we return 0 with a nil error. Only removal failures are reported.
func CleanupStaleTemps(cp CachePaths) (int, error) {
	matches, err := filepath.Glob(cp.TempPattern())
	if err != nil {
		// Bad glob — nothing we can do; treat as zero removals.
		return 0, nil
	}
	if len(matches) == 0 {
		return 0, nil
	}
	removed := 0
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			return removed, fmt.Errorf("remove stale temp %s: %w", m, err)
		}
		removed++
	}
	return removed, nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
