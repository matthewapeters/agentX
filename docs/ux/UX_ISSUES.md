# UX ISSUES

UX issues are tracked in the project issue tracker. This file was previously used for
Tkinter/Wayland-specific bug archaeology and has been cleared for the Go TUI implementation.

## How to Use This File

Add new issue entries below the divider. Use the format:

```
[ ] <Short description of the problem>. <Observed behaviour>. <Expected behaviour>.
```

Semaphore semantics:

| Marker | Set by | Meaning |
|--------|--------|---------|
| `[ ]`  | User   | Issue reported; not yet addressed. |
| `[/]`  | Agent  | Fix committed and tests pass; ready for UAT. |
| `[X]`  | Either | Fix attempted but failed or blocked; needs follow-up. |

For each `[ ]` entry:

1. Locate the affordance in `docs/ux/03_PANEL_DETAILS.md` using the Affordance ID from `docs/ux/UX_LIFECYCLE.md`.
2. Check the Gherkin spec for the affordance.
3. Diagnose whether the bug is a code defect, missing/wrong Gherkin expectation, or spec/code/test drift.
4. Fix the root cause and update the Traceability Matrix status.

---
