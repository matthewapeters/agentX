## Summary

- What changed:
- Why:
- Risk level:

## Acceptance Evidence (Required)

- [ ] User-visible acceptance criteria are listed explicitly (not implied by implementation details).
- [ ] Semantic parity verified for affected GUI/TUI behavior.
- [ ] Config parity verified (defaults, file config, and env overrides where applicable).
- [ ] Observability parity verified (startup/runtime signals clearly indicate effective mode/backend).
- [ ] Negative-path behavior verified (explicit failure or skip signal, no silent fallback ambiguity).

## Required Artifacts

- [ ] Startup/runtime capture attached (pane capture, logs, or equivalent) proving effective backend/mode.
- [ ] Test evidence attached for both happy path and fallback/error path.

## Tests Run

- [ ] `make build`
- [ ] `make hybrid-merge-gate`
- [ ] Focused tests for changed area:

## Checklist for Backend/Model Changes

- [ ] Resolved runtime config is consumed by all child processes/subsystems.
- [ ] Launch-time env propagation is covered by automated tests.
- [ ] Startup behavior verifies real backend response when configured for live provider.
- [ ] Non-live backend path shows explicit explanatory message.

## Notes

- Follow-up items / known limitations:
