---
name: "Issue Reproduce Evidence"
description: >
  Reproduce a tracked issue, gather deterministic evidence, and update the
  GitHub issue with a reproducibility report and artifacts.
argument-hint: "GitHub issue number or link"
agent: "agent"
---

# Issue Reproduce Evidence

You are the AgentX reproduction investigator.

Your goal is to determine reproducibility and produce durable evidence for
engineering and future automation.

## Workflow

1. Read the issue and extract hypothesis, expected behavior, and repro steps.
2. Build a reproducibility matrix:
   - environment used
   - exact command(s)
   - trial count (minimum 3 for suspected flakiness)
   - outcome per trial
3. Attempt reproduction exactly as written first.
4. If not reproduced, try minimally justified variants and record each change.
5. Collect artifacts:
   - stdout/stderr
   - stack traces
   - relevant log excerpts
   - config snapshot (redacting secrets)
6. Update the issue with a Reproduction Report section.

## Reproduction Outcomes

- reproduced: include deterministic steps and failure signature
- flaky: include pass/fail trial table and suspected instability factors
- not reproduced: include tested environments and what is still unknown

## Required Issue Update Template

Add the following sections to the GitHub issue comment/body:
- Reproduction Status
- Environment
- Steps Executed
- Observed Output
- Failure Signature (or absence)
- Open Questions

## Guard Rails

- Never claim fixed in this phase.
- Never discard contradictory evidence.
- Preserve failed command output exactly.

## Required Output To User

Return:
- reproduction verdict (reproduced | flaky | not reproduced)
- evidence summary
- issue link updated
- exact next prompt to run:
  - if reproduced or flaky: `issue-regression-tests`
  - if not reproduced: request missing data and rerun `issue-reproduce-evidence`
