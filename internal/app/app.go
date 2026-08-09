package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/config"
	"agentx/internal/runtime"
	"agentx/internal/surfaces"
	"agentx/internal/surfaces/chat"
	transporthttp "agentx/internal/transport/http"
)

// Options configures composition. The zero value uses the conventional config
// locations; tests inject alternates.
type Options struct {
	// Paths overrides the config file locations. Nil uses config.DefaultPaths.
	Paths *config.Paths
	// SessionRoot overrides the session storage root. Empty derives it from Paths.
	SessionRoot string
	// SessionName names the booted session (e.g. from --session). Empty generates
	// the default adjective-noun name.
	SessionName string
	// ResumeSessionID, when non-empty, resumes into that existing session
	// directory instead of creating a fresh one — the already-resolved
	// target from cli.ResolveResume, not the raw --resume flag value (see
	// docs/architecture/behavior/session_resume.feature.md).
	ResumeSessionID string
	// ResolveResumeTarget, if set, is called when the user triggers a
	// mid-session resume switch from inside the running chat surface
	// (ESC,r) with no target pre-selected: it should show a picker (or
	// resolve unambiguously) and return the chosen session ID. Injected
	// from the CLI layer (cmd/agentx) rather than called via a direct
	// internal/cli import — internal/app must not depend on internal/cli
	// per the import-direction matrix
	// (docs/implementation/08_go_module_layout.md) — so this is dependency
	// injection, not a layering exception. Nil (e.g. in a test that never
	// exercises the resume trigger) degrades to an ordinary shutdown if the
	// trigger somehow fires anyway.
	ResolveResumeTarget func() (string, error)
}

// resumePortEnvVar carries the outgoing process's transport port across
// syscall.Exec during a mid-session resume switch, so the new process can
// try to reclaim the exact same port (Settings.PreferredTransportPort) —
// already-attached surfaces' reconnect polling then succeeds against the
// endpoint they already have, rather than needing to discover a changed one
// (docs/architecture/behavior/session_resume.feature.md §5).
const resumePortEnvVar = "AGENTX_RESUME_PORT"

// preferredTransportPort reads resumePortEnvVar, returning 0 (no
// preference — the ordinary case) if it's unset or unparsable.
func preferredTransportPort() int {
	v := os.Getenv(resumePortEnvVar)
	if v == "" {
		return 0
	}
	p, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return p
}

// shutdownTimeout bounds graceful shutdown.
const shutdownTimeout = 5 * time.Second

// modelCheckTimeout bounds the startup model-readiness probe.
const modelCheckTimeout = 5 * time.Second

// Build resolves configuration and returns a started orchestrator.
func Build(opts Options) (*runtime.Orchestrator, error) {
	paths := opts.Paths
	if paths == nil {
		p, err := config.DefaultPaths()
		if err != nil {
			return nil, fmt.Errorf("resolve config paths: %w", err)
		}
		paths = &p
	}

	// Phase 1b: clean up stale config temp files before any writes begin.
	// An orchestrator crash that died mid-write can leave a config_*.tmp in
	// the cache dir; this keeps the cache dir from accumulating orphan temps.
	cp, err := config.DefaultCachePaths()
	if err == nil {
		if n, err := config.CleanupStaleTemps(cp); err != nil {
			return nil, fmt.Errorf("cleanup stale config temps: %w", err)
		} else if n > 0 {
			logStaleTempCleanup(n)
		}
	}

	cfg, _, err := config.Resolve(*paths)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	root := opts.SessionRoot
	if root == "" {
		root = paths.SessionRoot()
	}

	// Optional user prompt files (see docs/implementation/04, Instructions and
	// Bootstrap Prompts). Absent files load as empty.
	instructions, err := config.ReadPromptFile(paths.InstructionsPath())
	if err != nil {
		return nil, err
	}
	bootstrap, err := config.ReadPromptFile(paths.BootstrapPath())
	if err != nil {
		return nil, err
	}
	classification, err := config.ReadPromptFile(paths.ClassificationPath())
	if err != nil {
		return nil, err
	}
	thinkingPrompt, err := config.ReadPromptFile(paths.ThinkingPath())
	if err != nil {
		return nil, err
	}
	toolCatalog, err := config.ReadPromptFile(paths.ShellCommandsPath())
	if err != nil {
		return nil, err
	}
	plannerPrompt, err := config.ReadPromptFile(paths.PlannerPath())
	if err != nil {
		return nil, err
	}
	wavefrontClassifyPrompt, err := config.ReadPromptFile(paths.WavefrontClassifyPath())
	if err != nil {
		return nil, err
	}
	wavefrontSynthesisPrompt, err := config.ReadPromptFile(paths.WavefrontSynthesisPath())
	if err != nil {
		return nil, err
	}
	wavefrontSummaryPrompt, err := config.ReadPromptFile(paths.WavefrontSummaryPath())
	if err != nil {
		return nil, err
	}
	// Optional fan-group prompt corpus (~/.config/agentx/prompts.toml). Absent leaves
	// the experimental task classifier off. The config dir is derived from an existing
	// prompt-file path so no new config accessor is needed.
	promptCorpus, err := config.ReadPromptFile(filepath.Join(filepath.Dir(paths.InstructionsPath()), "prompts.toml"))
	if err != nil {
		return nil, err
	}

	transportStart, transportEnd := cfg.TransportPortRange()

	orc := runtime.New(runtime.Settings{
		SessionRoot:                  root,
		SessionName:                  opts.SessionName,
		Provider:                     cfg.EffectiveProvider(),
		OllamaHost:                   cfg.OllamaHost(),
		OllamaModel:                  cfg.OllamaModel(),
		LlamacppHost:                 cfg.LlamacppHost(),
		LlamacppModel:                cfg.LlamacppModel(),
		Instructions:                 instructions,
		BootstrapPrompt:              bootstrap,
		ClassificationPrompt:         classification,
		ClassificationRetries:        cfg.ClassificationRetries(),
		PromptCorpus:                 promptCorpus,
		PlannerPrompt:                plannerPrompt,
		PlannerThinkingBudget:        time.Duration(cfg.PlannerThinkingBudgetSeconds()) * time.Second,
		WavefrontClassifyPrompt:      wavefrontClassifyPrompt,
		WavefrontSynthesisPrompt:     wavefrontSynthesisPrompt,
		WavefrontSummaryPrompt:       wavefrontSummaryPrompt,
		MaxWidgetLines:               cfg.MaxWidgetLines(),
		InputMaxLines:                cfg.InputMaxLines(),
		MarkdownRenderer:             cfg.MarkdownRenderer(),
		ThinkingEnabled:              cfg.ThinkingEnabled(),
		ThinkingPrompt:               thinkingPrompt,
		ThinkingBudget:               time.Duration(cfg.ThinkingTimeBudgetSeconds()) * time.Second,
		ThinkingRoutes:               cfg.ThinkingRoutes(),
		ActiveBorderColor:            cfg.ActiveBorderColor(),
		InactiveBorderColor:          cfg.InactiveBorderColor(),
		ToolsEnabled:                 cfg.ToolsEnabled(),
		WavefrontEnabled:             cfg.WavefrontEnabled(),
		ToolCatalog:                  toolCatalog,
		ToolBlacklistPath:            paths.ToolBlacklistPath(),
		ToolApprovalsPath:            paths.ToolApprovalsPath(),
		ContinuationVerbsAllowedPath: paths.ContinuationVerbsAllowedPath(),
		ContinuationVerbsDeniedPath:  paths.ContinuationVerbsDeniedPath(),
		ToolOutputMaxBytes:           cfg.ToolOutputMaxBytes(),
		ToolOutputOverridesPath:      paths.ToolOutputOverridesPath(),
		ToolOutputAbsoluteMaxBytes:   cfg.ToolOutputAbsoluteMaxBytes(),
		TransportEnabled:             cfg.TransportEnabled(),
		TransportHost:                cfg.TransportHost(),
		TransportPortStart:           transportStart,
		TransportPortEnd:             transportEnd,
		ResumeSessionID:              opts.ResumeSessionID,
		PreferredTransportPort:       preferredTransportPort(),
	})
	if err := orc.Start(); err != nil {
		return nil, err
	}
	return orc, nil
}

// logStaleTempCleanup logs (via stderr) that stale config temps were removed.
// We deliberately skip log infrastructure here — at startup nothing is wired up
// yet, and the message is informational, not an error.
func logStaleTempCleanup(n int) {
	fmt.Fprintf(os.Stderr, "agentx: cleaned %d stale config temp(s)\n", n)
}

// Run builds the runtime, serves until ctx is canceled, then shuts down.
func Run(ctx context.Context, opts Options) error {
	orc, err := Build(opts)
	if err != nil {
		return err
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return orc.Shutdown(shutdownCtx)
}

// RunChat builds the runtime, verifies the configured model, then runs the
// two-panel chat surface wired to the orchestrator until the user quits (or ctx
// is canceled), and shuts the runtime down. This is the live `agentx` path that
// completes the first vertical slice (CHT-C4).
func RunChat(ctx context.Context, opts Options) error {
	orc, err := Build(opts)
	if err != nil {
		return err
	}
	// shutdownHandled is set true only on the resume-switch path below, once
	// it has already run its own ShutdownForResume — this deferred plain
	// Shutdown must not also run in that case (Shutdown is not designed to
	// be called twice). Every ordinary exit (including every early return
	// below) leaves it false, so the plain shutdown always fires exactly
	// once for those, unchanged from before this field existed.
	shutdownHandled := false
	defer func() {
		if shutdownHandled {
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = orc.Shutdown(shutdownCtx)
	}()

	// Verify the configured model before taking over the terminal, so an
	// unavailable model is reported as a clean CLI error rather than a dead UI.
	checkCtx, cancel := context.WithTimeout(ctx, modelCheckTimeout)
	err = orc.CheckModel(checkCtx)
	cancel()
	if err != nil {
		return err
	}

	// Bridge the surface to the runtime: prompts in, events and processing-state
	// out. The bus subscription and processing feed are closed on return.
	sub := orc.Bus().Subscribe()
	defer sub.Close()
	procCh, procCancel := orc.Processing().Subscribe()
	defer procCancel()

	// Each prompt runs under its own cancelable context so the surface can
	// interrupt the in-flight cycle via Stop without tearing down the program.
	// Expose peer-connection status to the surface only when the transport serves
	// (otherwise there are no peers and the surface skips connection polling).
	var connected func() []string
	if orc.Endpoint() != "" {
		connected = orc.Registry().ConnectedKinds
	}

	// resumeRequested is written from the bubbletea Update goroutine (the
	// bridge callback below) and read only after p.Run() returns, once that
	// goroutine has fully finished — no concurrent access, so no mutex is
	// needed here (unlike cancelPrompt/promptMu below, which are genuinely
	// read and written while the program is still running).
	var resumeRequested bool
	var promptMu sync.Mutex
	var cancelPrompt context.CancelFunc
	bridge := chat.Bridge{
		Submit: func(text string) {
			promptCtx, cancel := context.WithCancel(ctx)
			promptMu.Lock()
			cancelPrompt = cancel
			promptMu.Unlock()
			// Submit streams a full prompt cycle; run it off the UI goroutine.
			go func() {
				defer cancel()
				_ = orc.Submit(promptCtx, text)
			}()
		},
		Stop: func() {
			promptMu.Lock()
			cancel := cancelPrompt
			promptMu.Unlock()
			if cancel != nil {
				cancel()
			}
		},
		Approve:       func(decision string) { orc.Resolve(decision) },
		Events:        sub.C,
		Processing:    procCh,
		Connected:     connected,
		RequestResume: func() { resumeRequested = true },
	}

	// Auto-submit the bootstrap prompt (if configured) once the surface is
	// subscribed; its events buffer on the subscription until the program drains
	// them, so the response opens the session. Skipped when resuming: a resumed
	// session already has its own bootstrap turn from its original run —
	// re-submitting would append a second, redundant greeting on top of the
	// resumed history (harmless to the model's own context, since
	// historyFromEvents already excludes ephemeral events, but a visible
	// wart in the chat surface, which still renders them).
	if orc.Settings().ResumeSessionID == "" {
		go func() { _ = orc.SubmitBootstrap(ctx) }()
	}

	surface := chat.NewWithBridge(bridge)
	surface.SetMaxWidgetLines(orc.Settings().MaxWidgetLines)
	surface.SetMaxInputLines(orc.Settings().InputMaxLines)
	surface.SetMarkdownRenderer(orc.Settings().MarkdownRenderer)
	surface.SetTheme(orc.Settings().ActiveBorderColor, orc.Settings().InactiveBorderColor)

	// When the transport is served, surface the attach commands as a collapsed
	// launch-info widget (the chat runs in the alternate screen, so a printed hint
	// would be wiped). The user expands it and presses a digit to copy a surface's
	// full attach command to the clipboard; the command (and token) is never shown.
	if ep := orc.Endpoint(); ep != "" {
		kinds := surfaces.ExternalKinds()
		name := orc.Session().Name
		header := fmt.Sprintf("🔌 Attach surfaces · session %s · %s · enter to expand", name, ep)
		// Session-named, token-free commands: a same-machine peer auto-resolves the
		// endpoint and token from disk (SS-5); naming the session keeps the command
		// unambiguous when more than one agentx session is running.
		commands := make([]string, len(kinds))
		for i, k := range kinds {
			commands[i] = transporthttp.SessionLaunchCommand(k, name)
		}
		surface.SetLaunchInfo(header, name, kinds, commands)
	}
	p := tea.NewProgram(surface, tea.WithContext(ctx))
	if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return err
	}

	// p.Run() has returned: the terminal is restored from Bubble Tea's
	// alt-screen (same clean-quit path an ordinary ctrl+q already took).
	// Only now — never before the program has actually quit — is it safe to
	// resolve a picker on stdin/stdout and, on success, hand off to a
	// resumed session.
	if resumeRequested && opts.ResolveResumeTarget != nil {
		target, err := opts.ResolveResumeTarget()
		if err != nil {
			// The switch didn't happen (no candidates, invalid selection,
			// etc.) — report it and exit normally via the deferred plain
			// Shutdown, same as any other error return here. Never fall
			// back to silently continuing the just-quit session.
			return err
		}
		shutdownHandled = true
		return switchToResumedSession(orc, target)
	}
	return nil
}

// execFunc abstracts syscall.Exec so switchToResumedSession is testable — a
// real unit test cannot safely exec-replace the test binary's own process
// image, so tests swap this for a fake that records what it was called
// with instead.
var execFunc = syscall.Exec

// switchToResumedSession completes a mid-session resume switch (ESC,r):
// hands the current transport port across the process replacement (via
// resumePortEnvVar) so already-attached surfaces' reconnect polling can
// succeed against the endpoint they already have, shuts down — notifying
// attached surfaces via ShutdownForResume, not the plain Shutdown the
// caller's own deferred cleanup would otherwise run — then execs the same
// binary as `agentx --resume <target>` in place: same PID, same controlling
// terminal, no new process to hand off to or orphan
// (docs/architecture/behavior/session_resume.feature.md §4-5). Only
// returns on failure — a successful exec never returns at all.
func switchToResumedSession(orc *runtime.Orchestrator, target string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve own executable path: %w", err)
	}

	if u, err := url.Parse(orc.Endpoint()); err == nil {
		if port := u.Port(); port != "" {
			_ = os.Setenv(resumePortEnvVar, port)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := orc.ShutdownForResume(shutdownCtx, target); err != nil {
		return fmt.Errorf("shut down for resume: %w", err)
	}

	argv := []string{execPath, "--resume", target}
	return execFunc(execPath, argv, os.Environ())
}
