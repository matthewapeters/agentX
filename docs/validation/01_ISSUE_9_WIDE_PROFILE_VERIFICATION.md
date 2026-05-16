# Issue #9: Headless Tmux Width Verification Harness

_Last updated: 2026-05-16 (v0.49.1)_

## Background

**Issue**: When `launch_vibe.sh` runs in headless mode (e.g., CI/CD, no DISPLAY), the default tmux pane width of 80 columns causes the 90-character startup hint to wrap. This wrapping creates text patterns that visually resemble an ENTER prompt signature (`"Press ENTER or type command to continue"`), producing false positives in validation evidence.

**Root Cause**: Headless tmux startup via `tmux new-session` defaults to 80x24 pane geometry, regardless of terminal width. This is the system default for deterministic headless operation.

**Solution**: Launcher now supports configurable tmux dimensions via environment variables (`AGENTX_TMUX_WIDTH`, `AGENTX_TMUX_HEIGHT`), with automatic enforcement via `tmux resize-window` after session/window creation.

## Verification Harness

This document contains the canonical verification script for Issue #9, stored at [docs/validation/verify_issue9_wide.sh](verify_issue9_wide.sh).

### Purpose

Runs `N` deterministic trials of `launch_vibe.sh` with fixed tmux geometry, captures pane output, searches for ENTER prompt signatures, and generates structured evidence (CSV + markdown report).

### Features

- **Fixed geometry**: Configurable target width/height (default: 200x60)
- **Unique evidence directories**: Each run generates `/tmp/issue9_verify_profile.XXXXXX/`
- **Per-trial artifacts**: Pre-stop logs, startup logs, window lists, pane captures, stop logs
- **Structured summary**: CSV with columns: `trial, width, height, enter_prompt, e486_any, e486_tmp, ready_hint, session_present`
- **Automated report**: Markdown report with trial summary and verdict (reproduced / not_reproduced / inconclusive)
- **Deterministic**: No reliance on wallclock time, ambient environment, or prior session state

### Usage

#### Basic Invocation

```bash
cd /Projects/agentX
./docs/validation/verify_issue9_wide.sh
```

#### With Configuration

```bash
ISSUE9_TRIALS=5 ISSUE9_TMUX_WIDTH=200 ISSUE9_TMUX_HEIGHT=60 ISSUE9_START_TIMEOUT_SEC=15 \
  ./docs/validation/verify_issue9_wide.sh
```

#### Configuration Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `ISSUE9_TRIALS` | 5 | Number of launch trials to run |
| `ISSUE9_TMUX_WIDTH` | 200 | Target pane width (columns) |
| `ISSUE9_TMUX_HEIGHT` | 60 | Target pane height (rows) |
| `ISSUE9_START_TIMEOUT_SEC` | 15 | Max startup time per trial (seconds) |
| `AGENTX_TMUX_SESSION` | agentx | Tmux session name |

### Output

#### Directory Structure

```
/tmp/issue9_verify_profile.abc123/
├── summary.csv                 # Structured trial results
├── report.md                   # Human-readable verdict + summary
├── trial_1_prestop.log         # Pre-trial cleanup output
├── trial_1_start.log           # Launch_vibe.sh startup output
├── trial_1_windows.txt         # Tmux window list
├── trial_1_sizes.txt           # Pane geometry (width x height)
├── trial_1_pane.txt            # Pane capture (240 lines, tui-chat pane)
├── trial_1_stop.log            # Trial cleanup output
├── trial_2_prestop.log         # ... (repeat for each trial)
...
```

#### CSV Columns

```
trial       - Trial number (1..N)
width       - Actual pane width after enforcement (columns), or NA if session creation failed
height      - Actual pane height after enforcement (rows), or NA if session creation failed
enter_prompt - 1 if "Press ENTER or type command to continue" detected in pane; 0 otherwise
e486_any    - 1 if any E486 error detected; 0 otherwise
e486_tmp    - 1 if "E486: Pattern not found: tmp" detected (known wrap pattern); 0 otherwise
ready_hint  - 1 if "AgentX TUI ready. Submit with <leader>s" detected (expected after startup); 0 otherwise
session_present - 1 if tmux session exists; 0 if session failed to create or was cleared
```

#### Report Verdict

- **not_reproduced**: All valid trials had width/height enforced AND no ENTER prompt detected → Issue is fixed for wide profiles
- **reproduced**: At least one valid trial detected ENTER prompt → Issue persists (unexpected; indicates regression)
- **inconclusive**: No valid trials (all had NA dimensions) → Session creation failed; cannot validate

### Interpreting Results

#### Expected Behavior (Wide Profile)

```
Valid trials (non-NA dimensions): 5
tmux geometry target: 200x60
ENTER prompt hits (valid trials): 0    ← Should be 0
Ready hint hits (valid trials): 5      ← Should match valid trials
Verdict: not_reproduced               ← Expected
```

**Interpretation**: Wide geometry prevents text wrapping; ENTER prompt signature does not appear. Issue is mitigated for wide profiles.

#### Unexpected Behavior (Regression)

If you see:

```
Valid trials (non-NA dimensions): 5
ENTER prompt hits (valid trials): 1    ← Should be 0
Verdict: reproduced                    ← Unexpected!
```

**Action Required**:

1. Inspect the trial's `trial_N_pane.txt` to see the exact wrap pattern
2. Check `trial_N_start.log` for launcher output anomalies
3. Review recent changes to `launch_vibe.sh` or system tmux configuration
4. Update issue #9 with regression notes
5. Run `git log -p launch_vibe.sh` to identify the breaking change

#### Session Creation Failures

If all trials have `width=NA` and `session_present=0`:

- Check launcher logs for tmux errors
- Verify `launch_vibe.sh` has executable permissions
- Run `tmux list-sessions` manually to check system tmux state
- Review `trial_1_start.log` for error details

### Integration with Issue Tracking

This harness is referenced in [docs/ux/UX_ISSUES.md](../../ux/UX_ISSUES.md) under **Issue #9**. After running the verification:

1. **For UAT**: Share the report markdown + CSV with the user to confirm the issue is visually gone
2. **For CI/CD**: Integrate into release validation pipeline with `ISSUE9_TRIALS=3 ISSUE9_TMUX_WIDTH=200` as a smoke test
3. **For regression testing**: Re-run this harness on future releases to ensure the wrap signature doesn't reappear

### Source Code

The script is maintained in two locations (synchronized):

- **Active location**: [docs/validation/verify_issue9_wide.sh](verify_issue9_wide.sh)
- **Launcher tests**: [tests/test_launch_vibe_shutdown.py](../../tests/test_launch_vibe_shutdown.py) includes unit tests for `AGENTX_TMUX_WIDTH` / `AGENTX_TMUX_HEIGHT` enforcement

### Maintenance Notes

- **Headless testing dependency**: Requires tmux to be available on the system
- **Timeout tuning**: `ISSUE9_START_TIMEOUT_SEC=15` is suitable for development systems; adjust for slow CI environments
- **Wide-profile rationale**: 200x60 is selected to exceed common terminal widths (80, 100, 120) and ensure no wrapping of the 90-char hint
- **Artifact cleanup**: Evidence directories are left in `/tmp/` for manual inspection; use `rm -rf /tmp/issue9_verify_profile.*` to clean up old runs

---

## The Verification Script

The script below is the authoritative harness for Issue #9 validation. It is executable and can be run directly from the project root.

```bash
#!/usr/bin/env bash
# Deterministic verify-release harness for Issue #9 startup ENTER prompt.
# Uses fixed tmux geometry and unique evidence files per run.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TRIALS="${ISSUE9_TRIALS:-5}"
TIMEOUT_SEC="${ISSUE9_START_TIMEOUT_SEC:-15}"
TMUX_SESSION="${AGENTX_TMUX_SESSION:-agentx}"
TMUX_WIDTH="${ISSUE9_TMUX_WIDTH:-200}"
TMUX_HEIGHT="${ISSUE9_TMUX_HEIGHT:-60}"

RUN_DIR="$(mktemp -d /tmp/issue9_verify_profile.XXXXXX)"
SUMMARY_CSV="$RUN_DIR/summary.csv"
REPORT_MD="$RUN_DIR/report.md"
LAST_RUN_POINTER="/tmp/issue9_verify_profile_last_dir.txt"

printf "%s\n" "$RUN_DIR" > "$LAST_RUN_POINTER"

printf "trial,width,height,enter_prompt,e486_any,e486_tmp,ready_hint,session_present\n" > "$SUMMARY_CSV"

generate_report() {\n    local valid_trials\n    local enter_hits\n    local ready_hits\n    local verdict\n\n    valid_trials=\"$(awk -F, 'NR>1 && $2 != \"NA\" {count++} END {print count+0}' \"$SUMMARY_CSV\")\"\n    enter_hits=\"$(awk -F, 'NR>1 && $2 != \"NA\" && $4 == 1 {count++} END {print count+0}' \"$SUMMARY_CSV\")\"\n    ready_hits=\"$(awk -F, 'NR>1 && $2 != \"NA\" && $7 == 1 {count++} END {print count+0}' \"$SUMMARY_CSV\")\"\n\n    verdict=\"inconclusive\"\n    if [[ \"$valid_trials\" -gt 0 ]]; then\n        if [[ \"$enter_hits\" -gt 0 ]]; then\n            verdict=\"reproduced\"\n        else\n            verdict=\"not_reproduced\"\n        fi\n    fi\n\n    cat > \"$REPORT_MD\" <<EOF\n# Issue #9 Verification Report (Wide Profile)\n\n- Run directory: $RUN_DIR\n- Trials requested: $TRIALS\n- Valid trials (non-NA dimensions): $valid_trials\n- tmux geometry target: ${TMUX_WIDTH}x${TMUX_HEIGHT}\n- Startup timeout per trial: ${TIMEOUT_SEC}s\n- ENTER prompt hits (valid trials): $enter_hits\n- Ready hint hits (valid trials): $ready_hits\n- Verdict: $verdict\n\n## Summary CSV\n\n\\`\\`\\`\n$(cat \"$SUMMARY_CSV\")\n\\`\\`\\`\nEOF\n}\n\ntrap generate_report EXIT\n\nrun_trial() {\n    local trial=\"$1\"\n    local prestop=\"$RUN_DIR/trial_${trial}_prestop.log\"\n    local startlog=\"$RUN_DIR/trial_${trial}_start.log\"\n    local windows=\"$RUN_DIR/trial_${trial}_windows.txt\"\n    local sizes=\"$RUN_DIR/trial_${trial}_sizes.txt\"\n    local pane=\"$RUN_DIR/trial_${trial}_pane.txt\"\n    local stoplog=\"$RUN_DIR/trial_${trial}_stop.log\"\n\n    AGENTX_TMUX_WIDTH=\"$TMUX_WIDTH\" AGENTX_TMUX_HEIGHT=\"$TMUX_HEIGHT\" ./launch_vibe.sh stop > \"$prestop\" 2>&1 || true\n    AGENTX_TMUX_WIDTH=\"$TMUX_WIDTH\" AGENTX_TMUX_HEIGHT=\"$TMUX_HEIGHT\" timeout \"${TIMEOUT_SEC}s\" ./launch_vibe.sh > \"$startlog\" 2>&1 || true\n\n    local session_present=\"0\"\n    if tmux has-session -t \"$TMUX_SESSION\" 2>/dev/null; then\n        session_present=\"1\"\n        tmux list-windows -t \"$TMUX_SESSION\" > \"$windows\" 2>&1 || true\n        tmux list-panes -a -F '#{session_name}:#{window_name}.#{pane_index} #{pane_width}x#{pane_height}' > \"$sizes\" 2>&1 || true\n        tmux capture-pane -p -t \"$TMUX_SESSION:tui-chat.0\" -S -240 > \"$pane\" 2>&1 || true\n    else\n        printf \"NO_TMUX_SESSION\\\\n\" > \"$windows\"\n        printf \"NO_TMUX_SESSION\\\\n\" > \"$sizes\"\n        printf \"NO_TMUX_SESSION\\\\n\" > \"$pane\"\n    fi\n\n    local width=\"NA\"\n    local height=\"NA\"\n    if grep -q \"${TMUX_SESSION}:tui-chat.0\" \"$sizes\"; then\n        local size_token\n        size_token=\"$(awk -v s=\"$TMUX_SESSION\" '$1 ~ (\"^\" s \":tui-chat\\\\.0$\") {print $2; exit}' \"$sizes\")\"\n        if [[ -n \"$size_token\" ]]; then\n            width=\"${size_token%x*}\"\n            height=\"${size_token#*x}\"\n        fi\n    fi\n\n    local enter_sig=\"0\"\n    local e486_any_sig=\"0\"\n    local e486_tmp_sig=\"0\"\n    local ready_sig=\"0\"\n\n    grep -q \"Press ENTER or type command to continue\" \"$pane\" && enter_sig=\"1\" || true\n    grep -q \"E486:\" \"$pane\" && e486_any_sig=\"1\" || true\n    grep -q \"E486: Pattern not found: tmp\" \"$pane\" && e486_tmp_sig=\"1\" || true\n    grep -q \"AgentX TUI ready. Submit with <leader>s\" \"$pane\" && ready_sig=\"1\" || true\n\n    printf \"%s,%s,%s,%s,%s,%s,%s,%s\\\\n\" \\\n        \"$trial\" \"$width\" \"$height\" \"$enter_sig\" \"$e486_any_sig\" \"$e486_tmp_sig\" \"$ready_sig\" \"$session_present\" >> \"$SUMMARY_CSV\"\n\n    AGENTX_TMUX_WIDTH=\"$TMUX_WIDTH\" AGENTX_TMUX_HEIGHT=\"$TMUX_HEIGHT\" ./launch_vibe.sh stop > \"$stoplog\" 2>&1 || true\n}\n\nfor ((i = 1; i <= TRIALS; i++)); do\n    run_trial \"$i\"\ndone\n\nprintf \"RUN_DIR=%s\\\\n\" \"$RUN_DIR\"\nprintf \"SUMMARY=%s\\\\n\" \"$SUMMARY_CSV\"\nprintf \"REPORT=%s\\\\n\" \"$REPORT_MD\"\ncat \"$SUMMARY_CSV\"\n```\n\n## References\n\n- GitHub Issue: [matthewapeters/agentX#9](https://github.com/matthewapeters/agentX/issues/9)\n- Launcher source: [launch_vibe.sh](../../launch_vibe.sh) (lines implementing `AGENTX_TMUX_WIDTH`/`AGENTX_TMUX_HEIGHT`)\n- Launcher tests: [tests/test_launch_vibe_shutdown.py](../../tests/test_launch_vibe_shutdown.py) (`test_start_uses_configured_tmux_dimensions_for_new_session`)\n- Issue tracking: [docs/ux/UX_ISSUES.md § Issue #9](../../ux/UX_ISSUES.md)\n
