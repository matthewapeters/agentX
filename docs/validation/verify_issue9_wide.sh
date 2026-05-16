#!/usr/bin/env bash
# Deterministic verify-release harness for Issue #9 startup ENTER prompt.
# Uses fixed tmux geometry and unique evidence files per run.
# Ref: docs/validation/01_ISSUE_9_WIDE_PROFILE_VERIFICATION.md

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../" && pwd)"
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

generate_report() {
    local valid_trials
    local enter_hits
    local ready_hits
    local verdict

    valid_trials="$(awk -F, 'NR>1 && $2 != "NA" {count++} END {print count+0}' "$SUMMARY_CSV")"
    enter_hits="$(awk -F, 'NR>1 && $2 != "NA" && $4 == 1 {count++} END {print count+0}' "$SUMMARY_CSV")"
    ready_hits="$(awk -F, 'NR>1 && $2 != "NA" && $7 == 1 {count++} END {print count+0}' "$SUMMARY_CSV")"

    verdict="inconclusive"
    if [[ "$valid_trials" -gt 0 ]]; then
        if [[ "$enter_hits" -gt 0 ]]; then
            verdict="reproduced"
        else
            verdict="not_reproduced"
        fi
    fi

    cat > "$REPORT_MD" <<EOF
# Issue #9 Verification Report (Wide Profile)

- Run directory: $RUN_DIR
- Trials requested: $TRIALS
- Valid trials (non-NA dimensions): $valid_trials
- tmux geometry target: ${TMUX_WIDTH}x${TMUX_HEIGHT}
- Startup timeout per trial: ${TIMEOUT_SEC}s
- ENTER prompt hits (valid trials): $enter_hits
- Ready hint hits (valid trials): $ready_hits
- Verdict: $verdict

## Summary CSV

$(cat "$SUMMARY_CSV")
EOF
}

trap generate_report EXIT

run_trial() {
    local trial="$1"
    local prestop="$RUN_DIR/trial_${trial}_prestop.log"
    local startlog="$RUN_DIR/trial_${trial}_start.log"
    local windows="$RUN_DIR/trial_${trial}_windows.txt"
    local sizes="$RUN_DIR/trial_${trial}_sizes.txt"
    local pane="$RUN_DIR/trial_${trial}_pane.txt"
    local stoplog="$RUN_DIR/trial_${trial}_stop.log"

    AGENTX_TMUX_WIDTH="$TMUX_WIDTH" AGENTX_TMUX_HEIGHT="$TMUX_HEIGHT" ./launch_vibe.sh stop > "$prestop" 2>&1 || true
    AGENTX_TMUX_WIDTH="$TMUX_WIDTH" AGENTX_TMUX_HEIGHT="$TMUX_HEIGHT" timeout "${TIMEOUT_SEC}s" ./launch_vibe.sh > "$startlog" 2>&1 || true

    local session_present="0"
    if tmux has-session -t "$TMUX_SESSION" 2>/dev/null; then
        session_present="1"
        tmux list-windows -t "$TMUX_SESSION" > "$windows" 2>&1 || true
        tmux list-panes -a -F '#{session_name}:#{window_name}.#{pane_index} #{pane_width}x#{pane_height}' > "$sizes" 2>&1 || true
        tmux capture-pane -p -t "$TMUX_SESSION:tui-chat.0" -S -240 > "$pane" 2>&1 || true
    else
        printf "NO_TMUX_SESSION\n" > "$windows"
        printf "NO_TMUX_SESSION\n" > "$sizes"
        printf "NO_TMUX_SESSION\n" > "$pane"
    fi

    local width="NA"
    local height="NA"
    if grep -q "${TMUX_SESSION}:tui-chat.0" "$sizes"; then
        local size_token
        size_token="$(awk -v s="$TMUX_SESSION" '$1 ~ ("^" s ":tui-chat\\.0$") {print $2; exit}' "$sizes")"
        if [[ -n "$size_token" ]]; then
            width="${size_token%x*}"
            height="${size_token#*x}"
        fi
    fi

    local enter_sig="0"
    local e486_any_sig="0"
    local e486_tmp_sig="0"
    local ready_sig="0"

    grep -q "Press ENTER or type command to continue" "$pane" && enter_sig="1" || true
    grep -q "E486:" "$pane" && e486_any_sig="1" || true
    grep -q "E486: Pattern not found: tmp" "$pane" && e486_tmp_sig="1" || true
    grep -q "AgentX TUI ready. Submit with <leader>s" "$pane" && ready_sig="1" || true

    printf "%s,%s,%s,%s,%s,%s,%s,%s\n" \
        "$trial" "$width" "$height" "$enter_sig" "$e486_any_sig" "$e486_tmp_sig" "$ready_sig" "$session_present" >> "$SUMMARY_CSV"

    AGENTX_TMUX_WIDTH="$TMUX_WIDTH" AGENTX_TMUX_HEIGHT="$TMUX_HEIGHT" ./launch_vibe.sh stop > "$stoplog" 2>&1 || true
}

for ((i = 1; i <= TRIALS; i++)); do
    run_trial "$i"
done

printf "RUN_DIR=%s\n" "$RUN_DIR"
printf "SUMMARY=%s\n" "$SUMMARY_CSV"
printf "REPORT=%s\n" "$REPORT_MD"
cat "$SUMMARY_CSV"
