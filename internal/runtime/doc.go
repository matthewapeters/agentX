// Package runtime is the AgentX orchestrator: it coordinates configuration,
// session identity, the event bus, the processing-state feed, and event
// persistence across one process lifetime.
//
// Source contract: docs/implementation/01_runtime_blueprint.md (Runtime
// Lifecycle). Backlog task: CHT-A5.
//
// Lifecycle (GIVEN/WHEN/THEN):
//
//	GIVEN orchestrator settings (session root + model settings)
//	WHEN  Start runs
//	THEN  config-derived settings are applied, a session is created, the event
//	      bus and processing-state feed start (state idle/none), and a recorder
//	      drains the bus to disk; the orchestrator then accepts prompts.
//
//	GIVEN a started orchestrator
//	WHEN  Shutdown runs
//	THEN  it stops accepting prompts, persists a final processing-state snapshot,
//	      flushes the recorder, and returns.
//
// The orchestrator imports state and session (not config); the composition root
// (internal/app, CHT-A6) resolves config and maps it into Settings, honoring the
// import-direction matrix in docs/implementation/08_go_module_layout.md.
package runtime
