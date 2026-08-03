# Source contracts:
#   - docs/implementation/05_security_approvals_and_command_policy.md (approval round-trip)
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-3)
#
# Behavior: a tool call that needs approval pauses the cycle in awaiting_input
# until the surface resolves it. Approving allows and persists the scope; denying
# blocks; interrupting while awaiting fails cleanly.

@functional @arch:tool-approval
Feature: Tool approval round-trip
  As the AgentX tool runtime
  I want risky commands gated by an interactive approval
  So that nothing mutating or networked runs without explicit consent

  # use-case: UC-TOOL-APPROVAL
  Scenario: Approving for the session allows and persists
    Given a started orchestrator with a tool policy
    When approval is requested for "write_file" with:
      | arg     | value       |
      | path    | ./notes.txt |
      | content | hello       |
    And the user approves for the "session"
    Then the request is allowed
    And "write_file" with the same arguments is now allowed by policy

  # use-case: UC-TOOL-APPROVAL
  # variant: global
  Scenario: Approving globally allows and persists
    Given a started orchestrator with a tool policy
    When approval is requested for "write_file" with:
      | arg     | value       |
      | path    | ./notes.txt |
      | content | hello       |
    And the user approves for the "global"
    Then the request is allowed
    And "write_file" with the same arguments is now allowed by policy

  # use-case: UC-TOOL-APPROVAL
  # variant: plan-scoped
  Scenario: Approving for the plan allows and scopes to that plan only
    Given a started orchestrator with a tool policy
    When approval is requested within plan "plan-1" for "write_file" with:
      | arg     | value       |
      | path    | ./notes.txt |
      | content | hello       |
    And the user approves for the plan
    Then the request is allowed
    And "write_file" with the same arguments is now plan-approved for "plan-1"
    And "write_file" with the same arguments still needs approval

  # use-case: UC-TOOL-APPROVAL
  # variant: deny
  Scenario: Denying blocks and does not persist
    Given a started orchestrator with a tool policy
    When approval is requested for "write_file" with:
      | arg     | value       |
      | path    | ./notes.txt |
      | content | hello       |
    And the user denies the request
    Then the request is denied
    And "write_file" with the same arguments still needs approval

  # use-case: UC-TOOL-APPROVAL
  # variant: interrupt
  Scenario: Interrupting while awaiting fails cleanly
    Given a started orchestrator with a tool policy
    When approval is requested for "write_file" with:
      | arg     | value       |
      | path    | ./notes.txt |
      | content | hello       |
    And the awaiting request is interrupted
    Then the request fails
