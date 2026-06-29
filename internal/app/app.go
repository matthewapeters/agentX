package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"agentx/internal/config"
	"agentx/internal/runtime"
	"agentx/internal/surfaces/chat"
)

// Options configures composition. The zero value uses the conventional config
// locations; tests inject alternates.
type Options struct {
	// Paths overrides the config file locations. Nil uses config.DefaultPaths.
	Paths *config.Paths
	// SessionRoot overrides the session storage root. Empty derives it from Paths.
	SessionRoot string
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

	cfg, _, err := config.Resolve(*paths)
	if err != nil {
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

	orc := runtime.New(runtime.Settings{
		SessionRoot:           root,
		OllamaHost:            cfg.OllamaHost(),
		OllamaModel:           cfg.OllamaModel(),
		Instructions:          instructions,
		BootstrapPrompt:       bootstrap,
		ClassificationPrompt:  classification,
		ClassificationRetries: cfg.ClassificationRetries(),
		MaxWidgetLines:        cfg.MaxWidgetLines(),
		ThinkingEnabled:       cfg.ThinkingEnabled(),
		ThinkingPrompt:        thinkingPrompt,
		ThinkingBudget:        time.Duration(cfg.ThinkingTimeBudgetSeconds()) * time.Second,
		ThinkingRoutes:        cfg.ThinkingRoutes(),
		ActiveBorderColor:     cfg.ActiveBorderColor(),
		InactiveBorderColor:   cfg.InactiveBorderColor(),
		ToolsEnabled:          cfg.ToolsEnabled(),
		ToolReadOnly:          cfg.ToolReadOnly(),
		ToolCatalog:           toolCatalog,
	})
	if err := orc.Start(); err != nil {
		return nil, err
	}
	return orc, nil
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
	defer func() {
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
		Approve:    func(decision string) { orc.Resolve(decision) },
		Events:     sub.C,
		Processing: procCh,
	}

	// Auto-submit the bootstrap prompt (if configured) once the surface is
	// subscribed; its events buffer on the subscription until the program drains
	// them, so the response opens the session.
	go func() { _ = orc.SubmitBootstrap(ctx) }()

	surface := chat.NewWithBridge(bridge)
	surface.SetMaxWidgetLines(orc.Settings().MaxWidgetLines)
	surface.SetTheme(orc.Settings().ActiveBorderColor, orc.Settings().InactiveBorderColor)
	p := tea.NewProgram(surface, tea.WithContext(ctx))
	if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return err
	}
	return nil
}
