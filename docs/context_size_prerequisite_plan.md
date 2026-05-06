# Dynamic Context Resizing - Implementation Plan (PRE-02)

**Status:** Complete — 2026-04-26
**Date:** 2026-04-26
**Feature branch:** `feature/context-size-prerequisite`
**Linked spec:** `docs/ux/03_PANEL_DETAILS.md §PD-10` (ContextMeterWidget cut-sheet)
**Linked ARCH items:** ARCH-02, ARCH-05, ARCH-06, ARCH-08, ARCH-09, ARCH-10
**Linked REQ:** REQ-03, REQ-05, REQ-06, REQ-07, REQ-08
**Token strategy:** TOK-02 (model-family char/token ratios)

---

## Objective

Ensure the context meter denominator is always the actual context-window size of the
currently selected model, and refreshes immediately when the user switches models.

The current code derives `max_tokens` from `parameter_size` (e.g. `7B`), which is not
context capacity. This plan replaces that behavior with provider-backed capacity data.

The design also introduces a provider abstraction so model listing and capacity lookup
are backend-agnostic (Ollama now, extensible later).

---

## Success Criteria

- On startup, the app queries the active LLM provider for all model names and capacities.
- Startup data is stored in an in-memory `ModelMetadataStore` used by both model selector
  and context visualizer.
- A disk cache file `sessions/_model_cache.json` is used across app runs.
- If model names match the cache on startup, capacities are loaded from cache without
  re-querying each model capacity endpoint.
- Model change triggers immediate meter redraw (Q-09 = yes).
- Denominator never uses `parameter_size` as a proxy for context window.
- Both `src/agentix/` and `src/agentx/` consume a shared provider abstraction.
- Redraw calls from non-UI threads are scheduled via `root.after(0, ...)`.
- Tests cover startup enumeration, cache reuse, cache refresh, fallback behavior, and
  model-change redraw wiring.

---

## Decisions on Ambiguities and Doubts

| Ref | Decision |
|-----|----------|
| **Q-08** | On startup, query all models and capacities, store for runtime use. Add disk cache; if model set is unchanged on startup, reuse cached capacities and skip per-model capacity re-query. |
| **Q-09** | Yes, model-change callback must trigger immediate meter redraw. |
| **DOUBT-01** | Approved direction: treat ARCH-02 as replacing `parse_parameter_size` usage in `src/agentix/models.py` with provider capacity lookup. |
| **DOUBT-02** | Fix both source trees consistently; shared abstraction to reduce future refactor risk. |
| **DOUBT-03** | Encapsulate model listing + capacity retrieval per LLM service behind a common API in AgentX (`ILLMServiceProvider`). |
| **DOUBT-04** | Use separate constant `FALLBACK_CONTEXT_WINDOW = 4096`. |
| **DOUBT-05** | Interim solution approved: store/cache ownership lives in session-level store until widget exists. |

---

## Scope

### In scope

- `ILLMServiceProvider` protocol and `OllamaServiceProvider` implementation.
- Startup model enumeration and capacity prefetch into `ModelMetadataStore`.
- Disk cache support in `sessions/_model_cache.json`.
- Immediate redraw on `active_model` change.
- Shared provider usage in both `src/agentix/` and `src/agentx/`.
- `IGUIManager.update_context_meter(...)` hook and GUIManager stub.
- `Context.token_breakdown(model_name=...)` for meter numerator bands.
- Unit + integration + functional tests for new behavior.

### Out of scope

- Full visual `ContextMeterWidget` implementation.
- Tokenizer-based counting migration (TOK-03/TOK-04).
- Context trimming policy changes.
- Additional provider implementations beyond Ollama in this phase.

---

## File Map

| File | Action | Why |
|------|--------|-----|
| `src/agentx/providers/base.py` | New | `ILLMServiceProvider` protocol |
| `src/agentx/providers/ollama_provider.py` | New | Ollama model list + context capacity retrieval |
| `src/agentx/providers/__init__.py` | New | provider exports |
| `src/agentx/model_metadata_store.py` | New | runtime model/capacity store + disk cache |
| `src/agentix/constants.py` | Modify | add `OLLAMA_SHOW_ENDPOINT`, `FALLBACK_CONTEXT_WINDOW` |
| `src/agentix/models.py` | Modify | use provider context capacity, not `parse_parameter_size` |
| `src/agentx/session.py` | Modify | startup store population + model-change redraw |
| `src/agentx/igui_manager.py` | Modify | add `update_context_meter` contract |
| `src/agentx/gui/gui_manager.py` | Modify | add `update_context_meter` stub |
| `src/shared/models/context.py` | Modify | add `token_breakdown(...)` |
| `src/shared/models/message.py` | Verify/Modify | ensure `metadata` dict exists |
| `tests/test_llm_service_provider.py` | New | provider behavior tests |
| `tests/test_model_metadata_store.py` | New | startup + cache behavior tests |
| `tests/test_context_token_breakdown.py` | New | token band calculations |
| `tests/test_active_model_meter_wiring.py` | New | active model -> redraw wiring |

---

## Phase 1 - Baseline Audit and Contracts

- [/] **1.1** Trace all usage of `parse_parameter_size` and `get_model` in
  `src/agentix/models.py`; document callers.
- [/] **1.2** Confirm `session.active_model` setter and callback wiring path in
  `src/agentx/session.py`.
- [/] **1.3** Confirm `src/agentx/session_state.py` model setter runtime/test role.
- [/] **1.4** Confirm insertion point for `update_context_meter` in
  `src/agentx/igui_manager.py`.
- [/] **1.5** Validate import topology to avoid circular dependency between
  `src/agentix/` and `src/agentx/providers/`.
- [/] **1.6** Define disk cache schema and validation rule (model set equality).

---

## Phase 2 - Provider Abstraction and Startup Metadata Store

- [/] **2.1** Add constants in `src/agentix/constants.py`:
  - `OLLAMA_SHOW_ENDPOINT = "/api/show"`
  - `FALLBACK_CONTEXT_WINDOW = 4096`

- [/] **2.2** Create `ILLMServiceProvider` protocol in `src/agentx/providers/base.py`:
  - `list_models() -> list[str]`
  - `get_context_length(model_name: str) -> int`
  - `get_model_metadata(model_name: str) -> dict[str, str | int]`

- [/] **2.3** Create `OllamaServiceProvider` in `src/agentx/providers/ollama_provider.py`:
  - `list_models()` via `/api/tags`
  - `get_context_length()` via `/api/show` key probing:
    1. `llama.context_length`
    2. `context_length`
    3. `num_ctx`
    4. any key ending with `.context_length`
  - fallback to `FALLBACK_CONTEXT_WINDOW`

- [/] **2.4** Create `ModelMetadataStore` in `src/agentx/model_metadata_store.py`:
  - in-memory maps for capacities + metadata
  - disk cache load/save
  - `populate(force: bool = False)` startup behavior
  - model-set comparison logic
  - thread lock for safe concurrent access

- [/] **2.5** Wire startup population in `AgentXSession.__init__`:
  - create provider
  - create metadata store
  - call `populate()` before UI main loop

- [/] **2.6** Update `src/agentix/models.py:get_model()` to use provider capacity
  lookup instead of `parse_parameter_size` for max-context semantics.

---

## Phase 3 - Context Breakdown API (ARCH-01)

- [/] **3.1** Add `Context.token_breakdown(model_name: str = "") -> dict[str, int]`.
- [/] **3.2** Ensure `Message.metadata: dict[str, Any]` exists with safe default.
- [/] **3.3** Tag working-memory system message with
  `metadata["is_working_memory"] = True` in session context assembly.

---

## Phase 4 - GUI Contract Hook (ARCH-05)

- [/] **4.1** Add `update_context_meter(max_tokens: int, breakdown: dict[str, int]) -> None`
  to `IGUIManager` protocol.
- [/] **4.2** Add no-op/logging stub in `GUIManager` so session wiring is safe before
  visual widget exists.

---

## Phase 5 - Session Wiring (ARCH-10)

- [/] **5.1** Extend `active_model` setter:
  - no-op guard when unchanged
  - update config + adapter model
  - compute breakdown via `Context.token_breakdown(...)`
  - read denominator from `ModelMetadataStore.get_context_length(model)`
  - call `gui.update_context_meter(...)` immediately

- [/] **5.2** Ensure redraw still occurs for:
  - REQ-05 prompt submit
  - REQ-06 context toggle
  - REQ-07 stream completion

- [/] **5.3** Confirm stream-completion redraw path uses scheduled UI update helper.

---

## Phase 6 - Thread Safety (ARCH-06)

- [/] **6.1** Add `_schedule_meter_redraw(max_tokens, breakdown)` helper in session.
- [/] **6.2** Verify no direct Tk mutation from non-main thread.
- [/] **6.3** Verify `ModelMetadataStore` lock behavior under concurrent reads/writes.

---

## Phase 7 - Tests

### 7.1 Provider tests (`tests/test_llm_service_provider.py`)

- [/] **7.1.1** list models success
- [/] **7.1.2** list models network failure returns empty
- [/] **7.1.3** context-length key probe priority (parametrized)
- [/] **7.1.4** fallback on network error
- [/] **7.1.5** fallback on malformed payload
- [/] **7.1.6** metadata extraction

### 7.2 Metadata store tests (`tests/test_model_metadata_store.py`)

- [/] **7.2.1** populate with no cache fetches all
- [/] **7.2.2** identical model set uses cache and skips per-model re-query
- [/] **7.2.3** added model fetches only missing capacities
- [/] **7.2.4** removed model prunes stale entries
- [/] **7.2.5** force refresh bypasses cache
- [/] **7.2.6** cache file schema persistence
- [/] **7.2.7** fallback when model missing in store

### 7.3 Context breakdown tests (`tests/test_context_token_breakdown.py`)

- [/] **7.3.1** empty context returns zeroed bands
- [/] **7.3.2** ratio mapping by model family (parametrized)
- [/] **7.3.3** disabled messages excluded
- [/] **7.3.4** working memory split from system
- [/] **7.3.5** tool roles aggregated into tool band
- [/] **7.3.6** attachments counted in attachments band

### 7.4 Wiring tests (`tests/test_active_model_meter_wiring.py`)

- [/] **7.4.1** active model change triggers immediate redraw with new denominator
- [/] **7.4.2** unchanged model is no-op
- [/] **7.4.3** fallback denominator used when capacity unavailable
- [/] **7.4.4** stream done uses scheduled redraw path

---

## Phase 8 - Docs and Release Readiness

- [/] **8.1** ~~Update `docs/ux/context_visualizer.md` to mark PRE-02 complete.~~ `context_visualizer.md` deleted; content absorbed into `03_PANEL_DETAILS.md §PD-10` (REQ-08 marked ✅).
- [/] **8.2** Update `docs/architecture.md` with provider abstraction + metadata store.
- [/] **8.3** Update `CHANGELOG.md` with PRE-02 changes.
- [/] **8.4** Bump version in `pyproject.toml`: `0.18.26 -> 0.19.0`.
- [/] **8.5** Commit all changes to working branch.

---

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|------|-----------|-----------|
| Startup delay while querying all model capacities | Medium | Use disk cache and skip re-query when model set unchanged |
| Stale cache when model internals change but name does not | Low | Support forced refresh path (`populate(force=True)`) |
| Provider API payload variability | Medium | Keep parsing/key-probing inside provider implementation only |
| Circular imports across source trees | Low | validate in Phase 1; move base protocol to shared package if needed |
| Threading errors during redraw | Low | centralize scheduling via `root.after(0, ...)` |

---

## Definition of Done

- All Phase 1-8 items are marked complete (`[/]`).
- New test files pass and full test suite passes without regressions.
- PRE-02 is marked complete in UX spec.
- Architecture docs, changelog, and version are updated.
- Changes are committed.

---

## Audit Notes

### 1.1 `parse_parameter_size` / `get_model` usage map

- `parse_parameter_size` is used inside `src/agentix/models.py:get_model()` to derive
  `max_tokens` from `model["details"]["parameter_size"]`.
- `get_model(args)` callers found:
  - `src/agentix/agent.py` (`max_tokens = get_model(args)`)
  - `src/agentix/bridge/tool_loop.py` (`self._max_tokens = get_model(self.config)`)
- This confirms the current denominator source is still the parameter-size proxy.

### 1.2 `active_model` runtime setter path

- Runtime source of truth is `AgentXSession.active_model` in `src/agentx/session.py`.
- Setter currently updates:
  - `self._active_model`
  - `self.config["agentx"]["ollama_model"]`
  - `self.agentix_adapter.agentix_config.model`

### 1.3 `SessionState.active_model` role

- `SessionState` defines its own `active_model` property in
  `src/agentx/session_state.py`, but `src/agentx/session.py` does not read or write
  `self._state.active_model` in runtime flow.
- Current role appears data-model scaffolding / compatibility, not active runtime model
  propagation.

### 1.4 GUI callback wiring and insertion point

- Model-change callback chain is centered in `src/agentx/session.py:_setup_agentix_ui()`:
  `self.gui.set_model_change_callback(on_model_change)` and callback body sets
  `self.active_model = model`.
- `IGUIManager.update_context_meter` does not yet exist.
- Insertion point in `src/agentx/igui_manager.py`: after the plan update methods
  (`update_plan_synthesis` / `mark_plan_node_invalidated`) and before callback
  registration methods.

### 1.5 Import topology / circular risk

- Current dependency direction is primarily `agentx -> agentix`.
- No existing `agentix -> agentx` imports were found.
- Planned `src/agentix/models.py -> agentx.providers.ollama_provider` introduces a new
  reverse edge; this is acceptable if provider modules avoid importing from
  `agentix.__init__` or session-layer modules.
- Keep provider dependencies narrow (stdlib + HTTP client + optional direct constants)
  to avoid import cycles.

### 1.6 Disk cache presence and schema rule

- No existing `sessions/_model_cache.json` was found in workspace.
- Cache validity rule remains: model-set equality between provider `list_models()` and
  cache `models`.
