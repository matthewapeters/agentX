package app

import (
	"context"
	"fmt"
	"time"

	"agentx/internal/config"
	"agentx/internal/runtime"
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
		OllamaHost:  cfg.OllamaHost,
		OllamaModel: cfg.OllamaModel,
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
