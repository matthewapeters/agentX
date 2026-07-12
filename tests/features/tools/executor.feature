# Source contracts:
#   - docs/implementation/05_security_approvals_and_command_policy.md (execution safety)
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-2)
#
# Behavior: the executor runs an approved descriptor as an argv vector (no shell),
# captures stdout/stderr/exit, enforces a timeout, persists full output to a
# session artifact, and returns the FULL captured text + ref — never a
# further-truncated excerpt of it. Bounding a result's size is the tool call's
# job (a narrower command); the only truncation the executor itself performs is
# the output_max_bytes capture safety net, and that is always labeled honestly
# in the result text, never silent (RCA: session nimble-pebble-2, 2026-07-12 —
# a line-truncated preview whose header still claimed the untruncated count
# misled a planner into thinking a `tree` pin covered directories it did not).

@integration @arch:tool-executor
Feature: Tool execution
  As the AgentX tool runtime
  I want approved tools executed safely with captured, persisted output
  So that the agent gets structured results and an auditable record

  # use-case: UC-TOOL-EXEC
  Scenario: Reading a file captures output and persists an artifact
    Given a tool executor
    And a file "hello.txt" containing "hello world"
    When the tool "read_file" runs on that file
    Then the result status is "ok"
    And the result exit code is 0
    And the result preview contains "hello world"
    And the result has an artifact ref
    And reading the result output yields "hello world"

  # use-case: UC-TOOL-EXEC
  # variant: failure-captured
  Scenario: A failing command is captured as an error
    Given a tool executor
    When the tool "read_file" runs on a missing file
    Then the result status is "error"
    And the result exit code is not 0

  # use-case: UC-TOOL-EXEC
  # variant: timeout
  Scenario: A command that exceeds its timeout is reported
    Given a tool executor
    When a command sleeps for 5 seconds with a 1 second timeout
    Then the result status is "timeout"

  # use-case: UC-TOOL-EXEC
  # variant: no-shell
  Scenario: Arguments are passed without a shell
    Given a tool executor
    When a command echoes the literal "$HOME"
    Then the result preview contains "$HOME"

  # use-case: UC-TOOL-EXEC
  # variant: full-result-in-context
  Scenario: Output under the byte cap is returned in full, not previewed
    Given a tool executor
    When a command emits 50 numbered lines
    Then the result reports 50 lines
    And the result preview contains "1"
    And the result preview contains "50"

  # use-case: UC-TOOL-EXEC
  # variant: windowed-paging-still-available
  Scenario: A specific window can still be paged from the artifact on request
    Given a tool executor
    When a command emits 50 numbered lines
    Then reading the result output at offset 49 limit 1 yields "50"

  # use-case: UC-TOOL-EXEC
  # variant: byte-cap-truncation-labeled-honestly
  Scenario: Output exceeding the byte capture cap is truncated with an explicit notice
    Given a tool executor with a 16 byte output cap
    When a command emits 50 numbered lines
    Then the result is marked truncated
    And the result preview contains "capture stopped at 16 bytes"
