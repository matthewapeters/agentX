# Source contracts:
#   - docs/architecture/cascade_classifier.md + prompt_fan_groups.md
#   - internal/llm/invoke (the Ollama-backed fanout.Invoker)
#
# Behavior: the invoker turns a fanout.Invocation into a schema-constrained model
# completion and parses the JSON reply into a fanout.Response the pool votes on.
# The model call is stubbed here (canned/captured) so parsing and request-building
# are deterministic; one @integration scenario exercises the real Ollama HTTP path
# against a fake server.

@llm @invoker @arch:invocation-pool
Feature: Ollama-backed invoker turns invocations into votable responses
  As the classifier fan-out
  I want each invocation sent as a constrained completion and parsed back
  So that a small model's JSON reply becomes a comparable, votable verdict

  # use-case: UC-INVOKE-PARSE  (TC-INV-001)
  @unit
  Scenario: The invoker parses a structured verdict
    Given an invoker whose model returns '{"relation":"continuation","confidence":0.8,"why":"a follow-up"}'
    And an invocation whose verdict field is "relation"
    When the invoker runs the invocation
    Then the response verdict is "continuation"
    And the response confidence is 0.8
    And the response field "why" is "a follow-up"

  # use-case: UC-INVOKE-SCHEMA  (TC-INV-002)
  @unit
  Scenario: The invoker sends the contract as a constrained-decoding schema
    Given an invoker capturing its request
    And an invocation requiring fields "relation" and "confidence" at temperature 0.5
    When the invoker runs the invocation
    Then the sent request format requires "relation" and "confidence"
    And the sent request temperature is 0.5

  # use-case: UC-INVOKE-MALFORMED  (TC-INV-003)
  @unit
  Scenario: Malformed model output yields no verdict, to be quarantined at the fold
    Given an invoker whose model returns 'sorry, I cannot help with that'
    And an invocation whose verdict field is "relation"
    When the invoker runs the invocation
    Then the response has no verdict

  # use-case: UC-INVOKE-FENCED  (TC-INV-004)
  @unit
  Scenario: The invoker tolerates JSON wrapped in prose
    Given an invoker whose model returns 'Sure thing! {"relation":"new","confidence":0.9} hope that helps'
    And an invocation whose verdict field is "relation"
    When the invoker runs the invocation
    Then the response verdict is "new"

  # use-case: UC-INVOKE-OLLAMA  (TC-INV-005)
  @integration
  Scenario: A structured completion reaches Ollama with its options and format
    Given a fake Ollama returning content '{"relation":"new"}'
    When a structured completion is sent at temperature 0.3 with a format schema
    Then the completion returns '{"relation":"new"}'
    And the fake Ollama received a format schema and temperature 0.3
