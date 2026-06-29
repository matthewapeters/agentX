# Source contracts:
#   - docs/implementation/05_security_approvals_and_command_policy.md (policy layers,
#     evaluation order, descriptor contract, execution safety)
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-1)
#
# Behavior: the command policy validates a tool call's arguments against the
# descriptor schema, then evaluates blacklist -> global whitelist -> session
# whitelist -> approval. Blacklist always wins; deny/approval carry reason codes.

@unit @arch:tool-policy
Feature: Command policy evaluation
  As the AgentX tool runtime
  I want every proposed tool call evaluated against policy
  So that unsafe commands are blocked and risky ones are gated by approval

  # use-case: UC-TOOL-POLICY
  Scenario: A read-only tool runs without approval
    Given a tool policy
    When "read_file" is evaluated with:
      | arg  | value         |
      | path | ./agentx.toml |
    Then the decision is "allow"

  # use-case: UC-TOOL-POLICY
  # variant: mutating-needs-approval
  Scenario: A mutating tool needs approval
    Given a tool policy
    When "write_file" is evaluated with:
      | arg     | value       |
      | path    | ./notes.txt |
      | content | hello       |
    Then the decision is "needs_approval"
    And the reason is "approval_required"

  # use-case: UC-TOOL-POLICY
  # variant: session-approval-allows
  Scenario: A session-approved tool is allowed
    Given a tool policy
    And "write_file" is approved for the "session" with:
      | arg     | value       |
      | path    | ./notes.txt |
      | content | hello       |
    When "write_file" is evaluated with:
      | arg     | value       |
      | path    | ./notes.txt |
      | content | hello       |
    Then the decision is "allow"

  # use-case: UC-TOOL-POLICY
  # variant: global-approval-allows
  Scenario: A globally-approved tool is allowed
    Given a tool policy
    And "http_get" is approved for the "global" with:
      | arg | value               |
      | url | https://example.com |
    When "http_get" is evaluated with:
      | arg | value               |
      | url | https://example.com |
    Then the decision is "allow"

  # use-case: UC-TOOL-POLICY
  # variant: blacklist-overrides-approval
  Scenario: Blacklist overrides a prior approval
    Given a tool policy
    And "http_get" is approved for the "global" with:
      | arg | value               |
      | url | https://example.com |
    And the tool "http_get" is blacklisted
    When "http_get" is evaluated with:
      | arg | value               |
      | url | https://example.com |
    Then the decision is "deny"
    And the reason is "blacklisted"

  # use-case: UC-TOOL-POLICY
  # variant: blacklist-by-argument-pattern
  Scenario: A tool is denied when an argument matches a forbidden pattern
    Given a tool policy
    And the tool "read_file" is denied when an argument matches "^/etc/"
    When "read_file" is evaluated with:
      | arg  | value       |
      | path | /etc/shadow |
    Then the decision is "deny"
    And the reason is "blacklisted"

  # use-case: UC-TOOL-POLICY
  # variant: missing-required-arg
  Scenario: A missing required argument is a schema violation
    Given a tool policy
    When "read_file" is evaluated with no arguments
    Then the decision is "deny"
    And the reason is "schema_violation"

  # use-case: UC-TOOL-POLICY
  # variant: unknown-arg
  Scenario: An unknown argument is a schema violation
    Given a tool policy
    When "read_file" is evaluated with:
      | arg   | value |
      | bogus | x     |
    Then the decision is "deny"
    And the reason is "schema_violation"

  # use-case: UC-TOOL-POLICY
  # variant: option-injection-guard
  Scenario: A path argument beginning with a dash is rejected
    Given a tool policy
    When "read_file" is evaluated with:
      | arg  | value |
      | path | -rf   |
    Then the decision is "deny"
    And the reason is "schema_violation"
