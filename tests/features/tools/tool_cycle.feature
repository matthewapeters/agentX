# Source contracts:
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (The single_tool cycle)
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-4)
#
# Behavior: a single_tool prompt proposes one tool call, runs it under policy, and
# answers with the result. Ordering is user_prompt → classification → tool_call →
# tool_result → agent_response. Read-only mode blocks mutating tools.

@e2e @arch:tool-cycle
Feature: Single-tool prompt cycle
  As the AgentX runtime
  I want a classified single_tool prompt to run a tool and answer with its result
  So that the agent can act on the local environment, not just talk about it

  # use-case: UC-TOOL-CYCLE
  Scenario: A single_tool prompt runs a tool then answers
    Given a started orchestrator that runs the "read_file" tool and replies "here it is"
    When the prompt "show me the file" runs the tool cycle
    Then the tool cycle's content events are, in order:
      | content_type   |
      | user_prompt    |
      | classification |
      | tool_call      |
      | tool_result    |
      | agent_response |
    And the tool cycle's final state is "completed"
    And the tool cycle answer is "here it is"

  # use-case: UC-TOOL-CYCLE
  # variant: read-only-blocks-write
  Scenario: A write tool is blocked in read-only mode
    Given a started orchestrator in read-only mode that proposes "write_file" and replies "blocked"
    When the prompt "edit the file" runs the tool cycle
    Then a tool_result records status "denied"
    And the tool cycle's final state is "completed"
