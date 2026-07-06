@functional
Feature: Conversation context continuity
  Each turn is assembled with the prior enabled turns folded in, so the model has
  conversational continuity. User prompts and agent responses are enabled by
  default; thinking and tool events are retained but disabled by default.
  (docs/implementation/04_llm_prompt_tooling_runtime.md, ADR 0006)

  Scenario: A second turn carries the first turn's prompt and response
    Given a started orchestrator with no bootstrap prompt and a capturing model that replies "hi"
    When the user submits the prompt "hello"
    And the user submits the prompt "again"
    Then the model context includes in order:
      | role      | content |
      | user      | hello   |
      | assistant | hi      |
      | user      | again   |

  Scenario: The bootstrap exchange is excluded from later context
    Given a started orchestrator with instructions "Sys." and bootstrap prompt "Are you there?" and a capturing model that replies "ready"
    When the orchestrator runs its bootstrap prompt
    And the user submits the prompt "real question"
    Then the model context excludes "Are you there?"
    And the model context excludes "ready"
    And the model context includes in order:
      | role | content       |
      | user | real question |

  Scenario: User and agent turns persist as context-enabled
    Given a started orchestrator with no bootstrap prompt and a capturing model that replies "hi"
    When the user submits the prompt "hello"
    And the bootstrap session is flushed to disk
    Then the persisted "user_prompt" events are context-enabled
    And the persisted "agent_response" events are context-enabled
