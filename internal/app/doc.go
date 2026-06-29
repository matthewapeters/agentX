// Package app is the composition root: it resolves configuration, assembles the
// runtime dependency graph, and runs the process until shutdown.
//
// Source contract: docs/implementation/08_go_module_layout.md (internal/app =
// high-level composition) and docs/build-plan/03_chat_surface_backlog.md (CHT-A6).
//
// Build resolves config into runtime.Settings and returns a started
// orchestrator. Run builds, serves until the context is canceled (e.g. on
// SIGINT), then shuts the orchestrator down gracefully. The chat surface is
// attached at CHT-B5; until then Run serves a headless orchestrator.
package app
