# Source contract: docs/implementation/05_security_approvals_and_command_policy.md
#   docs/build-plan/04_tool_runtime_backlog.md (TOOL-5)
#
# Behavior: the command policy persists across sessions. The blacklist loads from a
# TOML file; global approvals are saved and reloaded so an approval survives a restart.

@unit @arch:command-policy
Feature: Persisted command policy
  As the AgentX runtime
  I want the command policy to persist across sessions
  So that global approvals and blacklist rules survive a restart

  # use-case: UC-TOOL-PERSIST
  Scenario: A global approval survives a restart
    Given a global approval of "read_file" with:
      | name | value          |
      | path | /home/me/n.txt |
    When the approvals are saved and reloaded into a fresh policy
    And "read_file" is evaluated with:
      | name | value          |
      | path | /home/me/n.txt |
    Then the decision is "allow"

  # use-case: UC-TOOL-PERSIST
  # variant: blacklist-loaded-from-disk
  Scenario: A blacklist rule loaded from disk denies a tool
    Given a blacklist file denying tool "*" matching "/\.ssh/"
    When the blacklist is loaded into a fresh policy
    And "read_file" is evaluated with:
      | name | value             |
      | path | /home/me/.ssh/key |
    Then the decision is "deny"
    And the reason is "sensitive_path"
