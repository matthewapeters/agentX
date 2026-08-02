# Source contracts:
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (Native tool calls)
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-4)
#
# Behavior: a model-issued native tool call runs under policy, and the loop
# answers with the result folded back in. Ordering is user_prompt → tool_call →
# tool_result → agent_response — no classification event (the native
# tool-calling loop has no classify step).

@e2e @arch:tool-cycle
Feature: Native tool-call cycle
  As the AgentX runtime
  I want a model-issued native tool call to run and answer with its result
  So that the agent can act on the local environment, not just talk about it

  # use-case: UC-TOOL-CYCLE
  Scenario: A native tool call runs a tool then answers
    Given a started orchestrator that runs the "read_file" tool and replies "here it is"
    When the prompt "show me the file" runs the tool cycle
    Then the tool cycle's content events are, in order:
      | content_type   |
      | user_prompt    |
      | tool_call      |
      | tool_result    |
      | agent_response |
    And the tool cycle's final state is "completed"
    And the tool cycle answer is "here it is"
