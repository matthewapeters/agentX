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

  # use-case: UC-OLLAMA-CTXLEN
  # The model's max context window is read from /api/show model_info, keyed by
  # the model's own architecture (SS-7: visualizer denominator + chat num_ctx).
  Scenario: Context length is read from the model's architecture
    Given a stub Ollama server reporting architecture "qwen2" with context length 32768
    When the context length for model "test" is fetched
    Then the reported context length is 32768

  # use-case: UC-OLLAMA-CTXLEN
  # variant: missing model_info
  Scenario: Context length lookup fails when unreported
    Given a stub Ollama server listing model "test"
    When the context length for model "test" is fetched
    Then the context length lookup fails

  # use-case: UC-OLLAMA-NUMCTX
  # The requested context window reaches Ollama as options.num_ctx so the model
  # uses its full window instead of the small server default.
  Scenario: Chat sends the context window as num_ctx
    When a chat request for model "test" is sent with prompt "hi" and context window 8192
    Then the chat request set num_ctx to 8192
