## Input Widget Persistence: Phase 2 Scope

**Status**: Skeleton struct defined (Phase 1), no runtime integration yet.  
**Target Phase**: Phase 2 (non-blocking, after output/context widget persistence stable).

---

## Phase 2 Design Decisions (To Be Made)

### 1. Compose-Box Scroll State

**Question**: When should compose-box horizontal scroll position be captured and persisted?

**Options**:

- A) On every keystroke (granular, high I/O)
- B) On input widget render (every frame if content scrolls)
- C) On send / on blur (lower I/O, less precise recovery)
- D) Manual save via explicit command (explicit, but easy to forget)

**Recommendation**: Option C (on send + optional manual save). Rationale:

- Scroll position is transient; exact position recovery less critical than content preservation
- Reduces I/O and log spam
- Aligns with context widget (scroll only saved on user navigation, not every keystroke)

**Acceptance check**: Document chosen behavior in input_widget.go before implementing.

---

### 2. Input History Collection

**Question**: Should recent input be captured and persisted for quick recall?

**Sub-questions**:

- **Scope**: Per-session or global (across all sessions)?
- **Size**: Max recent entries (suggest 50)?
- **Sensitivity**: Should history exclude inputs containing common PII patterns (passwords, tokens, URLs)?
- **Collection point**: On send only? On cancel/blur?

**Options**:

- A) Global history (accessible across all sessions, useful for copy/paste across sessions)
- B) Per-session history (isolated per session, lower storage, simpler scoping)
- C) No history (skip entirely, defer to Phase 3)

**Recommendation**:

- Start with **Option B (per-session)** for MVP. Lower complexity, security-friendly (isolation per session).
- **Size**: 50 entries max per session (reasonable recall window).
- **Collection**: Append on send only (not on every keystroke).
- **Sensitivity**: No PII filtering in Phase 2; can be added in Phase 3 if needed.

**Acceptance check**: Add this design decision to InputAppletState before full implementation.

---

### 3. State Ownership: Where Does Persistence Live?

**Current question**: Does input_widget.go manage persistence directly (like output_widget.go now does), or should it be abstracted into a separate handler?

**Current architecture** (output_widget.go):

- Widget directly calls `LoadAppletState(sessionID)` on startup
- Widget directly calls `SaveAppletState()` on state change

**Recommendation**: Follow same pattern as output/context widgets.

- Input widget owns state load/save calls
- No separate abstraction needed for Phase 2

---

### 4. Timestamp & Last-Accessed Tracking

**Question**: Should we track when input was last accessed, independent of content changes?

**Use cases**:

- Display "last used: 5 min ago" in session UI
- Trigger implicit save on idle (e.g., after 30 seconds no input)

**Recommendation**: Defer to Phase 3. Phase 2 saves only on explicit send.

---

## Phase 2 Implementation Checklist

- [ ] Define `InputAppletState` fields: `ComposeBoxScrollOffset`, `RecentInputHistory`, `LastInputTimestamp`
- [ ] Add `LoadInputAppletState(sessionID string)` helper in input_persistence.go
- [ ] Add `SaveInputAppletState(sessionID string, state *InputAppletState)` helper
- [ ] In input_widget.go: Load state on startup (graceful fallback if missing)
- [ ] On send action: Append to history (max 50), save state with timestamp
- [ ] On scroll: Update `ComposeBoxScrollOffset`, save state (debounce to avoid I/O spam)
- [ ] Write tests (load/save/reload cycle, session isolation, history append)

---

## Phase 3+ Future Enhancements (Not in Scope)

- [ ] PII filtering for sensitive inputs
- [ ] Global input history (cross-session recall)
- [ ] Input history search/fuzzy-find UI
- [ ] Idle detection + auto-save
- [ ] Input versioning (multi-level undo)

---

## Sign-Off

**Approved by**: [To be filled during Phase 2 intake]  
**Date**: 2026-06-13 (skeleton created as part of persistence scaffold foundation)  
**Next phase**: When output/context persistence is stable and validated in integration tests
