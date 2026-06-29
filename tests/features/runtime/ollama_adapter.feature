# Source contracts:
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (Default Model Service)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-C1)
#
# Behavior: the Ollama adapter streams chat content deltas, assembles the full
# response, probes model readiness, and surfaces server errors.

@integration @arch:ollama-adapter
Feature: Ollama chat adapter
  As the agentx runtime
  I want to stream completions from a local Ollama
  So that the chat surface can render live responses

  # use-case: UC-OLLAMA-CHAT
  Scenario: Chat streams content deltas
    Given a stub Ollama server that streams "Hel" then "lo"
    When a chat request for model "test" is sent with prompt "hi"
    Then the streamed deltas are "Hel" then "lo"
    And the assembled response is "Hello"

  # use-case: UC-OLLAMA-READY
  Scenario: Readiness passes when the model is listed
    Given a stub Ollama server listing model "test"
    When readiness is checked for model "test"
    Then readiness passes

  # use-case: UC-OLLAMA-READY
  # variant: missing
  Scenario: Readiness fails when the model is missing
    Given a stub Ollama server listing model "other"
    When readiness is checked for model "test"
    Then readiness fails

  # use-case: UC-OLLAMA-ERROR
  Scenario: Chat surfaces a server error
    Given a stub Ollama server that returns HTTP 500
    When a chat request for model "test" is sent with prompt "hi"
    Then the chat returns an error
