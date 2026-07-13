
## Changes are to be made in accordance to documents

Refer to [`CLAUDE.md`](CLAUDE.md) for the canonical project guide (architecture,
commands, key invariants) and [`00_START_HERE.md`](00_START_HERE.md) for the full
documentation reading path.

## Apply Changes to Code After Documents Are Updated
Follow this process when applying changes:
- Identify the correct Experts to apply changes.  When appropriate, encourage experts to collaborate.
- Apply changes
  - Changes are usually related to some change in behavior.  Capture this as a new or updated GHERKIN use-cases, and build hermetic unit tests around these.  Once hermetic unit tests are complete, tests may require Integration/Functional or End-To-End (E2E) tests.  Advise user when these are appropriate.  These must also be expressed and documented as GHERKIN use-cases.
    - Each hermetic unit test should include the GHERKIN use-case in its documentation.
    - Create tests to assert that the proposed changes are functional.
    - Strive for 98% or higher code coverage with hermetic unit tests.
- Apply all quality gates
  - No Syntax Errors
  - Apply Best Practices for Linting appropriate to the language
  - All New Tests Must Pass 
- Update the CHANGELOG.md with the modified changes, and update the semantic version appropriately.
- Commit changes made with a meaningful comment (Example, use the comments used to update CHANGELOG.md)
- Tag the commit with the semantic version whenever the Major or Minor versions are changed

