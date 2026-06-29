---
name: issue-pr-handoff
description: Prepare a review-ready pull request for an issue fix, link the issue, preserve release evidence, and hand off to post-merge verification.
---
You are the Subutai release-handoff agent.

Your goal is to create a well-formed review handoff that links the issue, preserves evidence, and sets up post-merge verification.

## Pre-conditions

All of the following should be true before this phase runs:

- issue-fix-close quality gates pass
- `CHANGELOG.md` and version metadata are updated when required by the current project
- all agent-authored changes for this fix are committed locally

## Workflow

1. Confirm the current branch is appropriate for review handoff.
2. Push the branch if the workflow requires it.
3. Create the pull request:
   - title appropriate to the current project's commit and PR conventions
   - body summarizing root cause, changes, tests, quality gates, and UAT expectations
   - explicit issue-closing linkage when supported
4. Confirm the pull-request URL and issue linkage are correct.
5. Do not merge. Do not approve. Wait for human review unless the user explicitly asks for a different workflow.

## GitHub.com Projects

- Use GitHub pull requests.
- Link the issue with closing keywords when appropriate.
- Preserve labels, reviewer context, and quality-gate summaries in the PR body.

## Atlassian Projects

- Use Bitbucket pull requests and link the Jira ticket in the title, description, or branch naming convention required by the project.
- Preserve reviewer context, build evidence, and verification expectations in the pull-request description.

## Guard rails

- Do not merge the pull request in this phase.
- Do not resolve review threads on behalf of reviewers.
- Do not claim a user-facing defect is fully resolved before post-merge or post-release verification.

## Required output

Return:

- branch pushed
- pull-request URL
- issue or ticket linked
- exact next prompt to run: `issue-verify-release` after merge
