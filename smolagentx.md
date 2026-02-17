# SmolAgentX — HF smolagents CodeAgent Integration Notes (revised)

Date: 2026-02-17 (rev)
Author: GitHub Copilot Chat Assistant (for maintainers' review)

Purpose
-------
Consolidate the SmolAgents PoC plan using the repository architecture. Make the document assistant-friendly: add concise module mappings, indexable prompts, and clear, implementable PoC steps so agents and maintainers can iterate quickly.

Quick repo context (from docs/architecture.md)
----------------------------------------------
- Branch scanned: smolagents
- High-value modules:
  - Entrypoints: src/agentx/main.py, src/agentx/__main__.py, agentx_diagnostics.py
  - Session & orchestration: src/agentx/session.py
  - External service management: src/agentx/service_manager.py
  - File tools & GUI: src/agentx/file_explorer.py, src/agentx/gui/*, src/agentx/widget_registry.py
  - Persistence/history: src/agentx/history.py
  - Integrations/adapters: src/agentx/integration/*
- For full file map, see docs/architecture.md

Top-level goals
---------------
1. Demonstrate value with low-risk automation (test generation, PR scaffolding).
2. Provide human-in-the-loop safety: sandboxed runs + draft PRs + audit logs.
3. Make repository assistant-friendly: indexable summaries, prompts, and a small session/router prototype.

Prioritized targets (mapped to repo)
-----------------------------------
1. Entry & diagnostics (low risk)
   - Files: src/agentx/main.py, agentx_diagnostics.py
   - Why: simple, deterministic actions (start, health checks); minimal side effects.
   - PoC idea: a tool that, when triggered, runs diagnostics and proposes a README update or issue if a dependency is missing.

2. Test scaffolding for a low-risk module (medium risk)
   - Files: pick a small module, e.g., src/agentx/file_explorer.py
   - Why: straightforward behavior to assert (list_directory/open_file/navigation).
   - PoC idea: TestGenAgent inspects function signatures/docstrings → generates unit tests → run tests in sandbox → open draft PR with passing tests.

3. PR automation with prompts + audit (medium risk)
   - Files: integration across repo for PR metadata and artifact storage (.smolagents/*, CI workflows)
   - Why: demonstrates lifecycle from suggestion → sandbox run → draft PR → human review.

Agent / Tools table (revised and mapped to modules)
---------------------------------------------------
| Agent / Tool | How it helps (mapped example) | Estimated LOE (PoC) |
|--------------|-------------------------------|---------------------|
| CodeAgent (core) | Produce minimal patches (e.g., implement small bugfix in file_explorer.open_file) and a short justification. | Low — days |
| TestGenAgent | Generate unit tests for file_explorer and session helpers; create test files and suggested assertions. | Low–Medium — 1–2 weeks |
| PRAgent | Create draft PRs with patch diff, test results, and prompt/audit artifacts; attach logs. | Medium — 1–2 weeks |
| RefactorAgent | Suggest safe refactors for GUI/widget cleanup or History loading; include regression tests. | Medium — 2–3 weeks |
| DocAgent | Update smolagentx.md, docs/architecture.md, and generate README snippets from docstrings. | Low — days |
| DependencyAgent | Scan pyproject.toml/uv.lock and propose non-breaking updates, with compatibility notes. | Medium — 1–2 weeks |
| SecurityAgent | Run static analysis and dependency-supply-chain checks, produce a remediation summary. | Medium — ~2 weeks |
| SandboxRunner / Orchestrator | Execute agents in an ephemeral environment, run tests, collect artifacts, ensure safety gating. | High — multi-week (infra & policies) |

Assistant-friendly index additions (to add to repo)
---------------------------------------------------
- .repo_index.json (keyword -> file path -> 1-line summary) for retrieval.
- .smolagents/prompts/* (prompt templates for TestGenAgent, PRAgent, etc.)
- .smolagents/audit/* (store prompt inputs, outputs, diffs, and test logs)

Suggested integration patterns (concrete)
----------------------------------------
- Human-in-the-loop PR generation:
  1. Agent writes patch + tests.
  2. SandboxRunner applies patch to ephemeral branch, runs tests/lint/type-check.
  3. If checks pass, PRAgent opens a draft PR with artifacts and the original prompt in .smolagents/audit/.
  4. Maintainer reviews and merges.

- Suggest-only mode:
  - Agent produces unified diff + reasoning in PR comment or .smolagents/suggestions/*.patch for maintainers to import manually.

- Autonomy-limited mode:
  - For trivial formatting/lint fixes, agent commits to a dedicated branch and can be auto-merged when CI & owner policy allow.

Prompt templates (examples)
---------------------------
- Test generation:
  "You are a code agent. Given the file src/agentx/file_explorer.py and its docstrings, produce pytest unit tests that cover edge cases for open_file and list_directory. Output only the test file content and a 1-line summary of what was tested."

- Fix-from-test:
  "You are a code agent. Given a failing test and the repository snapshot, produce a minimal unified diff to make the test pass. Include a 1-2 line justification."

- Router decision:
  "Given the user intent and these keywords, choose one tool to handle the turn from [CodeAgent, TestGenAgent, PRAgent, DocAgent] and justify the selection in 1 sentence."

Assistant-friendly outputs & structure (what to commit)
------------------------------------------------------
- Keep smolagentx.md structured with:
  - One-line module maps (path -> responsibility).
  - Table of agents (above).
  - Concrete PoC steps and acceptance criteria.
  - Prompt templates.
  - Index pointers to docs/architecture.md and candidate modules.

Minimal PoC plan (detailed steps)
--------------------------------
1. Pick target: src/agentx/file_explorer.py.
2. Add prompt template files: .smolagents/prompts/testgen_file_explorer.txt
3. Implement a tiny runner script scripts/agent_runner.py that:
   - Loads config, extracts target files, invokes TestGenAgent with prompt template, writes tests to tests/test_file_explorer_generated.py
   - Runs pytest in an isolated process
   - On success, prepares a patch and stores audit artifacts to .smolagents/audit/
4. Add GitHub Actions workflow .github/workflows/agent-poc.yml (manual dispatch) restricted to smolagents branch; when triggered, it runs scripts/agent_runner.py in an ephemeral job and uploads artifacts.
5. If artifacts pass, PRAgent can be invoked to open a draft PR (human review required before merge).

Acceptance criteria
-------------------
- Generated tests run and pass in the sandbox.
- Draft PR includes: patch, test results, prompt used, and audit logs.
- Maintainer can trivially reproduce the run locally using scripts/agent_runner.py.

Risks and mitigations (short)
----------------------------
- Hallucinated code: require passing tests + linters + human review for non-trivial changes.
- Secrets/exfiltration: run agents in isolated runner with no credentials and no network egress (or very restricted).
- License/IP risk: flag third-party code suggestions for manual review; add license step.

Suggested small commits to make now (recommendation)
---------------------------------------------------
1. docs/.repo_index.json (automatically generated map) — enables fast routing.
2. .smolagents/prompts/testgen_file_explorer.txt — prompt template for TestGenAgent.
3. scripts/agent_runner.py — minimal harness (no secrets) to run prompt -> write tests -> run pytest -> collect artifacts.
4. docs/architecture.md (already added) and updated smolagentx.md (this document).

Auditability & traceability
--------------------------
- For every agent action store:
  - prompt.txt
  - agent_response.json
  - diff.patch
  - test_results.json
  - runner.log
- Place these under .smolagents/audit/<timestamp>/

Next steps I can take (if you want me to proceed)
-------------------------------------------------
- Create the updated smolagentx.md (this draft) on the smolagents branch and commit it.
- Add prompt templates and a minimal scripts/agent_runner.py skeleton.
- Generate .repo_index.json (scanning the branch) and commit.