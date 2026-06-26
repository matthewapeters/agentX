// Package chat is the human-agent chat surface: a Bubble Tea program composing a
// two-panel layout (output on top, input on the bottom). It is the surface
// launched by `agentx`.
//
// Source contract: docs/implementation/01_runtime_blueprint.md (Bubble Tea
// Adoption) and docs/architecture/00_ARCHITECTURE_RECONCILIATION.md (2-panel
// chat surface). Backlog task: CHT-B1.
//
// CHT-B1 establishes the layout, focus, and resize/quit handling. Later tasks
// add output rendering (CHT-B2), input editing (CHT-B3), the processing-state
// indicator (CHT-B4), and the orchestrator round trip (CHT-B5).
package chat
