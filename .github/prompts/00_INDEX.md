# Issue Workflow Prompts — Index

_Last updated: 2025-05-13 (v0.48.3)_

This folder contains Copilot agent prompts for the full issue lifecycle in the
AgentX project. Each prompt is narrow in scope and hands off to the next.

## Lifecycle Pipeline

```
issue-intake
    │  first contact, duplicate check, triage, labels
    ▼
issue-reproduce-evidence
    │  structured reproduction, artifacts, reproducibility matrix
    ▼
issue-investigate
    │  root-cause analysis, blast radius, fix strategy
    ▼
issue-regression-tests
    │  Gherkin scenarios, hermetic unit + integration tests (failing baseline)
    ▼
issue-fix-close
    │  fix code, quality gates, changelog, version, close issue
    ▼
issue-pr-handoff
    │  push branch, create PR, apply checklist, wait for review
    ▼
issue-verify-release
       post-merge verification, durable audit trail, reopen criteria
```

## Prompt Reference

| File | Purpose |
|------|---------|
| [issue-intake.prompt.md](issue-intake.prompt.md) | First contact, duplicate detection, triage metadata |
| [issue-reproduce-evidence.prompt.md](issue-reproduce-evidence.prompt.md) | Reproduction steps, artifacts, reproducibility matrix |
| [issue-investigate.prompt.md](issue-investigate.prompt.md) | Root-cause analysis, blast radius, fix strategy |
| [issue-regression-tests.prompt.md](issue-regression-tests.prompt.md) | Gherkin scenarios, failing regression tests |
| [issue-fix-close.prompt.md](issue-fix-close.prompt.md) | Fix code, quality gates, CHANGELOG, close issue |
| [issue-pr-handoff.prompt.md](issue-pr-handoff.prompt.md) | Create PR, apply checklist, link issue, hand off |
| [issue-verify-release.prompt.md](issue-verify-release.prompt.md) | Post-release verification, reopen criteria |
| [ux-review.prompt.md](ux-review.prompt.md) | UX affordance review (separate workflow) |

## Conventions

- **UAT Claim Policy**: the agent may only claim "ready for UAT". User confirmation
  is required before any issue is marked "resolved".
- **No skipping steps**: each prompt enforces pre-conditions from the prior step.
- **One issue per run**: these prompts operate on a single issue at a time.
