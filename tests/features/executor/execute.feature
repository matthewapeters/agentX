# Source contracts:
#   - docs/architecture/cascade_classifier.md (§ Execute + verify)
#   - internal/executor (drain -> gate -> run -> verify; FSVerifier)
#
# Behavior: the executor drains a task record into a concrete tool call, gates it
# through the command policy, runs it, and verifies the effect. A run whose effect
# cannot be verified is reported as phantom, never as done — the anti-phantom-success
# guarantee that retires the vivid-willow failure class.

@executor @arch:executor
Feature: the executor drains a task into a verified effect
  As the request-handling brain
  I want a recognized action executed and its effect verified
  So that success is never reported for an action that did not actually happen

  # use-case: UC-EXEC-OK  (TC-EXEC-001)
  @unit
  Scenario: A verified run reports executed
    Given a proposer that proposes tool "write_file"
    And the policy allows the call
    And the runner reports status "ok"
    And the effect verifies
    When the task "write hello.txt" is executed
    Then the executor outcome is "executed"

  # use-case: UC-EXEC-PHANTOM  (TC-EXEC-002)  the vivid-willow guarantee
  @unit
  Scenario: A run whose effect is not verified reports phantom
    Given a proposer that proposes tool "write_file"
    And the policy allows the call
    And the runner reports status "ok"
    And the effect does not verify
    When the task "write hello.txt" is executed
    Then the executor outcome is "phantom"

  # use-case: UC-EXEC-DENY  (TC-EXEC-003)
  @unit
  Scenario: A denied call is not run
    Given a proposer that proposes tool "rm_rf"
    And the policy denies the call
    When the task "delete everything" is executed
    Then the executor outcome is "denied"
    And the runner did not run

  # use-case: UC-EXEC-NOTOOL  (TC-EXEC-004)
  @unit
  Scenario: No proposed tool reports no_tool
    Given a proposer that proposes nothing
    When the task "muse about the weather" is executed
    Then the executor outcome is "no_tool"

  # use-case: UC-EXEC-CONFINE-IN  (TC-EXEC-007)
  @unit
  Scenario: A call inside the working directory runs normally
    Given a proposer that proposes writing to "notes/out.txt"
    And the executor is confined to "/work"
    And the policy allows the call
    And the runner reports status "ok"
    And the effect verifies
    When the task "write a note" is executed
    Then the executor outcome is "executed"

  # use-case: UC-EXEC-CONFINE-NEEDS  (TC-EXEC-008)  the safety boundary
  @unit
  Scenario: A call outside the working directory needs approval and does not run
    Given a proposer that proposes writing to "/etc/passwd"
    And the executor is confined to "/work"
    And the policy allows the call
    When the task "overwrite passwd" is executed
    Then the executor outcome is "needs_approval"
    And the runner did not run

  # use-case: UC-EXEC-CONFINE-APPROVE  (TC-EXEC-009)
  @unit
  Scenario: An approved out-of-directory call runs
    Given a proposer that proposes writing to "../outside.txt"
    And the executor is confined to "/work"
    And the policy allows the call
    And the user approves the flagged call
    And the runner reports status "ok"
    And the effect verifies
    When the task "write outside" is executed
    Then the executor outcome is "executed"

  # use-case: UC-EXEC-CONFINE-DECLINE  (TC-EXEC-010)
  @unit
  Scenario: A declined out-of-directory call does not run
    Given a proposer that proposes writing to "../outside.txt"
    And the executor is confined to "/work"
    And the policy allows the call
    And the user declines the flagged call
    When the task "write outside" is executed
    Then the executor outcome is "denied"
    And the runner did not run

  # use-case: UC-EXEC-VERIFY-FILE  (TC-EXEC-005)
  @unit
  Scenario: The filesystem verifier confirms a written file
    Given a runner result with status "ok" and exit 0
    And the call wrote a non-empty file at "out.txt"
    When the filesystem effect is verified
    Then the effect is confirmed

  # use-case: UC-EXEC-VERIFY-PHANTOM  (TC-EXEC-006)
  @unit
  Scenario: The filesystem verifier rejects a missing file
    Given a runner result with status "ok" and exit 0
    And the call named a file at "gone.txt" that was never written
    When the filesystem effect is verified
    Then the effect is rejected
