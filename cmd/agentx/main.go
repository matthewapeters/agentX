// Command agentx is the single runtime entrypoint for AgentX.
//
// Architecture (target): agentx boots the core orchestration server and the
// human-agent chat surface together. Additional surfaces (files, config,
// context, context-history, context-visualizer, and future surfaces) are
// launched as separate client processes from other terminals and attach to the
// server over the HTTP/SSE transport. See docs/implementation/ for contracts.
//
// The default launch boots the orchestrator and the two-panel chat surface
// together in one process (the first vertical slice; see docs/build-plan/).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"agentx/internal/app"
	"agentx/internal/cli"
	"agentx/internal/config"
	"agentx/internal/session"
)

// version is the build-time version of the agentx runtime. It is overridable
// via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "agentx:", err)
		os.Exit(1)
	}
}

// run is the testable entrypoint body: parse args, then either report version or
// launch the runtime until interrupted.
func run(args []string) error {
	cmd, err := cli.Parse(args)
	if err != nil {
		return err
	}
	if cmd.ShowVersion {
		fmt.Println("agentx", version)
		return nil
	}
	if cmd.GenSessionName {
		fmt.Println(cli.NewSessionName())
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cmd.Launch != nil {
		return cli.RunSurface(ctx, *cmd.Launch)
	}

	opts := app.Options{
		SessionName: cmd.SessionName,
		// Injected regardless of whether this launch itself used --resume:
		// the mid-session ESC,r trigger can fire during any running
		// session, not just one that was itself resumed at launch. Reuses
		// resolveResumeSessionID with an empty (ambiguous) target — the
		// same bare-picker behavior --resume with no value already has —
		// since RunChat only calls this after its own bubbletea program has
		// already quit and the terminal is restored, dependency injection
		// keeps internal/app from needing to import internal/cli directly
		// (docs/implementation/08_go_module_layout.md's import-direction
		// matrix).
		ResolveResumeTarget: func() (string, error) { return resolveResumeSessionID("") },
	}
	if cmd.Resume {
		id, err := resolveResumeSessionID(cmd.ResumeTarget)
		if err != nil {
			return err
		}
		opts.ResumeSessionID = id
	}
	return app.RunChat(ctx, opts)
}

// resolveResumeSessionID resolves a --resume flag's target to a concrete
// session ID before the runtime boots: cli.ResolveResume needs a
// session.Store (built from the conventional session root, same as
// app.Build resolves internally) and interactive stdin/stdout for the
// picker when the target is ambiguous — neither fits app.Build's role as a
// pure composition function, so this runs first, in the CLI layer, same as
// cli.NewSessionName's pre-launch printing above
// (docs/architecture/behavior/session_resume.feature.md §2).
func resolveResumeSessionID(target string) (string, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return "", fmt.Errorf("resolve config paths: %w", err)
	}
	store := session.NewStore(paths.SessionRoot())
	return cli.ResolveResume(store, target, os.Stdin, os.Stdout)
}
