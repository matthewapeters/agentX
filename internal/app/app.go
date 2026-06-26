package app

import (
	"context"
	"errors"
	"fmt"
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

	orc := runtime.New(runtime.Settings{
		SessionRoot: root,
		OllamaHost:  cfg.OllamaHost(),
		OllamaModel: cfg.OllamaModel(),
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

	bridge := chat.Bridge{
		Submit: func(text string) {
			// Submit streams a full prompt cycle; run it off the UI goroutine.
			go func() { _ = orc.Submit(ctx, text) }()
		},
		Events:     sub.C,
		Processing: procCh,
	}

	p := tea.NewProgram(chat.NewWithBridge(bridge), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return err
	}
	return nil
}
