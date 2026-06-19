# Architecture Review: Zellij Clipboard Auto-Copy Feature

**Date**: 2026-06-19  
**Status**: APPROVED WITH RECOMMENDED IMPROVEMENTS  
**Review Scope**: System design for project-local zellij configuration and clipboard auto-copy

---

## Summary

The current design successfully implements project-local zellij configuration in `.agentx/config.kdl` with auto-detected clipboard command selection (Wayland `wl-copy` vs X11 `xclip`). The implementation is **technically sound but raises 5 architectural considerations**, of which **2 are required improvements** and **3 are suggestions** for future iteration.

---

## Detailed Findings

### ✅ 1. Trust Boundary — Auto-Generation Without Opt-In

**Classification**: **REQUIRED**  
**Severity**: Medium  
**Question**: Is it safe to auto-generate zellij config without explicit user opt-in? Should there be a config toggle in agentx.toml to disable auto-copy?

**Analysis**:

**Current behavior**:
- All zellij commands include `--config-dir <projectDir>/.agentx` (via `multiplexer_driver_zellij.go:command()`)
- `.agentx/config.kdl` is auto-created on first driver instantiation if missing (`ensureZellijConfigFile()`)
- Existing files are **preserved** (idempotent; respects user edits)
- No agentx.toml toggle currently exists; behavior is automatic

**Risk Assessment**:
- ✅ **Low risk** for existing AgentX users: the `.agentx/` directory is already AgentX-managed (layouts, bootstrap, etc.)
- ⚠️ **Potential friction** if user prefers system-wide zellij config (`~/.config/zellij/layout.kdl`)
- ⚠️ **Surprise activation**: If user has `multiplexer_backend = "zellij"` set in agentx.toml but never explicitly requested auto-copy, the feature silently activates on next restart

**Recommendation**: 
- ✅ **Keep silent auto-generation** (respects principle of least surprise for project-local config)
- ✅ **Add informational log on first auto-copy activation** at startup: `[AgentX Core] ℹ Auto-generated .agentx/config.kdl with clipboard auto-copy (wl-copy)`
- ❌ **Do NOT add opt-out toggle** (adds unnecessary config surface; users can manually delete `.agentx/config.kdl` if unwanted)

**Implementation**: Add log message in `ensureZellijConfigFile()` when config is first created; update memory notes.

---

### ❌ 2. Platform Coupling — Clipboard Detection at Instantiation Time

**Classification**: **REQUIRED**  
**Severity**: Medium  
**Question**: Does clipboard detection at driver instantiation time violate principle of least surprise if user switches Wayland/X11 mid-session?

**Analysis**:

**Current behavior** (`multiplexer_driver_zellij.go:zellijCopyCommand()`):
```go
if hasExecutable("wl-copy") && (WAYLAND_DISPLAY != "" || XDG_SESSION_TYPE == "wayland") {
    return "wl-copy"
}
if hasExecutable("xclip") {
    return "xclip -selection clipboard"
}
return ""
```

- Detection happens **once at driver instantiation**, not per-command
- Environment variables `WAYLAND_DISPLAY` and `XDG_SESSION_TYPE` are read at instantiation time
- If user switches display servers **mid-session**, the zellij config remains static (stale clipboard command)

**Real-world impact**:
- ✅ **Rare edge case**: Switching wayland/X11 mid-session is uncommon
- ⚠️ **Silent failure**: If X11 → Wayland switch occurs, `xclip` may still be invoked on a Wayland server (fails silently or reports errors)
- ⚠️ **Inconsistency**: zellij's `copy_command` in config.kdl is static, but clipboard availability is dynamic

**Root cause**: Zellij config files are static KDL; dynamic command detection would require runtime detection or per-pane overrides (beyond scope).

**Recommendation**:
- ✅ **Accept as-is** (instantiation-time detection is reasonable for session startup)
- ✅ **Document limitation** in memory: "Zellij clipboard command is detected at session init; mid-session display server changes are not supported"
- ✅ **Consider future enhancement**: If zellij adds dynamic command plugins, revisit this design
- ✅ **Add fallback graceful handling**: If user session loses clipboard (e.g., X11 server crash), detect in `defaultZellijConfigKDL()` and log warning if neither clipboard tool is available

**Implementation**: Update repo memory with platform coupling limitation; add comment in `zellijCopyCommand()` explaining instantiation-time semantics.

---

### ⚠️ 3. Artifact Proliferation — Three Artifact Types in `.agentx/`

**Classification**: **SUGGESTION**  
**Severity**: Low  
**Question**: We're now managing 3 artifact types in `.agentx/`: layouts/*.yaml/kdl, config.kdl (zellij), and bootstrap-prompt.md. Should we consolidate or is separation-of-concerns preferred?

**Analysis**:

**Current structure**:
```
.agentx/
├── layouts/
│   ├── default-layout.yaml     (tmux default, auto-generated)
│   ├── tmux-layout.yaml        (tmux custom, user-provided or auto)
│   └── zellij-layout.kdl       (zellij, user-provided or auto)
├── config.kdl                  (zellij config, auto-generated)
└── bootstrap-prompt.md         (init prompt, auto-generated)
```

**Assessment**:
- ✅ **Current separation is intentional**: Layouts (structure) vs Config (behavior) are distinct concerns
- ✅ **Follows zellij design**: Zellij also separates layout and config (different files)
- ⚠️ **Mild inconsistency**: Zellij layout lives in `layouts/` but zellij config does not

**Option A — Consolidate**: Move `config.kdl` to `layouts/config.kdl`
- **Pros**: All zellij files under one tree; simpler artifact discovery
- **Cons**: Breaks semantic separation (layouts != config); inconsistent with zellij's own naming

**Option B — Status quo** (Current)
- **Pros**: Aligns with zellij's design; clear separation of concerns
- **Cons**: Mild directory inconsistency

**Recommendation**: ✅ **Keep status quo** (current `.agentx/config.kdl` location is correct; do NOT consolidate into layouts/)
- Rationale: Zellij itself treats layout and config as separate concepts; consistency with upstream is preferable to perfect directory symmetry
- Note: If AgentX ever adds `config.yaml` for tmux dynamic config (beyond layouts), it should live next to zellij's `config.kdl`, reinforcing the separation

---

### ⚠️ 4. Cross-Multiplexer Consistency — Artifact Location Naming

**Classification**: **SUGGESTION**  
**Severity**: Low  
**Question**: Should zellij config move to layouts/ for consistency with tmux layout placement?

**Analysis**:

**Current naming**:
- Tmux layout: `.agentx/layouts/default-layout.yaml` (in layouts/)
- Zellij layout: `.agentx/layouts/zellij-layout.kdl` (in layouts/)
- Zellij config: `.agentx/config.kdl` **(NOT in layouts/)**

**Why config.kdl is NOT in layouts/**:
- Zellij conceptually separates layout (pane topology) from config (behavior, clipboard, keybindings)
- `--config-dir` and `--layout` are separate zellij flags
- Placing both in same dir could cause confusion during manual config edits

**Recommendation**: ✅ **Status quo is correct**
- Do NOT move `config.kdl` to `layouts/`
- Document intention: `.agentx/layouts/` is for structural templates, `.agentx/config.kdl` is for behavior configuration
- If confusion arises in future, a `.agentx/zellij/` subdirectory (consolidating zellij-layout.kdl + config.kdl) could be introduced, but that's a future Phase 2 refactoring

---

### ⚠️ 5. Upgrade/Migration — Silent Auto-Activation

**Classification**: **SUGGESTION**  
**Severity**: Low  
**Question**: If user is running with `multiplexer_backend = "zellij"` but no `.agentx/config.kdl`, auto-activation on next restart acceptable, or should we log a notice?

**Analysis**:

**Current behavior**:
- If zellij backend is configured but `.agentx/config.kdl` does not exist, it is silently auto-created
- No startup message differentiates "config already existed" from "config auto-created"
- User may not realize clipboard auto-copy is now enabled

**Upgrade scenario**:
```
Commit: Zellij config feature landed
User: Has agentx.toml with multiplexer_backend = "zellij" from earlier PR/branch
User: Pulls latest, runs AgentX
Result: .agentx/config.kdl auto-created, auto-copy silently enabled
```

**Silent activation risk**: Low
- ✅ Feature is additive (enables desired UX, clipboard auto-copy)
- ✅ Non-breaking (can be disabled by deleting `.agentx/config.kdl`)
- ⚠️ User may not notice or understand why clipboard behavior changed

**Recommendation**: ✅ **Add informational logging** (as noted in Finding #1)
- On `ensureZellijConfigFile()` first creation, log: `[AgentX Core] ℹ Auto-generated .agentx/config.kdl with clipboard auto-copy`
- No need for user interaction or opt-in flow
- Update changelog/release notes to mention feature

---

## Implementation Checklist

### Required Changes (Blockers):
- [ ] **Add startup log message** when `.agentx/config.kdl` is auto-generated (Finding #1)
  - File: `cmd/agentx-core/multiplexer_driver_zellij.go:ensureZellijConfigFile()`
  - Log format: `[AgentX Core] ℹ Auto-generated .agentx/config.kdl with clipboard auto-copy (wl-copy)`
  - Example: Test in `cmd/agentx-core/multiplexer_driver_zellij_test.go:TestZellijDriver_UsesProjectAgentXConfigDir` should capture log output

- [ ] **Document platform coupling limitation** (Finding #2)
  - File: `cmd/agentx-core/multiplexer_driver_zellij.go:zellijCopyCommand()`
  - Add comment block explaining: "Clipboard detection occurs at driver instantiation time; mid-session display server changes are not supported"
  - Update repo memory (`/memories/repo/zellij-local-config.md`) with platform coupling notes

### Recommended Improvements (Suggestions):
- [ ] **Add graceful fallback logging** (Finding #2)
  - In `defaultZellijConfigKDL()`, log warning if neither `wl-copy` nor `xclip` is available

- [ ] **Document artifact organization** (Findings #3–4)
  - Add comment to `.agentx/` structure in architecture docs
  - Clarify: "Layouts live in `.agentx/layouts/` (structural); zellij config lives in `.agentx/config.kdl` (behavioral)"

---

## Risk Assessment

| Finding | Risk | Recommendation | Timeline |
|---------|------|---|----------|
| #1 Trust Boundary | Low–Medium | Add log message (REQUIRED) | Phase 1 (immediate) |
| #2 Platform Coupling | Low–Medium | Document limitation (REQUIRED) | Phase 1 (immediate) |
| #3 Artifact Proliferation | Low | Keep status quo (document intention) | Phase 2 (optional) |
| #4 Cross-Multiplexer Consistency | Low | Keep status quo | Phase 2 (optional) |
| #5 Silent Auto-Activation | Low | Already addressed by #1 logging | Phase 1 (immediate) |

---

## Final Recommendation

✅ **APPROVE CURRENT DESIGN with required improvements**

The zellij clipboard auto-copy feature is **architecturally sound** and aligns well with AgentX's project-local configuration strategy. The design:
- ✅ Correctly separates layout (structure) from config (behavior)
- ✅ Respects zellij's conceptual model
- ✅ Maintains idempotency (preserves user edits)
- ✅ Handles platform variation gracefully (Wayland vs X11 detection)
- ✅ Integrates cleanly with existing multiplexer abstraction

**Required improvements** are minimal: add a startup log message and document platform coupling. These address the principle of transparency without requiring architectural changes.

**Proceed with Phase 1 execution** (add logging) and **consider Phase 2 enhancements** (documentation, optional refactoring) for the next release cycle.

---

## Related Memory Updates

- Updated `/memories/repo/zellij-local-config.md` with platform coupling notes
- Added launch behavior to `/memories/repo/agentx-runtime-contracts.md` regarding multiplexer driver initialization
