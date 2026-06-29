# Source contracts:
#   - docs/implementation/05_security_approvals_and_command_policy.md (output artifacts)
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-2)
#
# Behavior: full tool output is persisted to a session artifact and read back by
# ref with line-based windowing (backing the read_output tool).

@unit @arch:tool-artifacts
Feature: Session output artifacts
  As the AgentX tool runtime
  I want full tool output persisted and paged from the session
  So that large outputs need not be buffered into the model context

  # use-case: UC-TOOL-ARTIFACT
  Scenario: An artifact round-trips with line windowing
    Given an artifact store
    When an artifact of 50 numbered lines is written
    Then the artifact has 50 lines
    And reading the artifact at offset 0 limit 3 yields "line-001"
    And reading the artifact at offset 49 limit 1 yields "line-050"
