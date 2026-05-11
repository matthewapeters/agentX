# AgentX – Copilot Instructions

## What This Project Is

AgentX is a local-first AI agent framework with a Tkinter GUI. It connects to **Ollama** (LLM inference) and optionally **Agentix** (code-analysis middleware). Users interact through the GUI; the app streams LLM responses, executes tools, and persists conversations to `sessions/` on disk.

## Quality Control Gates

### PEP 8 Compliant Documentation and Linting

- Apply inline comments and docstrings in compliance with PEP8 standards.
- All modules, classes and methods must have complete docstrings.
- All tests must have Gherkin use-cases in their docstrings (GIVEN...WHEN...THEN...) to explain the nature of the test.
- Perform isort and black linting on all source code and test code to ensure that code is of highest professional quality.
- Use mypy hints for all input and return parameters.  Include parameter types in docstrings.

### All Tests Pass and Provide High % Code Coverage

- Tests must be paramaterized to cover full range of input permutation use-cases.  Each variance of the test should be clearly documented within the test docstrings.  Failing tests should identify the failing use-cases.
- Permutations for parameterized tests and test data should be maintained under the `./tests/` folder.  Ignore the `./tests/` folder in coverage considerations.
- Use Pytest marking on tests to clearly identify `Unit`, `Functional`, `Integration`, and `End-to-End` tests.  Unit, Functional and Integration tests must be created and maintained by the agent.  End-to-end tests should only be created with the explicit direction of the user.
  - Unit tests must be hermetic in nature and must identify the unit they are testing.  Any access to filesystem, networking, or external services is to be faithfully mocked.
  - Integration tests must be hermetic in nature and apply only to the integration of two ore mroe project code units.  Each test must document the various units being tested.  Mocking of filesystem, networking, and external servicves is to be faithfully mocked.
  - Functional tests must clearly identify the internal and external units being tested.  When more than one external system is involved, multiple permutations of the test should be employed such that each external system is tested in isolation, and then with decreasing number of mocked external services.
  - Example: A test that retrieves a file from the internet and writes it to the filesystem should have the following permutations
    - actual internet retrieval, mock write to filesystem
    - mock internet retrieval, actual write to filesystem
    - actual internet retrieval and actual write to filesystem
  - End-to-End tests should only be created at the direction of the user.
- Run all tests, starting with unit tests.  Ensure a minimum of 98% code coverage by unit tests.  Fix problems with changed code or update tests as appropriate so 100% tests pass.  Do not ignore tests that failed before the change.

### Semantic Versioning

- When the agent introduces changes, update the .pyproject.yaml version with the next semantic version given the new changes.  The semantic version increase logic is as follows:
  - When a project change is made to non-code elements such as documentation, architectural diagrams, or UX mockups, and there are no code changes, the patch version should be retained, but a **post-release counter** (`.postN`) should be appended to indicate a non-code change.  This format is PEP 440-compliant and accepted by `uv`, `pip`, and PyPI.  Examples:
    - 2.3.1 --> 2.3.1.post1
    - 2.3.1.post1 --> 2.3.1.post2
  - If the change is a fix to existing code, increase the patch version and drop any `.postN` suffix.  Example: 2.3.1 --> 2.3.2
  - If the change is a new feature, and does not introduce a backwards-compatibility breaking change, increase the minor value by 1 and set the patch value to 0.  Any `.postN` suffix should be removed.
    - Examples:
      - 2.3.2 --> 2.4.0
      - 2.3.1.post2 --> 2.4.0

  - If the change introduces a breaking change for backwards compatibility, increase the major value by 1 and set the minor and patch values to 0.  Any `.postN` suffix should be removed.  If the major version is currently 0, increase the minor version instead and set the patch to 0 (since 0.x versions are considered pre-release and may have breaking changes without a major version bump).
     - Examples:
       - 2.4.0.post3 --> 3.0.0
       - 0.4.0.post2 --> 0.5.0
       - 0.4.0 --> 1.0.0 (if we want to signal a major milestone with the 1.0 release, even if there are breaking changes in the 0.4.0 release that would normally warrant a minor version bump to 0.5.0)


### SCV Commits on Completed Work

- Document a summary of the applied changes as a commit message and commit the changes.  Ensure sure new files and modified files are added to the commit.
- Maintain a changelog file `CHANGELOG.md` and update it with a summary of the changes, the semantic version, and the date of the change.  Follow this format for changelog entries:
  ```

### Session Closure and Anti-Drift Commit Discipline (Required)

- The agent must not end a coding task with agent-authored files left modified and uncommitted unless the user explicitly asks to pause without committing.
- Before any commit, the agent must run quality control gates relevant to the touched scope and report results:
  - Required minimum: targeted tests for changed units and integrations.
  - Required when feasible: full project test run and lint/type gates (`black`, `isort`, `flake8`, `mypy`).
  - If any gate fails, the agent must report the failure details and either fix them in the same session or explicitly record why they remain unresolved.
- The agent must create a local commit for each completed change set. Do not push unless the user explicitly asks to push.
- The agent must treat uncommitted changes created in the same session as in-scope work, never as unrelated "other" changes.
- At session end, the agent must provide a closure report including:
  - files changed
  - gates executed and pass/fail status
  - semantic version update
  - commit hash(es)

#### Commit Hygiene Workflow (Required)

1. Inspect current working tree (`git status --short`).
2. Separate user-preexisting changes from agent-authored changes.
3. Run required quality gates.
4. Update semantic version and `CHANGELOG.md`.
5. Commit agent-authored completed work locally.
6. Confirm remaining uncommitted files (if any) and why they remain.
  ## [Version] - YYYY-MM-DD
  ### Code Changes
  #### Added
  - Summary of new features, units, or additions.

  #### Changed
  - Summary of changes to existing features.

  #### Fixed
  - Summary of bug fixes or issues resolved.

  #### Removed
  - Summary of any features, units, or code that was removed.

  ### Test Changes

  #### Added
  - Summary of new tests added, including the type of test (Unit, Functional, Integration, End-to-End) and the use-cases they cover.
  - Each Gherkin use-case added as used in docstrings

  #### Changed
  - Summary of changes to existing tests or permutations, including any changes to test data or permutations.
  - Each Gherkin use-case changed as found in docstrings (Before, After)

  #### Fixed
  - Summary of any test or permutation failures that were fixed, including the use-cases that were failing and are now passing.
  - Each Gherkin use-case fixed as found in docstrings (Before, After)

  #### Removed
  - Summary of any tests or permutations that were removed, including the reason for removal (e.g. test was redundant, test was for a feature that was removed, etc.)
  - Each Gherkin use-case removed as found in docstrings (Before, After)

  ```

### Architectural Documentation

- Before committing any changes, ensure that the architectural documentation is updated to reflect the changes.
  - This includes updating module maps, class diagrams, sequence diagrams, and any other relevant documentation to ensure it accurately reflects the current state of the codebase.
- Architectural documentation should be clearly organized and easily navigable, with a clear structure that allows developers to quickly find the information they need about the architecture of the system.
  - Use folder structures, markdown files, and diagrams to organize the documentation in a way that is intuitive and easy to navigate.
- Ensure that all architectural documentation is kept up-to-date and accurately reflects the current state of the system.  Outdated documentation can lead to confusion and errors, so it's important to maintain it as part of the development process rather than treating it as an afterthought.

#### Document All Units and Integrations
- Ensure that all modules, classes, methods, and integrations are thoroughly documented with clear explanations of their purpose, functionality, and interactions with other units.  Use docstrings for code-level documentation and markdown files for higher-level architectural documentation.

#### Document UX mockups and user flows
- For any changes that affect the user interface or user experience, update the relevant documentation with new UX mockups and user flow diagrams to reflect the changes.  Ensure that these documents are clear and provide a comprehensive overview of the user interactions with the system.
- Use mermaid diagrams and/or ascii art in markdown files to document user flows and UX mockups.  Ensure that all user interactions are documented with clear explanations of the expected behavior and outcomes.
- **`docs/ux/UX_LIFECYCLE.md` is the single traceability hub for all UI affordances.**  Every affordance carries an `PD-XX-AF-NNN` ID that must appear in its spec section, source docstring, and test docstring.  When making any change to `src/agentx/gui/`, the agent must:
  1. Open `docs/ux/00_INDEX.md` first — check the Status Snapshot and Priority Work Queue.
  2. Look up the Affordance ID in the traceability matrix (`UX_LIFECYCLE.md §4`).
  3. Update the matrix `Status` column as part of the same commit.
  4. Follow the checklist in §5 of that document (add / modify / remove).
  This is how drift between spec, code, and tests is prevented.

#### UAT Claim Policy (Required)
- The agent **must not** claim a UX defect is "definitively fixed" or "resolved" before user UAT confirms behavior in a live environment.
- Agent-owned claims are limited to: code changes applied, tests passing, and "ready for UAT".
- User-owned claims are required for closure language: "confirmed fixed", "resolved in UAT", or equivalent.
- In `docs/ux/UX_ISSUES.md`, prefer wording such as "attempted fix" or "latest fix candidate" until user confirmation is recorded.
- If UAT fails, the issue remains open and the agent must update the issue history with the failed attempt count and next hypothesis.

##### Ux Detail and Simplifcation Guidelines
- When documenting UX mockups and user flows, aim for a balance between detail and simplicity.
  - Include enough detail to accurately convey the user interactions and expected outcomes, but avoid unnecessary complexity that may obscure the main points.
  - Use clear and concise language in the documentation to ensure it is easily understandable by all stakeholders.
  - When details are generalized, provide a unique reference ID to am additional Detail diagram that provides the specific details for that interaction.  This allows for a clear separation between the high-level user flow and the detailed interactions, while still providing access to the necessary information when needed.
- UX is a form of Behavioral Documentation.  As such, it should be treated with the same level of importance as code documentation and architectural documentation.
- Ensure that UX documentation is kept up-to-date and accurately reflects the current state of the system.  Each architectural detail should have a corresponding behavioral use-case  documented in the UX documentation, and each use-case should have a corresponding architectural detail documented in the architectural documentation.  This ensures a comprehensive and cohesive documentation set that covers both the structural and behavioral aspects of the system.
- UX affordances should be clearly documented as units and should be tested with the same level of rigor as other units in the system, including unit tests, functional tests, and integration tests as appropriate.

#### Document Tool Schemas and Usage Examples

#### Document class relationships and module maps
- Use Mermaid diagrams and markdown tables to document class relationships and module maps.  Ensure that all modules and classes are documented with their purpose, key methods, and interactions with other modules/classes.

### Document Curation Policy

A *document* is a curated object — it may be a single file, a folder of related files, or a section within a knowledge base.  Curation is an active discipline: it includes creating, updating, and **removing** unwanted, inferior, conflicting, or false information.  The following rules apply to all documentation in this project.

#### One Authoritative Home Per Topic
- Each design topic, specification, or decision must have exactly one authoritative home.  Two documents that make overlapping or conflicting claims about the same topic create a "split-brain" condition that degrades the reliability of both.
- When you discover conflicting documents, identify which is authoritative (usually the more recent, more structured, or index-registered one), migrate any still-applicable unique content from the other into the authoritative document, then remove the subordinate document.
- If it is unclear which document is authoritative, or whether content is still applicable, **stop and ask the user before proceeding**.

#### Revision Stamps
- Every documentation file should carry a revision stamp on the line immediately below its title:
  ```
  _Last updated: YYYY-MM-DD (vX.Y.Z)_
  ```
- Update the stamp whenever the file is modified.  This allows the agent to identify the more current source when two documents conflict, and gives readers an at-a-glance freshness signal without opening git log.

#### Indexed Folders
- A folder that contains an `00_INDEX.md` is an *indexed* folder.  Every document in an indexed folder must be registered in that index.  An unregistered file in an indexed folder is orphaned and must either be added to the index or removed.
- Folders without an `00_INDEX.md` are unmanaged; no index coverage is required.  Do **not** auto-create an index for an unmanaged folder unless explicitly directed by the user.

#### Deletion Protocol
- Before deleting any document, ask: **"will we need this?"**  If there is any doubt, stop and ask the user for direction.
- Deletion is low-risk because every committed document is fully recoverable: `git checkout <commit> -- path/to/file` restores it to the working tree at any time.  This is only true if the document was committed before deletion — ensure it is.
- When directed to delete, remove the file, deregister it from any index, and update any cross-references.

#### Folder Changelogs
- If a documentation folder contains a `CHANGELOG.md`, record curation operations in it (migrations, deletions, renames) with enough detail — including the relevant commit hash or tag — that the agent or user can recover a prior version from git if needed.
- If no `CHANGELOG.md` exists in the folder, no curation tracking is required beyond the git commit message.


## Commands

```bash
# Install (Python 3.12 only — enforced by pyproject.toml)
uv sync

# Run the app
python main.py
# or
python -m agentx

# Run health checks (verifies Ollama/Agentix reachability)
python agentx_diagnostics.py

# Tests
python -m pytest                              # all tests
python -m pytest tests/test_active_model.py  # single file
python -m pytest tests/test_active_model.py::TestActiveModelProperty::test_active_model_initialized_from_config -v  # single test
python -m pytest -m "not live"               # skip integration tests that need external services

# Lint / format
black src/ tests/ --line-length=120
isort src/ tests/ --profile=black --line-length=120
flake8 src/ tests/                            # config in .flake8
mypy src/
```

## Architecture

### Startup flow

```
main.py → src/agentx/main.py
  → AgentXSession.__init__()
      ├── ServiceManager   (Ollama + Agentix health checks / subprocess launch)
      ├── GUIManager       (Tkinter widgets, implements IGUIManager protocol)
      ├── FileExplorer     (file navigation panel)
      ├── AgentixBridgeAdapter  (wraps async Agentix with sync/generator API)
      └── ClientToolExecutor / ServerToolExecutor
  → session.perform_service_handshake()
  → session.layout()          (builds all Tkinter widgets)
  → root.mainloop()
```

### Key module map

| Module | Role |
|--------|------|
| `src/agentx/session.py` | Central orchestrator — wires everything together, drives streaming |
| `src/agentx/service_manager.py` | Manages external service lifecycle (Ollama, Agentix) |
| `src/agentx/gui/gui_manager.py` | Implements `IGUIManager`; all Tkinter widget logic lives here |
| `src/agentx/igui_manager.py` | `Protocol` defining the GUI boundary — business logic talks only to this |
| `src/agentx/file_explorer.py` | File navigation widget (list, open, history traversal) |
| `src/agentx/history.py` | Loads prior sessions from disk |
| `src/agentx/widget_registry.py` | Centralised widget lifecycle and cleanup |
| `src/agentx/integration/agentix_bridge_adapter.py` | Converts async Agentix calls → sync / streaming generators for Tkinter thread model |
| `src/agentx/integration/streaming_executor.py` | Background-thread streaming with progress tracking |
| `src/agentx/integration/code_analysis.py` | CST/AST-based code analysis tools |
| `src/shared/models/context.py` | Conversation history — single source of truth, synced to disk |
| `src/shared/models/message.py` | `Message` dataclass with `MessageRole` enum |
| `src/shared/models/response.py` | `ResponseChunk` enum (CONTENT, THINKING, TOOL_CALL, TOOL_RESULT, …) |
| `src/shared/models/tools.py` | Tool definitions and schemas |
| `src/agentix/bridge/bridge.py` | `AgentixBridge` — tool loop, streaming, tool execution |
| `src/agentix/tools/schema.py` | `extract_tool_schema(fn)` — Python function → OpenAI JSON schema |
| `src/agentx/integration/client_tool_executor.py` | File system tools (read/write/search) wired into bridge |
| `system_prompts/` | Markdown prompt files loaded at runtime (planner, python_coder, tool_use, classification) |

### Tool pipeline

The agentic tool loop lives in `src/agentix/bridge/bridge.py`:

```
AgentixBridge.process_prompt_streaming()
  ├── RESPOND_DIRECTLY → _stream_direct_response()
  ├── SINGLE_TOOL      → _stream_tool_response()   ──┐
  └── INVOKE_PLANNER   → _stream_planned_response() ──┤
                                                       │
          _run_tool_loop(max_rounds=N) ────────────────┘
            ├── _iter_llm_chunks()   — accumulates OpenAI streaming deltas
            ├── execute_tool()       — dispatches by name (CST, AST, file tools)
            └── ThreadPoolExecutor  — runs multiple tool calls in parallel
```

- **Tool wire format**: `TOOL_CALL` chunks → Ollama `assistant` + `tool_calls[]`; `TOOL_RESULT` chunks → `tool` role with `tool_call_id`
- **Registering new tools**: call `bridge.register_tool_implementations(impls, schemas)` — `AgentixBridgeAdapter._register_client_tools()` is the reference implementation
- **Schema generation**: `extract_tool_schema(fn)` in `src/agentix/tools/schema.py` converts any Python function with a docstring into a valid OpenAI schema
- **Context persistence**: `session._display_tool_call()` / `_display_tool_result()` store tool interactions in `Context` using `add_tool_call_message()` / `add_tool_result_message()` — do not call `handle_tool_call()` directly (double-execution bug, now fixed)

### Threading model

Tkinter must run on the main thread. LLM streaming runs on background threads. `AgentixBridgeAdapter` wraps async Agentix methods with sync calls and returns generators so `streaming_executor.py` can iterate them safely. Use `threading.Event` for coordination (see `_is_streaming` in `session.py`).

### Persistence

Conversations are stored under `sessions/<session_id>/context/` as JSON. `Context` (in `shared/models/context.py`) is the authoritative in-memory model and handles load/save. Do not write session state anywhere except through `Context`.

## Conventions

### Style
- **Line length**: 120 characters (black + isort + flake8 all agree)
- **Type hints**: required everywhere; use Python 3.12+ syntax (`list[str]`, `dict[str, Any]`)
- **Dataclasses** for data models; **Enums** for typed constants; **Protocols** for interfaces

### Naming
- Classes: `PascalCase`; interface classes prefixed with `I` (e.g. `IGUIManager`)
- Methods/attributes: `snake_case`; private members: single leading underscore (`_active_model`)
- Constants: `UPPER_SNAKE_CASE`

### Patterns
- **Adapter** — `AgentixBridgeAdapter` is the canonical example; use adapters when bridging async↔sync or external API boundaries
- **Protocol** — define boundaries with `Protocol` classes before implementing; `session.py` depends on `IGUIManager`, never on `GUIManager` directly
- **Registry** — `WidgetRegistry` owns widget lifecycle; don't create/destroy widgets outside it
- **Markers** — tag tests that need live external services with `@pytest.mark.live`

### Configuration
Runtime config lives in `agentx.toml` (loaded by `src/agentx/config.py`). Never hard-code hostnames, model names, or timeouts — read from `AgentXConfig`. The GUI config subset lives in `gui/gui_config.py`.

## Plan Documents

Multi-session work is tracked in Markdown plan files (e.g. `docs/tool_usage_plan.md`). Each
step is a numbered checkbox. When working on a plan:

- **At session start:** read the plan file to find the next `[ ]` step
- **While working:** mark the step `[/]` (in progress) before starting it
- **On success:** replace `[/]` with `[ ]` and add a ✓ comment inline, **or** leave `[/]`
  permanently to indicate "done" — follow whichever convention the plan file uses
- **On failure:** mark the step `[X]` and append a brief inline note explaining why, so it
  can be revisited or corrected in a future session; report the failure explicitly in your
  response so the user is aware

Step status legend (applies to all plan files in this repo):

| Marker | Meaning |
|--------|---------|
| `[ ]` | Not yet started |
| `[/]` | Complete |
| `[X]` | Failed or blocked — needs follow-up |

Never silently skip a failed step. Always surface `[X]` items before closing out a session.

## Reflect on your work and update the Project Version using Semantic Versioning (SemVer)

- the current project version is tracked in `pyproject.toml` under the `[project]` section (`version` key). Update the version there when making changes that should be reflected in the project version:
  - changes that do not introduce backward-incompatible changes should increment the patch version (third number)
  - changes that introduce new features in a backward-compatible manner should increment the minor version (second number).  When the minor version is incremented, the patch version must be reset to zero.
  - changes that introduce backward-incompatible changes should increment the major version (first number) if the major version is not zero; if the major version is zero, increment the minor version instead. When the major version is incremented, the minor and patch versions must be reset to zero.


## Additional Resources

- `docs/architecture.md` — module index with retrieval keywords, designed for AI-assisted reasoning
- `docs/tool_usage_plan.md` — phased implementation plan for the agentix tool usage path
- `smolagentx.md` — SmolAgents integration PoC design and suggested agent prompts
- `agentx_diagnostics.py` — run this first when external service behaviour is unexpected
