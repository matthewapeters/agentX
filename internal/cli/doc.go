// Package cli parses the agentx command line into a structured command.
//
// Source contract: docs/implementation/02_surface_orchestration_http.md
// (CLI launch contract) and docs/build-plan/03_chat_surface_backlog.md (CHT-A6).
// For this vertical slice only the default launch and --version are handled;
// surface-launch subcommands are deferred with the external-surface transport.
package cli
