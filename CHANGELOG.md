# Changelog

All notable changes to AgentX are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

---

## [0.20.0] - 2026-04-28

### Code Changes

#### Added

- `src/agentx/gui/context_meter_widget.py` — `ContextMeterWidget` (ARCH-04): donut chart Tkinter widget showing LLM context window utilisation by category. Features: seven band arcs (BAND-01–07), ghost arc for remaining capacity (ENH-02), border ring with three risk states (default / warning ≥80% / critical ≥100%, ENH-16), center percentage label with matching risk color, hover tooltips via `tk.Toplevel` (ENH-06), thread-safe `update()` via `canvas.after(0, ...)` (ARCH-06).
- `src/agentx/widget_registry.py` — added `context_meter_canvas: Optional[tk.Canvas]` field and corresponding `destroy_all()` teardown.

#### Changed

- `src/agentx/gui/input_panel.py` — imports and instantiates `ContextMeterWidget`, calls `create()` inside `InputPanel.create()`, and registers `context_meter_canvas` in the widget registry.
- `src/agentx/gui/gui_manager.py` — replaced `update_context_meter()` stub with real delegation to `self._input_panel.context_meter.update(max_tokens, breakdown)`.

### Test Changes

#### Added

- `tests/test_context_meter_widget.py` — 39 hermetic unit tests covering:
  - GIVEN new widget / WHEN `create()` called / THEN canvas placed at correct geometry
  - GIVEN `update()` on background thread / WHEN called / THEN `canvas.after(0, ...)` invoked (thread safety)
  - GIVEN each of seven band categories / WHEN `_render()` / THEN PIESLICE arc drawn with correct fill color (parameterized × 7)
  - GIVEN 50% usage / WHEN `_render()` / THEN ghost arc drawn with `_GHOST_COLOR`
  - GIVEN 100% usage / WHEN `_render()` / THEN no ghost arc drawn
  - GIVEN empty breakdown / WHEN `_render()` / THEN only ghost arc fills ring
  - GIVEN usage at each risk threshold / WHEN `_render()` / THEN border ring has correct color and width (parameterized × 8: 0%, 50%, 79%, 80%, 95%, 99%, 100%, 120%)
  - GIVEN usage at each risk threshold / WHEN `_render()` / THEN center label has correct fill color (parameterized × 4)
  - GIVEN 40% usage / WHEN `_render()` / THEN center label text is `"40%"`
  - GIVEN absurdly large token count / WHEN `_render()` / THEN center label capped at `"999%"`
  - GIVEN `max_tokens=0` / WHEN `_render()` / THEN no crash
  - GIVEN canvas not yet laid out / WHEN `_render()` / THEN retries via `canvas.after(50, ...)`
  - GIVEN each band / WHEN tooltip hover / THEN tooltip text contains band label and percentage (parameterized × 5)
  - GIVEN 50% usage / WHEN ghost arc hover / THEN tooltip shows "Remaining capacity" and token count
  - GIVEN destroyed Toplevel / WHEN `_hide_tooltip()` / THEN TclError silently swallowed
  - GIVEN no active tooltip / WHEN `_hide_tooltip()` / THEN no exception

## [0.19.3] - 2026-04-27

### Code Changes

#### Changed

- `src/agentix/models.py` now returns supplied cached `max_tokens` before any live model discovery so cached context-length lookups remain usable even when Ollama tag enumeration is unavailable.
- `src/agentix/bridge/tool_loop.py`, `src/agentix/bridge/bridge.py`, `src/agentx/integration/agentix_bridge_adapter.py`, and `src/agentx/session.py` now explicitly invalidate the bridge max-token cache when the active model changes, keeping prompt trimming aligned with the selected model.

#### Fixed

- Fixed the review regression where `get_model(..., max_tokens=...)` still hit `/api/tags` before honoring the cached value.
- Fixed stale tool-loop max-token caching after model switches, which could leave Agentix trimming against the previous model's context window.

### Test Changes

#### Added

- Hermetic regression tests proving cached `max_tokens` bypasses live model discovery and proving model changes invalidate the bridge/tool-loop max-token cache.

#### Fixed

- Targeted regression coverage for the corrected model-selection path remains at 98% for `agentix.models` with new cache-invalidation behaviors covered by hermetic unit tests.

## [0.19.2] - 2026-04-27

### Code Changes

#### Added

- `src/shared/providers/` introducing a shared provider boundary so `agentix` and `agentx` can both consume the same `ILLMServiceProvider`, constants, and Ollama adapter without reverse imports.
- `tests/test_agentix_models.py` and `tests/test_tool_loop_max_tokens.py` covering model-selection failures, cached max-token reuse, and tool-loop max-token wiring.

#### Changed

- `src/agentix/models.py` now hardens Ollama model enumeration, rejects malformed payloads, raises a clear error when no models match, and uses cached `max_tokens` when supplied.
- `src/agentix/bridge/tool_loop.py`, `src/agentix/agentix_config.py`, and `src/agentx/session.py` now propagate cached context-length values into the tool loop so Agentix avoids redundant live lookups when AgentX already knows model capacity.
- `src/agentx/protocols.py`, `src/agentx/session.py`, and `src/agentx/streaming_controller.py` now expose public context-meter protocol methods while keeping compatibility wrappers for existing call sites.
- `src/agentx/model_metadata_store.py` now exposes `population_failed` alongside `populated` so callers can distinguish completion from successful population.
- `src/agentx/providers/*` now act as compatibility wrappers over the shared provider implementation.

#### Fixed

- Removed the reverse dependency from `agentix` into `agentx.providers`, eliminating the reviewed layering violation.
- Fixed unhandled request/JSON failures and malformed response handling in Ollama model discovery.
- Fixed weak parameter-size validation and empty-model handling in Agentix model selection.
- Fixed the bridge max-token path so cached context lengths are actually consumed by the tool loop.

### Test Changes

#### Added

- Hermetic unit coverage for malformed Ollama payloads, fallback provider paths, model metadata cache failure semantics, public/compatibility meter APIs, and cached max-token routing into the tool loop.

#### Fixed

- Targeted hermetic coverage for the repaired core modules now reaches 98% (`agentix.models`, `agentx.model_metadata_store`, `shared.providers.ollama_provider`).

## [0.19.1] - 2026-04-27

### Code Changes

#### Added

- `src/agentx/providers/constants.py` introducing provider-scoped constants (`OLLAMA_MODELS_ENDPOINT`, `OLLAMA_SHOW_ENDPOINT`, `FALLBACK_CONTEXT_WINDOW`) to remove cross-tree imports.
- `src/agentx/protocols.py` introducing runtime-checkable `IMeterSession` for explicit context-meter contracts.
- `src/shared/token_utils.py` with module-level `chars_per_token()` and `estimate_text_tokens()` utilities.

#### Changed

- `src/agentx/providers/base.py` adds required `provider_id` contract to `ILLMServiceProvider`.
- `src/agentx/providers/ollama_provider.py` now exposes `provider_id = "ollama"`, accepts optional host values, and normalizes `None`/empty hosts safely.
- `src/agentx/model_metadata_store.py` now uses provider `provider_id` in cache payloads, exposes `populated: threading.Event`, unifies cache parsing with `_parse_cache_data()`, and adds `invalidate(model_name: str | None = None)` background refresh support.
- `src/agentx/session.py` now imports provider constants from `agentx.providers.constants`, starts model-store population asynchronously at startup, adds `on_context_assembled(shared_context)`, and simplifies `_context_meter_payload()` error-handling and model-name fallback semantics.
- `src/agentx/streaming_controller.py` replaces `hasattr` meter guards with `isinstance(..., IMeterSession)` checks and delegates assembled-context meter redraw via `on_context_assembled()`.
- `src/shared/models/context.py` now routes `MessageRole.SYNTHESIS` into the assistant meter band and uses shared token utilities while retaining compatibility shims.
- `src/agentix/models.py` adds optional fast-path `max_tokens` argument to avoid redundant live context-length HTTP calls.
- `pyproject.toml` test config now includes `pythonpath = ["src"]` to eliminate per-test `sys.path` mutation.

#### Fixed

- Addressed all 15 PR #5 review findings (A1-A7, P1-P8), including provider abstraction, cache semantics, meter protocol boundaries, startup threading behavior, and context-meter correctness.

### Test Changes

#### Added

- `tests/test_token_utils.py` (Unit):
  - GIVEN model-name families and text samples
  - WHEN token utility helpers run
  - THEN family ratios and ceiling token estimates are validated.
- `tests/test_protocols.py` (Unit):
  - GIVEN full and partial structural implementations
  - WHEN runtime protocol checks execute
  - THEN `IMeterSession` compatibility is correctly enforced.

#### Changed

- `tests/test_llm_service_provider.py`:
  - Removed `sys.path.insert` usage and switched constants import to `agentx.providers.constants`.
  - Added `provider_id` assertion and parametrized host-normalization coverage (`None`, empty, bare host, http/https variants).
- `tests/test_model_metadata_store.py`:
  - Removed `sys.path.insert`, updated constants import, and added `provider_id` to test provider.
  - Added coverage for `populated` event behavior, failure-path event setting, `invalidate()` single/all flows, provider-id cache serialization, and `_parse_cache_data()`.
- `tests/test_context_token_breakdown.py`:
  - Removed `sys.path.insert`.
  - Added explicit `SYNTHESIS` role routing assertions into `assistant` band.
- `tests/test_active_model_meter_wiring.py`:
  - Removed `sys.path.insert`, updated constants import.
  - Added `on_context_assembled()` meter-redraw behavior coverage.

#### Fixed

- New/updated targeted tests for PR #5 review scope now pass (`50 passed, 0 failed`).

## [0.19.0] - 2026-04-26

### Code Changes

#### Added

- `src/agentx/providers/base.py` with `ILLMServiceProvider` protocol.
- `src/agentx/providers/ollama_provider.py` implementing model enumeration and context-length lookup via Ollama endpoints.
- `src/agentx/model_metadata_store.py` for startup-populated, disk-backed model capacity metadata (`sessions/_model_cache.json`).

#### Changed

- `src/agentix/constants.py` adds `OLLAMA_SHOW_ENDPOINT` and `FALLBACK_CONTEXT_WINDOW`.
- `src/agentix/models.py` now derives `max_tokens` from provider context-length lookup instead of `parameter_size` proxy.
- `src/agentx/session.py` now initializes provider/store at startup, tags working-memory system messages via metadata, adds meter payload/scheduling helpers, and triggers redraw on model change, submit, and attachment-toggle events.
- `src/agentx/streaming_controller.py` now schedules meter redraw on submit-context assembly and after stream completion.
- `src/agentx/igui_manager.py` and `src/agentx/gui/gui_manager.py` now include `update_context_meter(max_tokens, breakdown)` contract/stub.
- `src/shared/models/message.py` now includes serializable `metadata` map.
- `src/shared/models/context.py` adds `token_breakdown(model_name)` with TOK-02 model-family ratios.
- `docs/context_size_prerequisite_plan.md` updated with Phase 1 audit findings and implementation progress tracking.
- `docs/ux/context_visualizer.md` marks PRE-02 complete and updates dynamic-context definition notes.
- `docs/architecture.md` module map updated with provider abstraction and metadata-store runtime flow.

#### Fixed

- Context-meter denominator source now aligns with active-model context window semantics instead of parameter-size heuristics.

### Test Changes

#### Added

- `tests/test_llm_service_provider.py` (Unit):
  - GIVEN Ollama tag/show payloads
  - WHEN provider methods are called
  - THEN model names, key-probe ordering, and fallback behavior are validated.
- `tests/test_model_metadata_store.py` (Unit):
  - GIVEN cache/no-cache startup conditions
  - WHEN `populate()` executes
  - THEN fetch/cached/fallback behavior is validated.
- `tests/test_context_token_breakdown.py` (Unit):
  - GIVEN enabled context messages and attachments
  - WHEN `token_breakdown()` executes
  - THEN role-band and attachment token estimates are correctly routed.
- `tests/test_active_model_meter_wiring.py` (Integration):
  - GIVEN active-model changes in session
  - WHEN setter logic runs
  - THEN GUI meter updates receive denominator and breakdown, including no-op and fallback paths.

#### Changed

- No existing tests removed; new PRE-02 tests run alongside the existing suite.

#### Fixed

- N/A

#### Removed

- N/A

## [0.18.26] - 2026-04-26

### Code Changes

#### Fixed

- **F-1 (HIGH)** `src/agentx/streaming_controller.py` — replay candidate lists (`original_tool_calls`, `original_tool_results`, `original_assistants`) now scoped to the replaying `task_id` via `and msg.task_id == _tid` filter, preventing cross-turn message contamination.
- **F-1 (HIGH)** `src/agentx/streaming_controller.py` — `_display_tool_call` and `_display_tool_result` now accept `task_id: str | None = None` and stamp it on persisted messages; streaming path passes the current task_id via a mutable box (`_current_task_id`); replay path passes `_tid` explicitly.
- **F-2 (HIGH)** `src/agentx/session.py` — `_persist_stream_messages` delegate restored missing `synthesis_of: list[str] | None = None` parameter and switched to keyword-based delegation to avoid positional argument corruption (the `refresh_gui` bool was being received as `synthesis_of`).
- **F-3 (MEDIUM)** `src/shared/models/message.py` — `from_dict` coerces non-list `synthesis_of` values to `[]` instead of storing them; `__post_init__` raises `ValueError` if `synthesis_of` is not a `list`.

### Test Changes

#### Added

- **Integration** `tests/test_replay_message_lineage_e2e.py` — T-1: `test_replay_does_not_supersede_prior_turn_messages` — verifies replay of a second task leaves prior-turn messages untouched.

  ```gherkin
  GIVEN a context with a prior turn (task_id=prior) and a replay target (task_id=target)
  WHEN the replay worker runs for task_id=target
  THEN messages from the prior turn (task_id=prior) are not superseded or modified
  ```

- **Integration** `tests/test_replay_message_lineage_e2e.py` — T-2: `test_replay_does_not_supersede_concurrent_plan_step_messages` — verifies replay of Step A in a two-step plan leaves Step B messages untouched.

  ```gherkin
  GIVEN a context with two plan steps (step_a_task_id, step_b_task_id)
  WHEN the replay worker runs for step_a_task_id
  THEN messages belonging to step_b_task_id are not superseded or modified
  ```

- **Integration** `tests/test_streaming_message_id_integration.py` — T-3: `test_persist_stream_messages_synthesis_of_is_list_type` — verifies the assistant message created by `_persist_stream_messages` has `isinstance(synthesis_of, list) == True`.

  ```gherkin
  GIVEN a StreamingController with a real Context
  WHEN _persist_stream_messages is called with synthesis_of=["msg_abc..."]
  THEN the persisted ASSISTANT message has synthesis_of as a list type
  ```

- **Integration** `tests/test_streaming_message_id_integration.py` — T-4: `test_session_delegate_forwards_synthesis_of_by_keyword` — verifies `session._persist_stream_messages` forwards `synthesis_of` and `refresh_gui` as keyword arguments to the controller.

  ```gherkin
  GIVEN a partially-initialized AgentXSession with a spy StreamingController
  WHEN session._persist_stream_messages(thinking, content, synthesis_of=[...], refresh_gui=False) is called
  THEN the spy records synthesis_of=[...] and refresh_gui=False (not positionally swapped)
  ```

- **Unit** `tests/test_message_model_ids.py` — T-5 (parametrized × 5): `test_from_dict_coerces_non_list_synthesis_of_to_empty_list` — verifies `Message.from_dict` coerces `True`, `False`, a bare string, `42`, and a dict to `[]`.

  ```gherkin
  GIVEN a message payload with synthesis_of=<non-list value>
  WHEN Message.from_dict(payload)
  THEN message.synthesis_of == [] and isinstance(message.synthesis_of, list) is True
  Permutations: True, False, "msg_a1b2c3...", 42, {"key": "value"}
  ```

- **Unit** `tests/test_message_model_ids.py` — `test_post_init_rejects_non_list_synthesis_of` — verifies `Message(..., synthesis_of=True)` raises `ValueError` mentioning `synthesis_of`.

  ```gherkin
  GIVEN synthesis_of=True (a boolean, not a list)
  WHEN Message(role=..., content=..., synthesis_of=True)
  THEN ValueError is raised with a message about synthesis_of type
  ```

#### Changed

- `tests/test_replay_message_lineage_e2e.py` — `_make_original_group` now stamps `task_id="task-1"` on all original messages so the scoped replay filter matches them correctly.

---

## [0.18.25] - 2026-04-25

### Code Changes

#### Changed

- `src/agentx/streaming_controller.py` — replay worker now persists lineage for replayed messages: replay `TOOL_CALL`/`TOOL_RESULT` messages set `cloned_from`, replay synthesis `ASSISTANT` sets `cloned_from` and `synthesis_of`, and supersession is applied only after replay outputs are fully persisted.
- `src/agentx/streaming_controller.py` — replay completion now applies `Context.supersede_message(...)` mappings so originals are disabled and point to replacements via `superseded_by`.

### Test Changes

#### Added

- **Integration/Functional** `tests/test_replay_message_lineage_e2e.py`: 20 parametrized tests covering replay lineage and supersession behavior end-to-end.

  ```gherkin
  GIVEN a replayed message group
  WHEN replay succeeds
  THEN replacements set cloned_from and originals set superseded_by
  ```

  ```gherkin
  GIVEN a replay attempt fails at any persistence stage
  WHEN replay exits with error
  THEN originals remain enabled and no superseded_by links are applied
  ```

  ```gherkin
  GIVEN replay emits tool results
  WHEN replay synthesis is persisted
  THEN synthesis_of references replay TOOL_RESULT message_ids only
  ```

  ```gherkin
  GIVEN replay replacement generation is persisted
  WHEN supersession completes
  THEN originals are disabled and replacements are enabled
  ```

  ```gherkin
  GIVEN multi-generation replay lineage
  WHEN get_ancestry is requested for the latest generation
  THEN ancestry is root-to-leaf and complete
  ```

  ```gherkin
  GIVEN replay supersession updates under interleaving patterns
  WHEN mappings are applied
  THEN supersession targets remain deterministic and lineage-safe
  ```

## [0.18.24] - 2026-04-20

### Code Changes

#### Added

- `tests/test_chat_panel_turn_rendering.py`: 10 new integration tests verifying correct pack-order of conversation-turn widgets in `ChatPanel`.  Tests cover: first-render order, full turn sequence (user → classify → think → respond), collapse/expand cycle, and multiple consecutive turns (parametrized: 1, 2, 3 turns).

#### Fixed

- `src/agentx/gui/chat_panel.py` — `_ensure_turn_started()`: moved `children.pack(fill=tk.X, …)` to *after* `_create_output_entry()` so Tkinter packs the user-entry frame before the children frame.  Previously `children` was packed first (index 0 in the slave list), causing all classification/thinking/tool/assistant entries to render *above* the user prompt on first render.  Collapse → expand accidentally "fixed" this because `pack_forget()` + `pack()` appended children to the end of the list.
- `src/agentx/gui/markdown_renderer.py` — `markdown_to_html()`: added guard for `_md_lib is None` so the function produces valid HTML (`<pre>` fallback) even when the `markdown` package is not installed.  Previously any call with `MARKDOWN_AVAILABLE=True` but missing library raised `AttributeError`.
- `tests/test_markdown_rendering.py` — `test_full_path_with_mocked_html_frame`: patched `agentx.gui.chat_panel.markdown_to_html` with a stub that returns `<table>` HTML so the test is self-contained regardless of whether the `markdown` package is installed.
- `tests/test_gui_manager_integration.py` — `test_header_preview_not_driven_by_newline`: removed incorrect assertion that header preview text varies by pixel width; the `_header_preview()` method truncates by word count (>15 words), not pixel width.
- `tests/integration/test_bootstrap_e2e.py` — `TestBootstrapDefaults`: fixed 6 methods that referenced undefined bare variables (`agentx`, `cm`, `candidates`, `prompt_path`, `instructions_path`); replaced with correct `toml_config` fixture access.

#### Changed

- `pyproject.toml`: added pytest markers `unit`, `functional`, `integration` to the `[tool.pytest.ini_options]` markers list.

### Test Changes

#### Added

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_user_entry_packed_before_children_frame_on_first_render`

  ```gherkin
  GIVEN a GUIManager with a chat panel
  WHEN a user message is sent (first render)
  THEN the user entry frame appears before the children frame in Tkinter's pack slave list
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_children_frame_packed_after_user_entry`

  ```gherkin
  GIVEN a conversation turn has been started
  WHEN inspecting pack order of turn_frame children
  THEN user entry pack-index < children frame pack-index
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_classify_entry_appended_to_children_not_turn_frame`

  ```gherkin
  GIVEN a user message has been displayed
  WHEN display_classify is called
  THEN the classification entry is a child of the children_frame, not the turn_frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_assistant_entry_appended_to_children_not_turn_frame`

  ```gherkin
  GIVEN a user message has been displayed
  WHEN display_agent_response is called
  THEN the assistant entry is parented to the children_frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_children_frame_is_indented`

  ```gherkin
  GIVEN a conversation turn
  WHEN inspecting the children_frame's pack configuration
  THEN padx has a non-zero left indent to visually nest responses under the user message
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_full_turn_sequence_pack_order`

  ```gherkin
  GIVEN a GUIManager with a fully laid-out chat panel
  WHEN a user message, classification, thinking, and agent response are all displayed
  THEN user_entry_frame is the first child of turn_frame, children_frame is the second
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_expand_after_collapse_preserves_correct_order`

  ```gherkin
  GIVEN a conversation turn has been rendered correctly
  WHEN the user entry is collapsed then expanded
  THEN the children_frame remains below the user_entry_frame in pack order
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_new_turn_starts_fresh_frame`

  ```gherkin
  GIVEN a first conversation turn is complete
  WHEN a second user message is displayed
  THEN a new turn_frame is created and the children_frame is correct within that new frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestMultipleTurnsRenderingOrder::test_multiple_turns_correct_order[1 turn]`

  ```gherkin
  GIVEN 1 user message and agent response
  WHEN rendered
  THEN user entry is before children frame in the turn's pack slave list
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestMultipleTurnsRenderingOrder::test_multiple_turns_correct_order[2 turns]`

  ```gherkin
  GIVEN 2 consecutive user messages each with an agent response
  WHEN rendered
  THEN every turn has user entry before children frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestMultipleTurnsRenderingOrder::test_multiple_turns_correct_order[3 turns]`

  ```gherkin
  GIVEN 3 consecutive user messages each with an agent response
  WHEN rendered
  THEN every turn has user entry before children frame
  ```

#### Fixed

- **Integration** `test_markdown_rendering.py::TestMarkdownRenderingHeadless::test_full_path_with_mocked_html_frame`

  ```gherkin
  BEFORE: AttributeError on _md_lib.markdown when markdown package is not installed
  AFTER:  markdown_to_html is patched so the test asserts <table> regardless of package availability
  ```

- **Integration** `test_gui_manager_integration.py::TestGUIManagerDisplay::test_header_preview_not_driven_by_newline`

  ```gherkin
  BEFORE: AssertionError — test incorrectly assumed header text varies by pixel width
  AFTER:  Test only asserts that newlines are condensed to spaces; truncation threshold is word-count-based
  ```

- **Integration** `tests/integration/test_bootstrap_e2e.py::TestBootstrapDefaults` (6 methods)

  ```gherkin
  BEFORE: NameError: name 'agentx' / 'cm' / 'candidates' / 'prompt_path' / 'instructions_path' is not defined
  AFTER:  All references replaced with correct toml_config fixture access
  ```

---

## [0.18.23] - 2026-04-19

### Code Changes

#### Fixed

- `pyproject.toml`: version tag format changed from PEP 440-incompatible `-letter` suffix (e.g. `0.18.22-i`) to PEP 440-compliant `.postN` post-release form (e.g. `0.18.22.post9`). The old format caused `uv build` to fail with a TOML parse error.
- `.github/copilot-instructions.md`: Semantic Versioning section updated — doc-only version examples changed from `2.3.1-a` / `2.3.1-b` to `2.3.1.post1` / `2.3.1.post2`. All references to the `-letter` alpha tag scheme replaced with `.postN` language.

---

## [0.18.22.post9] - 2026-04-19

### Code Changes

#### Added

- None

#### Changed

- None

#### Fixed

- None

### Documentation Changes

#### Added

- `docs/architecture.md`: new **§12 Context Construction Pipeline** section documenting all five filter layers applied before any message reaches the LLM API:
  - **Layer 0** — `_build_shared_context()` assembly order (WM injection, history, current session)
  - **Layer 1** — `Message.enabled` flag mechanics, including non-obvious `load_from_dir()` default of `False`, the `MessageEntry.__getattr__` proxy pattern, and per-role enabled defaults
  - **Layer 2** — `to_llm_messages()` internal-role exclusion set (`PLAN`, `TASK_NODE`, `SYNTHESIS`, `ASSERTION`) with per-role LLM-sent table
  - **Layer 3** — `to_llm_dict()` attachment filtering (`attachment.enabled == True`)
  - Full pipeline ASCII diagram showing the complete flow from `_build_shared_context` through wire-format output
  - Classification path note showing `classify_prompt()`'s independent `enabled`-only filter
- `docs/architecture.md`: Contents table updated with §12 entry; old §12–16 renumbered to §13–17

---

## [0.18.22.post8] - 2026-04-19

### Code Changes

#### Added

- None

#### Changed

- None

#### Fixed

- None

### Documentation Changes

#### Added

- `docs/ux/03_PANEL_DETAILS.md`: new **PD-11: FileExplorer** section documenting the Files tab widget — navigation bar (Back, Forward, Up, Home, Refresh), path label, three-column treeview, file/folder context menus (Attach, Edit, Add to memory), navigation history state, and related user flow references.
- `docs/ux/03_PANEL_DETAILS.md`: fully expanded **PD-07: SettingsTab** section — replaces the thin key-sections table with complete per-section widget inventories for all five collapsible groups (Appearance, Ollama, Agentix, Classification Display, Working Memory), ASCII mockup, and widget convention table.
- `docs/ux/02_USER_FLOWS.md`: **UF-11: File Explorer Navigation** sequence diagram covering directory navigation, file attach, file edit, folder pin to Working Memory, and history traversal (Back/Forward).
- `docs/ux/01_MAIN_LAYOUT.md`: Detail Diagram References table — added rows linking **Files Tab → PD-11** and **Settings Tab → PD-07**.
- `docs/ux/01_MAIN_LAYOUT.md`: Component Index — added `[→ PD-11]` and `[→ PD-07]` inline links on the Files tab and Settings tab rows.
- `docs/ux/03_PANEL_DETAILS.md`: PD-03 SidePanel — replaced the inline Files Tab and Settings Tab affordance tables with concise `[→ PD-11]` / `[→ PD-07]` forward-references to avoid duplication.

---

## [0.18.22.post7] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation / workspace config only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Added

- `.vscode/extensions.json` — workspace extension recommendations. Marks `vstirbu.vscode-mermaid-preview` and `mermaidchart.vscode-mermaid-chart` as `unwantedRecommendations` because both register Markdown preview renderers that conflict with `bierner.markdown-mermaid`, producing the double-nested "No diagram type detected" error in the Markdown preview. Only `bierner.markdown-mermaid` should be active in this workspace for Mermaid rendering.

---

## [0.18.22.post6] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10: rewrote self-referential arrow labels to remove leading underscores (`_interrupt_flag`, `_is_streaming`). Mermaid processes label text as Markdown, so `_word_` patterns are interpreted as italic markers, corrupting the token stream and causing "No diagram type detected". Replaced with plain descriptive text: `set interrupt flag = True` and `clear is_streaming event`. Also simplified the Note and partial-response label text.

---

## [0.18.22.post5] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10: replaced em dash `—` with a plain hyphen `-` in the `Note` text; the Unicode em dash caused "No diagram type detected" in the VS Code Mermaid preview renderer.

---

## [0.18.22.post4] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10 Mermaid parse error: added missing `participant Chat as ChatPanel` declaration and replaced `\n`-delimited `Note` text with a single-line string (inline `\n` escapes in `Note` blocks are not supported by all Mermaid versions and caused a NEWLINE parse error).

---

## [0.18.22.post3] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — replaced `actor User` with `participant User` in all 9 Mermaid sequence diagrams (UF-01 through UF-10 excluding UF-09). The `actor` keyword is not supported by the Mermaid version bundled with VS Code's markdown preview, causing "No diagram type detected" errors.

---

## [0.18.22.post2] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Window Layout Mockup**: ChatPanel is now shown on the **left (~66%)** and SidePanel on the **right (~34%)**, matching the actual `PanedWindow` widget order in `chat_panel.py` and `side_panel.py`.
- `docs/ux/01_MAIN_LAYOUT.md` — removed incorrect model-selector widget from the OS title bar in the mockup (it lives inside SidePanel, not the title bar).
- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Zone Map** sash table: left pane is ChatPanel (~66%), right pane is SidePanel (~34%); was previously inverted (25%/75%).
- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Component Index** `Left pane` / `Right pane` labels and percentages.
- `docs/ux/01_MAIN_LAYOUT.md` — corrected §4 SidePanel and §5 ChatPanel position strings to match actual sash proportions.
- `docs/ux/01_MAIN_LAYOUT.md` — added `screen_side` clarification note: this setting controls which side of the **monitor** the window is placed on, not the internal panel arrangement.
- `docs/ux/01_MAIN_LAYOUT.md` — added **Detail Diagram References** table beneath the mockup, providing `[→ PD-XX]` annotations on every generalised component and linking to corresponding sections in `03_PANEL_DETAILS.md` (quality gate: generalised components must carry detail diagram references).
- `docs/ux/03_PANEL_DETAILS.md` — corrected **PD-01 ChatPanel** position from "Right ~75%" to "Left ~66% (PanedWindow left pane)".
- `docs/ux/03_PANEL_DETAILS.md` — corrected **PD-03 SidePanel** position from "Left ~25%" to "Right ~34% (PanedWindow right pane)".

---

## [0.18.22.post1] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Added

- `docs/ux/README.md` — new UX documentation index with message-role icon legend and key UX principles.
- `docs/ux/01_MAIN_LAYOUT.md` — main window layout mockup with zone map, component index, and per-panel layout details (SidePanel, ChatPanel, InputPanel).
- `docs/ux/02_USER_FLOWS.md` — Mermaid sequence and flowchart diagrams for all 10 major user flows: basic chat, tool execution, hierarchical task execution, re-synthesis, file attachment, model switch, settings change, session history navigation, working memory management, and interrupt streaming.
- `docs/ux/03_PANEL_DETAILS.md` — per-panel affordance specifications for ChatPanel, InputPanel, SidePanel, ModelSelector, PlanTreeWidget, ResynthesisDialog, SettingsTab, ContextRenderer, CollapsibleSection, and ToolPanel.

#### Changed

- `docs/architecture.md` — full rewrite to reflect current codebase state (2026-04-19). Updated module map tables, class relationship Mermaid diagram, session decomposition (SessionState/StreamingController/ToolDispatcher), GUI decomposition (ChatPanel/InputPanel/SidePanel/ContextRenderer), classification pipeline (with phi4-mini, response_format fix, system-msg exclusion fix), tool pipeline, hierarchical task execution, working memory section, data model schemas, tool schema examples, threading model, persistence layout, configuration reference, and retrieval keywords index.
- `gui_manager.md` — replaced aspirational design doc with current-state documentation of GUIManager as a thin coordinator, describing the IGUIManager Protocol, panel decomposition, WidgetRegistry pattern, and achieved separation-of-concerns table.
- `docs/integration/01_ARCHITECTURE_OVERVIEW.md` — updated all stale module file paths (e.g. `src/agentx/gui_manager.py` → `src/agentx/gui/gui_manager.py`; `src/agentx/context.py` → `src/shared/models/context.py`). Updated component tables for both AgentX and Agentix subsystems to match current code structure. Updated data flow diagrams.

---

## [0.18.22] - 2026-04-18

### Code Changes

#### Fixed

- `src/agentix/query_payload.py` — emit `response_format: {"type": "json_object"}` (OpenAI-compat key) instead of Ollama-native `format: "json"` so classification calls receive structured JSON from all endpoints.
- `src/agentix/bridge/classify_prompt.py` — filter context to `user`/`assistant` roles only before classification (exclude `system` messages carrying working-memory identity to prevent persona contamination).
- `src/agentix/bridge/classify_prompt.py` — pass `system_prompts_dir` from `AgentixConfig` to `PromptLoader` so the classification system prompt is loaded correctly when invoked from the bridge.
- `agentx.toml` — set `agentix_bench_classification_model = "phi4-mini:3.8b"` (neutral model suitable for JSON classification; replaces agent-persona model).
- `system_prompts/prompt_classification.md` — updated to produce valid JSON output conforming to `PromptClassificationResponse` schema.

### Test Changes

#### Added

- `tests/integration/test_bootstrap_e2e.py` — 10 `@pytest.mark.live` end-to-end bootstrap tests verifying the full classification pipeline against a live Ollama instance.
