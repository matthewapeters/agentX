# Runtime Contracts — Family A Freeze Index (v1)

Last updated: 2026-06-26
Status: Frozen v1 (Phase 0 baseline)
Owner: Architecture

## Purpose

This folder is the authoritative, frozen home for the **Family A** (client-server
runtime) contracts that `docs/implementation/06_delivery_plan.md` Phase 0 and
`docs/build-plan/` M0 require to be locked before implementation. It does not
introduce new behavior — it gives the contracts that already exist (mostly as prose
across the implementation docs and the channel registry) one referenceable, versioned
location, and adds machine-checkable JSON Schema where validation pays off.

For the A-vs-B split, see
[`../00_ARCHITECTURE_RECONCILIATION.md`](../00_ARCHITECTURE_RECONCILIATION.md). The
schemas under `../schemas/` and designs under `../design/` are **Family B (future)**
and are out of scope for this freeze.

## Frozen Phase 0 Items → Canonical Source

| Phase 0 freeze item | Canonical source (authoritative prose) | Machine schema (this folder) |
| --- | --- | --- |
| Processing-state schema | `channel_registry.md` (Processing State Contract) + `../../implementation/02_surface_orchestration_http.md` | `processing-state.schema.json` |
| JSON event envelope | `../../implementation/03_configuration_and_storage.md` (JSON Event Envelope) | `event-envelope.schema.json` |
| Config + session path rules | `../../implementation/03_configuration_and_storage.md` (Configuration Source of Truth, Session Storage Root) | — (prose-frozen) |
| Session identity (session_id + session_name) | `../../implementation/03_configuration_and_storage.md` (Session metadata) | covered in event-envelope + surface-registration |
| Tool approval / command policy schema | `../../implementation/05_security_approvals_and_command_policy.md` | — (prose-frozen) |
| Surface registration + shutdown protocol | `../../implementation/02_surface_orchestration_http.md` (Surface Registration Contract) | `surface-registration.schema.json` |
| Attach-token contract | `../../implementation/02_surface_orchestration_http.md` (Attach security contract) | fingerprint field in `surface-registration.schema.json` |
| Surface launch CLI contract | `../../implementation/02_surface_orchestration_http.md` (Normative CLI Specification) | — (prose-frozen) |
| Makefile contract (`make all` = clean + build) | `../../implementation/09_makefile_and_quality_gate_contract.md` | — (prose-frozen; enforced by `Makefile`) |
| Godog / GIVEN-WHEN-THEN test contract | `../../implementation/07_test_and_documentation_contract.md` | — (prose-frozen; enforced by `tests/`) |
| Go module layout | `../../implementation/08_go_module_layout.md` | — (prose-frozen) |

## Freeze Semantics

- These contracts are **v1 frozen**: producers and consumers may rely on them.
- Adding new required fields or stricter conditions is a **breaking change** — bump a
  version (new `$id`/path) or coordinate a single cutover.
- Changes to a frozen contract must update both the canonical prose source and the
  corresponding schema here, and be recorded per the change-control rules in the
  source doc.
- Open items discovered during freeze are tracked in
  `../../implementation/90_open_questions.md`.

## Schemas In This Folder

- `event-envelope.schema.json` — persisted session-event envelope.
- `processing-state.schema.json` — shared session-level processing-state feed.
- `surface-registration.schema.json` — surface registration payload (incl. attach-token fingerprint).
