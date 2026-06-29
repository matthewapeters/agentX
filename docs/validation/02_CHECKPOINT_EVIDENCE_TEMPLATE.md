# Checkpoint Evidence Template (M0-M4)

Last updated: 2026-06-23
Status: Active reusable template

Use this template for every milestone checkpoint package in M0-M4.

## 1) Header

- Milestone: M0 | M1 | M2 | M3a | M3b | M4
- Checkpoint date: YYYY-MM-DD
- Checkpoint ID: <team-defined-id>
- Checkpoint decision: Proceed | Proceed with risk acceptance | Hold and remediate

## 2) Ownership (Fail-Close)

- Milestone owner (required): Delivery Lead
- Architecture gate owner (required): Architecture Reviewer
- UX gate owner (required): UX Reviewer
- QA gate owner (required): QA Lead
- Security gate owner (required for M1, M3b, M4): Security Reviewer

Owner assignment status:

- Assigned at kickoff: Yes | No
- If No, checkpoint is fail-close and cannot pass.

## 3) Scope And Source-Doc Mapping

Scope slice summary:

- <short scope statement>

Source-doc mapping:

| Scope item ID | Source contract/doc | Section/anchor | Notes |
| --- | --- | --- | --- |
|  |  |  |  |

## 4) Entrypoint Verification (Mandatory For M1-M4)

Confirm the active Go entrypoint is present before milestone kickoff:

- `test -d cmd/agentx`

Execution record (required):

- operator: <name/handle>
- timestamp_utc: <YYYY-MM-DDTHH:MM:SSZ>
- exit_code: <0|non-zero>

Normalized proof table (required for each gate command):

| command | exit_code | timestamp_utc | operator | artifact_link |
| --- | --- | --- | --- | --- |
|  |  |  |  |  |

Notes:

- `artifact_link` may be `none` if command has no stdout/stderr artifact.
- Non-zero `exit_code` requires Hold and remediate, or an explicit approved
   defer entry in Section 9.

Compatibility implications:

- <compatibility impact summary>

## 5) AC Coverage Table (Mandatory)

Test case ID convention: `TC-<milestone>-<area>-<nnn>` (example:
`TC-M3b-policy-003`).

| AC ID | Acceptance criterion (measurable) | Test case ID(s) | Test type | Evidence link/artifact | Result |
| --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |

Fail-close rule:

- Any unmapped AC requires explicit defer metadata in Section 9.

## 6) Regression Evidence (Mandatory)

Protected scenarios:

- <scenario 1>
- <scenario 2>

Rerun minimum required:

- <count and rationale>

Regression run evidence:

| Scenario/Test ID | Run count | Pass/Fail summary | Evidence link/artifact |
| --- | --- | --- | --- |
|  |  |  |  |

## 7) Negative-Path Matrix (Conditional)

Required for M3b and M4. Optional for M0/M1/M2/M3a unless risk requires.

| Scenario ID | Negative condition | Expected bounded behavior | Test case ID(s) | Evidence | Result |
| --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |

## 8) Documentation-First GIVEN/WHEN/THEN Links (Mandatory)

| Touched function/flow | GIVEN/WHEN/THEN doc link | Source contract link | Test scenario link |
| --- | --- | --- | --- |
|  |  |  |  |

## 9) Unresolved Decisions And Deferments (Mandatory If Any)

Required defer metadata fields: owner role, due milestone/date, tracking
reference.

| Decision/AC ID | Reason unresolved/deferred | Owner role | Due milestone/date | Tracking reference |
| --- | --- | --- | --- | --- |
|  |  |  |  |  |

References:

- Open questions log: `docs/implementation/90_open_questions.md`

## 10) Runtime Compatibility Delta (Mandatory)

- Cross-runtime or schema changes in this checkpoint: Yes | No
- If Yes, summarize compatibility impact:
- Backward compatibility status: Compatible | Requires migration | Not compatible
- Validation evidence link/artifact:

## 10a) Evidence Artifact Convention (Canonical)

- Base path: `docs/validation/evidence/<checkpoint_id>/`
- File naming: `<checkpoint_id>_<artifact_type>_<YYYYMMDD-HHMMSS>.<ext>`

## 11) Sign-Off Block

- Delivery Lead sign-off: Name/handle, date, status
- Architecture Reviewer sign-off: Name/handle, date, status
- UX Reviewer sign-off: Name/handle, date, status
- QA Lead sign-off: Name/handle, date, status
- Security Reviewer sign-off (M1/M3b/M4 required): Name/handle, date, status

Final disposition:

- Approved for milestone exit: Yes | No
- If No, required remediation actions:
