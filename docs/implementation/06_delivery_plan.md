# Implementation Delivery Plan

## Goal

Deliver a production-capable Go runtime with charm.land Bubbletea TUI and HTTP surface transport that implements UX and architecture contracts.

## Phase 0: Contracts Freeze

Outputs:

- freeze processing-state schema
- freeze event envelope schema
- freeze config and session path rules
- freeze tool approval policy schema
- freeze surface registration and shutdown protocol
- freeze session identity schema (session_id plus session_name)
- freeze surface launch CLI and attach token contract
- freeze Godog test contract and Gherkin traceability schema
- freeze documentation-first function expectation template (GIVEN/WHEN/THEN)
- freeze canonical Go module folder layout and package boundary rules
- freeze makefile contract (make all = make clean && make build)
- freeze failing-test ownership policy across downstream effects
- freeze dependency hygiene policy (dependency changes require go mod tidy and go mod vendor)
- freeze semver/changelog linkage policy (semver changes require CHANGELOG.md update)
- freeze quality gate severity policy (blockers and warnings required; nits optional)

Exit criteria:

- schemas documented and reviewed
- unresolved choices tracked in 90_open_questions.md

## Phase 1: Runtime Skeleton

Build:

- agentx entrypoint lifecycle
- add and pin charmbracelet/bubbletea submodule reference
- config resolution and first-run seeding
- session directory initialization
- HTTP server boot
- surface registry boot
- child surface process registration and shutdown protocol
- configured-range port allocator with conflict checks
- human-readable session name generator with collision handling
- endpoint-based launch command generation for child surfaces
- attach token issue and validation at surface registration
- baseline Godog project wiring and feature folder structure
- scaffold canonical cmd, internal, and tests folder structure per layout standard
- implement and document make all target aliasing clean then build

Exit criteria:

- startup and shutdown stable
- health and surfaces endpoints operational
- multiple concurrent local AgentX instances supported
- surface attach succeeds only with valid token
- generated launch command works in fresh terminal session
- make all passes locally and in CI baseline job

## Phase 2: TUI Integration (Bubble Tea)

Build:

- Bubble Tea primary surface host
- subscribe to processing state and event stream
- render baseline UX surfaces (output, input, system)
- author corresponding Godog feature scenarios for surface behavior and variants
- author function-level behavior expectations before implementation for touched functions

Exit criteria:

- prompt submit cycle visible end-to-end
- processing state reflected in UI without drift

## Phase 3: LLM and Prompt Stack

Build:

- Ollama adapter
- prompt assembly pipeline
- procedural prompt stages with strict schema validation
- model switch workflow without restart
- ask-user flow for in-flight prompt handling during model switch

Exit criteria:

- successful classify -> think -> tool -> respond cycle
- model switch success/failure surfaced cleanly

## Phase 4: Tool Runtime and Policy

Build:

- MCP-style tool descriptor loader
- policy evaluator (blacklist/whitelists)
- approval UX and persistence
- safe command execution pipeline

Exit criteria:

- approved tools execute and return structured results
- blocked tools are safely denied and logged

## Phase 5: Persistence and Replay

Build:

- append-oriented JSON event persistence
- session index and replay support
- minimal diagnostics for failed events

Exit criteria:

- complete session timeline recoverable from disk
- deterministic event ordering by epoch

## Phase 6: Hardening

Build:

- reliability and race testing
- load testing for streaming and tools
- failure injection for model/tool/storage paths
- CI merge gates for missing Gherkin coverage or missing behavior traceability
- CI merge gate for make all failure, including downstream test failures
- CI gate for dependency hygiene drift when dependencies change
- CI gate for missing CHANGELOG.md update on semantic version change

Exit criteria:

- acceptance criteria from UX traceability matrix satisfied
- architecture contract checks pass
- Godog suites pass for all required tags
- CI blocks merge on missing GIVEN/WHEN/THEN expectation docs
- blocker/warning issues resolved before merge; nits may be deferred

## Delivery Notes

- Keep implementation packages aligned with this plan.
- Add ADR entries for any cross-cutting decisions that alter contracts.
- No phase should redefine UX behavior contracts; only implement them.
