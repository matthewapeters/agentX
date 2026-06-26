// Package state is the canonical in-process event and processing-state layer.
//
// Source contract: docs/architecture/channel_registry.md and the frozen schemas
// in docs/architecture/runtime_contracts/ (event-envelope, processing-state).
// Backlog task: CHT-A3.
//
// Two primitives:
//
//	Bus — fan-out of Events to every Subscription, in published order, with a
//	per-subscriber queue so a slow consumer never blocks the publisher or other
//	subscribers ("ordered, atomic, all receive").
//
//	ProcessingPublisher — a session-level, low-frequency processing-state feed
//	(idle|working|completed|failed × classify|thinking|tool|respond|none) that
//	surfaces consume to render a consistent working indicator.
//
// This is the seam the future Family B orchestrator sits behind: surfaces depend
// on these contracts, not on orchestration internals
// (docs/architecture/00_ARCHITECTURE_RECONCILIATION.md).
package state
