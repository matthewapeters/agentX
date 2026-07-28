# Source contracts:
#   - docs/architecture/adr/0013-llm-provider-abstraction.md
#   - internal/llm/llamacpp (HTTP server client)
#
# Behavior: the llamacpp.Client speaks the llama.cpp server's OpenAI-compatible
# API. It streams deltas on success, surfaces HTTP and JSON errors, and delegates
# readiness to GET /v1/models. The LlamacppProvider wrapper converts between
# llamacpp-native request types and provider.Provider so the invoker stays
# backend-agnostic.

@llm @llamacpp @arch:llm-provider
Feature: llama.cpp HTTP adapter
  As the agentX invoker
  I want to speak the OpenAI-compatible API exposed by llama-server
  So that a locally-running llama.cpp instance can act as a backend

  # use-case: UC-LLAMACPP-CHAT  (TC-LLAMA-001)
  @unit
  Scenario: Chat streams content deltas
    Given a stub llama.cpp server that streams "Hel" then "lo"
    When a llama.cpp chat request for model "test" is sent with prompt "hi"
    Then the streamed llama.cpp deltas are "Hel" then "lo"
    And the assembled llama.cpp response is "Hello"

  # use-case: UC-LLAMACPP-ERROR  (TC-LLAMA-002)
  @unit
  Scenario: Chat surfaces HTTP errors
    Given a stub llama.cpp server that returns HTTP 500
    When a llama.cpp chat request for model "test" is sent with prompt "hi"
    Then the llama.cpp chat returns an error

  # use-case: UC-LLAMACPP-NUMCTX  (TC-LLAMA-003)
  @unit
  Scenario: Chat forwards n_ctx to the server
    Given a stub llama.cpp server that streams "" then ""
    When a llama.cpp chat request for model "test" is sent with prompt "hi" and context window 4096
    Then the llama.cpp chat request set n_ctx to 4096

  # use-case: UC-LLAMACPP-PROMPT-INJECT  (TC-LLAMA-004)
  # The invoker injects a JSON instruction into the user prompt. The server
  # receives the prompt verbatim (no format field in the request body).
  @unit
  Scenario: Complete forwards prompt with JSON instruction, no format field
    Given a stub llama.cpp server recording request bodies
    When a llama.cpp complete request for model "test" is sent with a prompt containing JSON instruction
    Then the llama.cpp completion returns the server's reply unchanged
    And the llama.cpp server received the prompt verbatim
    And the llama.cpp server received a request body containing "JSON"
    And the llama.cpp server received a request body containing no "format" field

  # use-case: UC-LLAMACPP-READY  (TC-LLAMA-005)
  @unit
  Scenario: Readiness passes when the model is listed
    Given a stub llama.cpp server listing model "test"
    When llama.cpp readiness is checked for model "test"
    Then llama.cpp readiness passes

  # use-case: UC-LLAMACPP-READY  (TC-LLAMA-006)
  @unit
  Scenario: Readiness fails when the model is missing
    Given a stub llama.cpp server listing model "other"
    When llama.cpp readiness is checked for model "test"
    Then llama.cpp readiness fails with "not available"

  # use-case: UC-LLAMACPP-CONTEXT  (TC-LLAMA-007)
  @unit
  Scenario: Context length reports the server-reported window
    Given a stub llama.cpp server reporting context length 4096 for model "test"
    When llama.cpp context length is requested for model "test"
    Then the llama.cpp context length is 4096

  # use-case: UC-LLAMACPP-CONTEXT  (TC-LLAMA-008)
  @unit
  Scenario: Context length reports 0 when server omits the field
    Given a stub llama.cpp server listing model "test" with no context length
    When llama.cpp context length is requested for model "test"
    Then the llama.cpp context length is 0

  # use-case: UC-LLAMACPP-ADAPTER  (TC-LLAMA-009)
  # LlamacppProvider satisfies provider.Provider so the invoker stays
  # backend-agnostic.
  @unit
  Scenario: LlamacppProvider satisfies provider.Provider
    Given a stub llama.cpp server recording request bodies
    Then the LlamacppProvider satisfies provider.Provider
