// Command agentx is the single runtime entrypoint for AgentX.
//
// Architecture (target): agentx boots the core orchestration server and the
// human-agent chat surface together. Additional surfaces (files, config,
// context, context-history, context-visualizer, and future surfaces) are
// launched as separate client processes from other terminals and attach to the
// server over the HTTP/SSE transport. See docs/implementation/ for contracts.
//
// This is a minimal bootstrap stub. The real server + 2-panel chat surface is
// delivered in milestone M1/M2; see docs/build-plan/ and the runtime contracts
// under docs/architecture/runtime_contracts/.
package main

import (
	"fmt"
	"os"
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

// run is the testable entrypoint body. It currently reports version and exits
// cleanly; surface/server bootstrap lands in M1.
func run(args []string) error {
	for _, a := range args {
		switch a {
		case "--version", "-v":
			fmt.Println("agentx", version)
			return nil
		}
	}
	fmt.Println("agentx", version, "- runtime bootstrap not yet implemented (see docs/build-plan/)")
	return nil
}
