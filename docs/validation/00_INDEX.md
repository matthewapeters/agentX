# Validation & Verification Documentation

_Last updated: 2026-05-16 (v0.49.1)_

This folder contains validation and verification infrastructure for AgentX, including issue-specific verification harnesses, validation protocols, and UAT procedures.

## Files in This Folder

| File | Purpose | Status |
|------|---------|--------|
| [01_ISSUE_9_WIDE_PROFILE_VERIFICATION.md](01_ISSUE_9_WIDE_PROFILE_VERIFICATION.md) | Deterministic verification harness for Issue #9 (headless tmux ENTER prompt false positive); enforces fixed tmux geometry (200x60 default) for reproducible validation evidence | Maintained |

## Quick Start: Running Verification Scripts

All verification scripts in this folder are executable bash scripts designed to run deterministically with configurable parameters. Each script generates a timestamped evidence directory and markdown report.

### General Workflow

1. **Read the verification document** — Understand the issue being validated and the evidence being collected
2. **Configure environment** — Set environment variables for geometry, trial count, timeouts (documented in each script's header)
3. **Run the script** — Execute the harness and collect evidence
4. **Review the report** — Check the generated `.md` report and `.csv` summary
5. **Record results** — Update `docs/ux/UX_ISSUES.md` with UAT status

### Example: Issue #9 Wide-Profile Verification

```bash
cd /Projects/agentX
ISSUE9_TRIALS=3 ISSUE9_TMUX_WIDTH=200 ISSUE9_TMUX_HEIGHT=60 ./docs/validation/verify_issue9_wide.sh
```

The script will:

- Generate a unique timestamped temp directory (e.g., `/tmp/issue9_verify_profile.abc123/`)
- Run 3 launch trials with fixed 200x60 geometry
- Collect pane captures, window lists, and diagnostic logs for each trial
- Generate `summary.csv` and `report.md` in the temp directory
- Print the directory path to stdout so you can inspect artifacts

## Maintenance Policy

- All verification scripts must be **hermetic** (reproducible on any system; deterministic results)
- Each script must **generate unique evidence directories** per run (using `mktemp`)
- Scripts must **capture detailed diagnostics** per trial (window lists, pane captures, logs)
- Results must be **parseable** (CSV summary + markdown report for human review)
- Scripts are **issue-specific** — one script per issue; do not reuse harnesses across issues
- When an issue is resolved (user confirms in UAT), leave the verification script in place for **regression testing** in future releases

## Adding a New Verification Script

When adding a new issue-specific verification harness:

1. Create a new shell script in `docs/validation/` named `verify_issue_NNN_SHORT_NAME.sh`
2. Document the issue, root cause, verification approach, and expected evidence in a markdown file named `0N_ISSUE_NNN_SHORT_DESCRIPTION.md`
3. Include the script content (or reference) in the markdown file
4. Add a row to the table above
5. Update `docs/ux/UX_ISSUES.md` with a link to the verification procedure
6. Commit with message: `docs(validation): add verification harness for issue #NNN`

## Cross-References

- **Issue tracking**: [docs/ux/UX_ISSUES.md](../ux/UX_ISSUES.md)
- **UX specs**: [docs/ux/03_PANEL_DETAILS.md](../ux/03_PANEL_DETAILS.md)
- **Launcher source**: [agentx](../../agentx)
- **Launcher tests**: [tests/test_launch_vibe_shutdown.py](../../tests/test_launch_vibe_shutdown.py)
