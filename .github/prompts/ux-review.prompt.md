---
name: "UX Review & Enforce"
description: >
  Review a UX panel or affordance change end-to-end: verify spec docs reflect
  requirements, update Gherkin use-cases, update ASCII diagrams, run UX tests,
  iteratively fix code until all tests pass with no regressions.
  Use when: ux review, ux change, affordance update, panel spec, gherkin update,
  ux test, ux regression, ux tdd, ux enforce
argument-hint: "PD number or affordance ID to review (e.g. PD-03 or PD-03-AF-002)"
agent: "agent"
---

# UX Review & Enforce

You are running a structured UX review cycle for AgentX.
The entry point for all UX work is [docs/ux/UX_LIFECYCLE.md](../docs/ux/UX_LIFECYCLE.md).

The lifecycle follows four phases: **Specify → Code → Test → Reconcile**.
This prompt drives the full cycle for the affordance or panel provided as the argument.
If no argument was given, default to a full-panel audit of every `📝` or `❌`
entry in the Traceability Matrix (§4 of UX_LIFECYCLE.md).

---

## Phase 0 — Establish Baseline

Before making any change, record what is currently passing so regressions are
detectable.

1. Run all UX-related tests and capture the count of passing/failing:
   ```
   python -m pytest tests/ -m "not live" -q 2>&1 | tail -5
   ```
2. Record the baseline pass/fail count. **Do not proceed if there are
   pre-existing failures** — surface them to the user first and ask whether
   to fix them or skip them explicitly.

---

## Phase 1 — Review Requirements (Specify)

3. Read [docs/ux/UX_LIFECYCLE.md](../docs/ux/UX_LIFECYCLE.md) — locate the
   target affordance row in the Traceability Matrix (§4).
4. Read the relevant section of
   [docs/ux/03_PANEL_DETAILS.md](../docs/ux/03_PANEL_DETAILS.md) for the
   panel being changed.
5. Check whether the current spec accurately describes what the code actually
   does:
   - If the spec is ahead of the code → code needs to be implemented (Phase 2).
   - If the code is ahead of the spec → spec needs to be updated first.
   - If they agree → move to Phase 2.
6. **Freeze the spec before writing any code.** If spec changes are needed,
   commit them separately with the message prefix `spec:`.

---

## Phase 2 — Verify Cut-Sheets (Component Spec)

For the affordance under review, verify the component cut-sheet in
`03_PANEL_DETAILS.md` is complete and accurate:

7. Confirm the **Affordance table row** exists with the correct `PD-XX-AF-NNN`
   ID.
8. Confirm the **State Fields table** lists all `tk` variables that drive this
   affordance.
9. Confirm the **ASCII or Mermaid diagram** reflects the current widget
   hierarchy. Update it if any widget was added, removed, or repositioned.
   - Use the diagram style already present in that panel's section.
   - If no diagram exists and the widget hierarchy is non-trivial, add one.

---

## Phase 3 — Verify Gherkin Use-Cases

10. Find the test class(es) for this affordance (Traceability Matrix, Test Class
    column).
11. For each `def test_*` method in that class, read the docstring and verify it:
    - Has a `GIVEN … WHEN … THEN …` structure.
    - References the Affordance ID: `[PD-XX-AF-NNN]`.
    - Accurately describes the post-change expected behaviour (not the old
      behaviour).
12. If any Gherkin use-case is stale or missing:
    - Update it in the test docstring **before** changing any test assertion.
    - This is the behavioural oracle. Code must satisfy what the Gherkin says.

---

## Phase 4 — Update Tests (Test-First TDD)

13. For each updated Gherkin use-case, review the corresponding test assertions:
    - Do the assertions actually verify the THEN clause?
    - Are edge cases covered (empty content, disabled state, error state)?
14. Update or add test assertions to match the new Gherkin.
15. Run only the affected test class to see the failures in isolation:
    ```
    python -m pytest tests/<test_file>.py::<TestClass> -v
    ```
16. Record which tests fail (these are the implementation targets for Phase 5).
    **Do not skip or comment out failing tests.**

---

## Phase 5 — Iterative Code Fix

17. For each failing test identified in Phase 4:
    a. Read the relevant source file (Traceability Matrix, Source Class/Method
       column).
    b. Identify the minimal change that satisfies the THEN clause.
    c. Apply the change.
    d. Re-run the single failing test to confirm it now passes.
    e. Run the full UX test suite to confirm no regressions:
       ```
       python -m pytest tests/ -m "not live" -q
       ```
18. Repeat step 17 for each remaining failing test until all pass.
19. If a code change requires a spec change (unexpected behaviour discovered),
    pause, update the spec, and update the Gherkin before continuing.

---

## Phase 6 — Reconcile (As-Built Update)

20. Update the Traceability Matrix row(s) in
    [docs/ux/UX_LIFECYCLE.md](../docs/ux/UX_LIFECYCLE.md):
    - Change status from `📝` / `⚠️` to `✅` for affordances now fully tested.
    - Update Test File / Test Class columns if they changed.
21. If affordances were added, add new rows with the next sequential
    `PD-XX-AF-NNN` ID.
22. If affordances were removed, delete the rows and search for the ID:
    ```
    grep -rn "PD-XX-AF-NNN" src/ tests/ docs/
    ```

---

## Phase 7 — Final Gate

23. Run the full test suite with coverage:
    ```
    python -m pytest tests/ -m "not live" --tb=short -q
    ```
24. **Pass criteria:**
    - All tests pass (0 failures).
    - No regressions vs the Phase 0 baseline.
    - Coverage remains ≥ 98%.
25. Apply black + isort to any changed source files:
    ```
    black src/ tests/ --line-length=120
    isort src/ tests/ --profile=black --line-length=120
    ```

---

## Phase 8 — Commit

26. Stage all changed files:
    ```
    git add src/ tests/ docs/ux/
    ```
27. Write the commit message following this structure:
    ```
    ux(<PD-XX>): <one-line summary of what changed>

    Affordances changed:
    - PD-XX-AF-NNN: <description> [<old status> → ✅]

    Spec: updated 03_PANEL_DETAILS.md §<section>
    Tests: updated/added <N> Gherkin use-cases in <test file>
    Code: changed <source file(s)>
    ```
28. Commit:
    ```
    git commit
    ```
29. Bump the project version in `pyproject.toml`:
    - Code fix → patch bump (`0.20.2` → `0.20.3`)
    - New affordance → minor bump (`0.20.2` → `0.21.0`)
    - Spec/doc only → `.post` suffix (`0.20.2` → `0.20.2.post1`)
30. Update `CHANGELOG.md` with the changes under the new version.
31. Commit the version bump:
    ```
    git add pyproject.toml CHANGELOG.md && git commit -m "chore: bump version to <X.Y.Z>"
    ```

---

## Guard Rails

- **Never change code before the spec and Gherkin are frozen** (Phase 1 and 3
  must be complete first).
- **Never skip or comment out a failing test** — either fix the code or
  escalate to the user.
- **Never mark a status ✅ in the matrix** unless there is at least one test
  per Gherkin clause that passes.
- If a test was passing before and now fails, treat it as a blocking regression
  — do not proceed to commit.
