# AgentX — Component Cut-Sheet Template

_Last updated: 2026-05-06 (v0.22.20.post3)_

Use this template for every UI component or sub-component that has user-visible
behaviour. This includes panels, reusable widgets, dialogs, and nested controls.

---

## Component Identity

- Component ID: `PD-XX` (or `PD-XX-AF-NNN` for a sub-component record)
- Component name: `<Name>`
- Source class/module: `<module path + class>`
- Parent component(s): `<where this lives>`
- Child components: `<named sub-components>`
- Owner tab/panel: `<Chat / Session / Files / Settings / Dialog>`

---

## Placement Diagram (Context)

Show where this component lives relative to siblings and parent components.
Use ASCII or Mermaid.

```text
MainWindow
  ├── ParentPanel
  │    ├── ThisComponent
  │    └── SiblingComponent
  └── OtherPanel
```

---

## Internal Structure Diagram (Labeled Sub-Components)

Label all named sub-components that have independent behaviour.

```text
ThisComponent
  ├── toggle_button
  ├── title_label
  └── content_container
```

Each labeled sub-component must either:

- have its own cut-sheet section, or
- be explicitly declared as passive/non-interactive.

---

## Behaviour Inventory

| Affordance ID | Sub-component | Trigger | Expected behaviour | Edge cases |
|---------------|---------------|---------|--------------------|------------|
| PD-XX-AF-001 | toggle_button | click | collapse/expand content | double-click, no-content |

---

## Gherkin Use-Cases (Complete)

Write one scenario per behaviour and state transition.

### Scenario: `<name>` `[PD-XX-AF-NNN]`

GIVEN `<initial state>`
WHEN `<user action>`
THEN `<observable result>`

Repeat until all behaviours in the inventory are covered.

---

## Test Mapping

| Affordance ID | Test file | Test class | Test function | Status |
|---------------|-----------|------------|---------------|--------|
| PD-XX-AF-001 | tests/test_<component>.py | Test<Component> | test_<behavior> | Planned / Passing |

---

## Code and Configuration References

- Source implementation:
  - `<module path>:<class/method>`
- Configuration keys consumed:
  - `<agentx.toml key>`
- Runtime lookups / external dependencies:
  - `<service call / lookup>`
- Data/state dependencies:
  - `<state fields and owning class>`

---

## Definition of Done

- [ ] All affordances have IDs (`PD-XX-AF-NNN`).
- [ ] Placement and internal diagrams are current.
- [ ] Behaviour inventory covers all user-visible states.
- [ ] Gherkin scenarios exist for all behaviours.
- [ ] Test mapping includes concrete file/class/function references.
- [ ] `docs/ux/UX_LIFECYCLE.md` matrix row status updated.
- [ ] Tests pass with no regressions.
