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

	return app.RunChat(ctx, app.Options{Logo: logo, SessionName: cmd.SessionName})
}
