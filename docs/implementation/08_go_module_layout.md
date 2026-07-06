# Go Module Folder Layout Standard (v1)

## Purpose

Provide a clear, enforceable project layout for AgentX so engineers avoid folder ambiguity while implementing a single-command Go application.

## Scope

This standard applies to repository organization, package boundaries, test placement, and documentation placement for all new Go implementation work.

## Core Principles

1. Single runtime command is anchored at cmd/agentx.
2. Internal runtime code lives under internal/.
3. Godog and Gherkin are mandatory for behavior testing.
4. Documentation-first delivery is mandatory before implementation.
5. New top-level folders require an architecture decision and documentation update.

## Canonical Repository Layout

Required top-level layout for v1:

    cmd/
      agentx/
        main.go

    internal/
      app/
      runtime/
      cli/
      config/
      state/
      transport/
        http/
      surfaces/
        output/
        input/
        system/
      session/
      tools/
      llm/
        ollama/
      prompting/

    tests/
      features/
        runtime/
        surfaces/
        tools/
        session/
      steps/
      suites/

    docs/
      architecture/
      ux/
      implementation/

    scripts/

Notes:

- Existing legacy command folders may remain temporarily during migration, but net-new command entrypoints should target cmd/agentx.
- No new public library packages are required in v1.

## Folder Ownership and Use Rules

cmd/agentx:

- Only process bootstrap and CLI wiring.
- No business logic beyond startup orchestration.

internal/app:

- High-level composition and dependency assembly.

internal/runtime:

- Runtime lifecycle, supervision, and shutdown flow.

internal/cli:

- Command parsing, validation, and user-facing command behavior.

internal/transport/http:

- HTTP handlers, SSE stream handlers, request/response DTO mapping.

internal/surfaces:

- Surface process contracts and registration behavior.

internal/session:

- Session identity, event persistence, replay metadata.

internal/tools:

- Tool descriptors, approval policy, execution controls.

internal/prompting and internal/llm:

- Prompt assembly and model adapter implementation.

tests/features:

- Gherkin feature files grouped by behavior domain.

tests/steps:

- Godog step definitions mapped to feature files.

tests/suites:

- Godog suite runners and shared test harness setup.

## Godog Test Structure Standard

Feature file convention:

    tests/features/<domain>/<use_case>.feature

Step definition convention:

    tests/steps/<domain>/<use_case>_steps_test.go

Suite runner convention:

    tests/suites/<domain>_godog_test.go

Required scenario metadata in feature files:

- Use-case identifier
- Variant identifier when applicable
- Source contract references to UX/architecture/implementation docs

## Documentation-First Placement

Before code for a new behavior:

1. Update relevant implementation contract in docs/implementation/.
2. Add or update Gherkin scenarios in tests/features/.
3. Add function-level GIVEN/WHEN/THEN expectations in design docs or package-level behavior notes.

Documentation expectation:

- Behavior-critical changes require documentation detail at least comparable to implementation complexity.

## Package and Import Guardrails

1. cmd/agentx imports internal packages, not vice versa.
2. internal packages should depend inward toward domain logic, not outward toward CLI glue.
3. Avoid circular dependencies by keeping DTOs close to transport boundaries.
4. Keep shared cross-domain primitives in narrowly scoped internal packages.

## Import Direction Matrix (Normative)

Use this table as the default dependency contract for v1.

| From | Allowed imports | Disallowed imports |
|---|---|---|
| cmd/agentx | internal/app, internal/cli, internal/config | tests/, docs/, direct imports from deep domain internals when app composition layer exists |
| internal/app | internal/runtime, internal/config, internal/state, internal/transport, internal/session, internal/tools, internal/llm, internal/prompting, internal/surfaces | cmd/agentx, tests/suites |
| internal/cli | internal/config, internal/session, internal/surfaces, internal/transport | cmd/agentx, tests/steps |
| internal/runtime | internal/state, internal/session, internal/surfaces, internal/transport, internal/tools, internal/llm, internal/prompting | cmd/agentx, tests/ |
| internal/transport/http | internal/state, internal/session, internal/surfaces, internal/tools | cmd/agentx, tests/ |
| internal/surfaces/* | internal/state, internal/session, internal/transport | cmd/agentx, tests/ |
| internal/session | internal/state, internal/config | cmd/agentx, tests/ |
| internal/tools | internal/config, internal/session, internal/state | cmd/agentx, tests/ |
| internal/llm/* | internal/config, internal/state | cmd/agentx, tests/ |
| internal/prompting | internal/config, internal/state, internal/session, internal/llm/fanout | cmd/agentx, tests/ |
| tests/steps | internal/* (test-only usage) | cmd/agentx |
| tests/suites | tests/steps, internal/* (test-only usage) | cmd/agentx |

Guidance:

- Prefer depending on internal/app and runtime contracts instead of reaching into deep implementation packages.
- Any exception to this matrix must be documented in implementation docs and approved in review.
- Sibling packages within one top-level group (notably `internal/llm/*` — `fanout`,
  `ollama`, `invoke`) may import one another. The matrix rows constrain cross-group
  direction, not intra-group composition.
- **Documented exception:** `internal/prompting/corpus` imports `internal/llm/fanout`.
  The prompting layer renders the LLM fan-out invocations (a fan-group compiles to
  `[]fanout.Invocation` with a `fanout.Contract`), so it legitimately depends on that
  invocation/contract primitive. `fanout` is a narrow, dependency-free primitive
  (guidance rule 4), so this does not create a cycle or an outward CLI dependency.
  See `docs/architecture/prompt_fan_groups.md`.

## Package Naming Standard (v1)

Naming rules:

1. Use short, lowercase package names.
2. Use singular names by default (example: session, not sessions).
3. Avoid generic names like utils, helpers, common, misc.
4. Name packages by domain responsibility (example: transport/http, not network_stuff).
5. Avoid stutter in identifiers (example: session.Manager, not session.SessionManagerManager).

Examples:

- Good: internal/session, internal/prompting, internal/transport/http
- Avoid: internal/utils, internal/common, internal/helpers

## New Package Checklist (Required)

Before adding a new package:

1. Confirm no existing package already owns the behavior.
2. Add or update behavior contract docs with GIVEN/WHEN/THEN expectations.
3. Add or update Godog feature coverage for the package behavior.
4. Define the package responsibility in one sentence.
5. Verify imports comply with the Import Direction Matrix.
6. Add package-level documentation comments.
7. Add test references in tests/features and tests/steps.
8. Update this layout standard if new directory patterns are introduced.

Definition of done for new package introduction:

- Documentation and Gherkin scenarios exist before implementation merge.
- CI traceability gates pass.
- Review confirms package boundary and naming compliance.

## Legacy Layout Migration Guidance

Migration policy for existing non-standard folders:

1. Do not create new features in legacy command folders.
2. Route all net-new command entrypoints through cmd/agentx.
3. Migrate legacy runtime code into internal/ packages incrementally.
4. For each migration, preserve behavior with Godog scenarios before and after move.
5. Record migration status in implementation docs until legacy folder removal is complete.

## Example Mapping

Behavior: launch child surface and register to session.

Expected artifact mapping:

- docs/implementation/02_surface_orchestration_http.md
- tests/features/surfaces/launch_child_surface.feature
- tests/steps/surfaces/launch_child_surface_steps_test.go
- internal/cli/surface_launch.go
- internal/surfaces/registry.go
- internal/transport/http/surface_register_handler.go

## Change Control

If an engineer proposes a new top-level folder:

1. Document rationale in docs/implementation/.
2. Add import-boundary implications.
3. Update this layout standard.
4. Add an ADR entry if cross-cutting impact exists.
