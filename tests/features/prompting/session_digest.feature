# Source contracts:
#   - docs/architecture/prompt_fan_groups.md (relatedness triage consumes the digest)
#   - internal/prompting/digest (v1: bounded recent-turn projection)
#
# Behavior: the v1 session digest is a pure projection over the event log — the most
# recent enabled conversational turns, windowed, honoring disabled turns and ignoring
# non-turn events. Cold start renders empty so triage returns "new".

@prompting @digest @arch:fan-groups
Feature: v1 session digest projects recent turns
  As the relatedness-triage stage
  I want a compact catch-up note built from the event log
  So that a new turn can be placed against the session without the whole history

  # use-case: UC-DIGEST-BUILD  (TC-DIG-001)
  @unit
  Scenario: The digest projects user and agent turns in order
    Given a user turn "set up auth"
    And an agent turn "done, added login"
    And a user turn "now add tests"
    When the digest is built keeping 10 turns
    Then the digest has 3 turns
    And the digest render contains "user: set up auth"
    And the digest render contains "agent: done, added login"
    And the digest cursor is 3

  # use-case: UC-DIGEST-WINDOW  (TC-DIG-002)
  @unit
  Scenario: The digest keeps only the most recent turns
    Given a user turn "one"
    And a user turn "two"
    And a user turn "three"
    And a user turn "four"
    When the digest is built keeping 2 turns
    Then the digest has 2 turns
    And the digest turn count is 4
    And the digest render contains "four"
    And the digest render omits "one"

  # use-case: UC-DIGEST-DISABLED  (TC-DIG-003)
  @unit
  Scenario: Disabled turns are excluded from the digest
    Given a user turn "keep me"
    And a disabled agent turn "forget me"
    And a user turn "keep me too"
    When the digest is built keeping 10 turns
    Then the digest has 2 turns
    And the digest render omits "forget me"

  # use-case: UC-DIGEST-NONTURN  (TC-DIG-004)
  @unit
  Scenario: Non-turn events are ignored
    Given a user turn "hello"
    And a thinking event
    And a tool_call event
    And an agent turn "hi there"
    When the digest is built keeping 10 turns
    Then the digest has 2 turns

  # use-case: UC-DIGEST-COLD  (TC-DIG-005)
  @unit
  Scenario: A cold session renders an empty digest
    When the digest is built keeping 10 turns
    Then the digest has 0 turns
    And the digest render is empty
    And the digest cursor is 0
