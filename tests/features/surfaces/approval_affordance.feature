# Source contracts:
#   - docs/implementation/05_security_approvals_and_command_policy.md (approval round-trip)
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-3)
#
# Behavior: while the runtime awaits a tool-approval decision the chat surface
# advertises the options and maps a/g/d to approve(session)/approve(global)/deny.

@functional @arch:chat-surface @ux:PD-01
Feature: Tool-approval affordance
  As a user of the chat surface
  I want clear approve/deny keys while a tool awaits my decision
  So that I consent to (or block) risky commands without guessing

  # use-case: UC-CHAT-APPROVAL
  Scenario: The hint advertises approval options while awaiting input
    Given a new chat surface sized 40 by 10
    When the processing state becomes awaiting input
    Then the chat hint shows "a session"

  # use-case: UC-CHAT-APPROVAL
  # variant: approve-session
  Scenario: Pressing a approves for the session
    Given a chat surface that records approvals sized 40 by 10
    When the processing state becomes awaiting input
    And the "a" key is pressed
    Then the approval decision is "session"

  # use-case: UC-CHAT-APPROVAL
  # variant: approve-global
  Scenario: Pressing g approves globally
    Given a chat surface that records approvals sized 40 by 10
    When the processing state becomes awaiting input
    And the "g" key is pressed
    Then the approval decision is "global"

  # use-case: UC-CHAT-APPROVAL
  # variant: deny
  Scenario: Pressing d denies
    Given a chat surface that records approvals sized 40 by 10
    When the processing state becomes awaiting input
    And the "d" key is pressed
    Then the approval decision is "deny"
