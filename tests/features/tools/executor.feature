# Source contracts:
#   - docs/implementation/05_security_approvals_and_command_policy.md (execution safety)
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-2)
#
# Behavior: the executor runs an approved descriptor as an argv vector (no shell),
# captures stdout/stderr/exit, enforces a timeout, persists full output to a
# session artifact, and returns a compact preview + ref.

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
  # variant: large-output-paged
  Scenario: Large output is previewed and fully paged from the artifact
    Given a tool executor
    When a command emits 50 numbered lines
    Then the result reports 50 lines
    And the result preview contains "1"
    And the result preview does not contain "50"
    And reading the result output at offset 49 limit 1 yields "50"
