# Source contracts:
#   - docs/architecture/adr/0013-llm-provider-abstraction.md
#   - internal/llm/provider (the backend-agnostic Provider interface)
#
# Behavior: the invoker uses a provider's FormatStyle to decide whether to send
# a JSON-schema format field or to inject a JSON instruction into the user prompt.
# The fan-out pool and the rest of the runtime see no difference between backends.

@llm @provider @arch:llm-provider
Feature: Provider abstraction drives format strategy
  As the agentX invoker
  I want to delegate format handling to the provider's FormatStyle
  So that the invoker stays backend-agnostic while each provider uses its
  native constrained-decoding capability when available

  # use-case: UC-PROVIDER-NATIVE  (TC-PROV-001)
  # Ollama-style: provider sends "format" in the payload unchanged.
  @unit
  Scenario: A Native-style provider receives the schema verbatim
    Given an invoker backed by a stub provider with FormatStyle "native"
    And an invocation requiring fields "relation" and "confidence"
    When the invoker runs the provider invocation
    Then the provider received a format schema
    And the provider received a prompt with no JSON instruction

  # use-case: UC-PROVIDER-PROMPT  (TC-PROV-002)
  # llama.cpp-style: provider does NOT get "format"; the invoker injects a JSON
  # instruction into the user prompt instead.
  @unit
  Scenario: A Prompt-style provider receives JSON instruction in the prompt
    Given an invoker backed by a stub provider with FormatStyle "prompt"
    And an invocation requiring fields "relation" and "confidence"
    When the invoker runs the provider invocation
    Then the provider received no format schema
    And the provider received a prompt containing a JSON instruction

  # use-case: UC-PROVIDER-UNCONSTRAINED  (TC-PROV-003)
  # When no contract is declared, neither style sends anything special.
  @unit
  Scenario: Unconstrained output sends neither schema nor JSON instruction
    Given an invoker backed by a stub provider with FormatStyle "native"
    And an invocation with no output contract
    When the invoker runs the provider invocation
    Then the provider received no format schema
    And the provider received a prompt with no JSON instruction

  # use-case: UC-PROVIDER-CHAT  (TC-PROV-004)
  # Chat (streaming) does not use Format at all — it is Complete-only.
  @unit
  Scenario: Chat dispatches messages unchanged regardless of FormatStyle
    Given an invoker backed by a stub provider with FormatStyle "prompt"
    And a streaming chat request for model "m" with messages
    When the invoker dispatches chat
    Then the provider received the messages unchanged

  # use-case: UC-PROVIDER-ADAPTER  (TC-PROV-005)
  # The runtime adapter delegates to the provider — the Model interface does not
  # need to know about FormatStyle.
  @unit
  Scenario: The runtime model adapter delegates Complete to the provider
    Given a runtime model adapter wrapping a stub provider with FormatStyle "native"
    When the adapter runs the completion
    Then the adapter forwards the request to the provider unchanged

  # use-case: UC-PROVIDER-ADAPTER  (TC-PROV-006)
  @unit
  Scenario: The runtime model adapter delegates Chat to the provider
    Given a runtime model adapter wrapping a stub provider with FormatStyle "prompt"
    When the adapter runs the chat
    Then the provider received the messages unchanged
