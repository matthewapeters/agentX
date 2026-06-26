# Dynamic Context Resizing — Behavioral Specification (PRE-02)

**Status:** Complete
**Date:** 2026-04-26
**Linked spec:** `docs/ux/03_PANEL_DETAILS.md §PD-10` (ContextMeterWidget cut-sheet)
**Linked ARCH items:** ARCH-02, ARCH-05, ARCH-06, ARCH-08, ARCH-09, ARCH-10
**Linked REQ:** REQ-03, REQ-05, REQ-06, REQ-07, REQ-08
**Token strategy:** TOK-02 (model-family char/token ratios)

---

## Objective

Ensure the context meter denominator is always the actual context-window size of the
currently selected model, and refreshes immediately when the user switches models.

The denominator must not be derived from a model's parameter size (e.g. `7B`), which is
not context capacity. This spec describes provider-backed capacity data: the application
queries the active LLM provider for real context window sizes.

The LLM provider abstraction is shared across all application components so that model
listing and capacity lookup are backend-agnostic (Ollama now, extensible later).

---

## Success Criteria

- On startup, the app queries the active LLM provider for all model names and capacities.
- Startup data is stored in an in-memory model metadata store used by both model selector
  and context visualizer.
- A disk cache file `sessions/_model_cache.json` is used across app runs.
- If model names match the cache on startup, capacities are loaded from cache without
  re-querying each model capacity endpoint.
- Model change triggers immediate meter redraw.
- Denominator never uses parameter size as a proxy for context window.
- Redraw calls from non-UI threads are scheduled safely (no direct UI mutation from background threads).
- Tests cover startup enumeration, cache reuse, cache refresh, fallback behavior, and
  model-change redraw wiring.

---

## Decisions on Ambiguities

| Ref | Decision |
|-----|----------|
| **Q-08** | On startup, query all models and capacities, store for runtime use. Add disk cache; if model set is unchanged on startup, reuse cached capacities and skip per-model capacity re-query. |
| **Q-09** | Yes, model-change event must trigger immediate meter redraw. |

---

## Scope

### In scope

- LLM provider interface and Ollama implementation.
- Startup model enumeration and capacity prefetch into a model metadata store.
- Disk cache support in `sessions/_model_cache.json`.
- Immediate redraw on active model change.
- Context meter numerator bands via token breakdown.
- Unit + integration + functional tests for new behavior.

### Out of scope

- Full visual ContextMeterWidget implementation.
- Tokenizer-based counting migration (TOK-03/TOK-04).
- Context trimming policy changes.
- Additional provider implementations beyond Ollama in this phase.

---

## Ollama Provider Behavior

The Ollama provider must:

- List models via `/api/tags`
- Look up context window capacity via `/api/show`, probing keys in priority order:
  1. `llama.context_length`
  2. `context_length`
  3. `num_ctx`
  4. any key ending with `.context_length`
- Fall back to a configurable constant `FALLBACK_CONTEXT_WINDOW = 4096` when no
  capacity key is found or the endpoint is unreachable.

---

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|------|-----------|-----------|
| Startup delay while querying all model capacities | Medium | Use disk cache and skip re-query when model set unchanged |
| Stale cache when model internals change but name does not | Low | Support forced refresh path |
| Provider API payload variability | Medium | Keep parsing/key-probing inside provider implementation only |
| Threading errors during redraw | Low | Centralize scheduling so UI updates occur on the UI thread |

---

## Definition of Done

- Model metadata store populates on startup and serves capacity data to both model selector and context meter.
- Disk cache is read on startup and written after enumeration; cache miss triggers a full re-query.
- Model change event triggers immediate meter redraw with correct denominator.
- Fallback denominator is used when capacity is unavailable.
- All test scenarios pass without regressions.
- Architecture docs and changelog are updated.
