# Source contracts:
#   - docs/architecture/prompt_fan_groups.md (stage-0 relatedness triage + the
#     context-directive table; conservative-about-dropping-context stance)
#   - internal/prompting/pipeline (digest -> triage -> directive -> action classify)
#
# Behavior: the pipeline chains two cascade stages over a session. Relatedness triage
# runs over the digest and produces a context directive; action classification then
# runs scoped by that directive. Dropping context is catastrophic, so only a confident
# "new" clears the thread; an abstain keeps it.

@prompting @pipeline @arch:fan-groups
Feature: the classify pipeline chains triage into action classification
  As the server's request-handling brain
  I want relatedness triage to scope what the action classifier sees
  So that a new turn is placed against the right context before it is classified

  Background:
    Given the classify pipeline corpus is loaded

  # use-case: UC-PIPE-CHAIN  (TC-PIPE-001)
  @unit
  Scenario: Both stages run and a continuation carries context
    Given the session has a prior turn "set up auth"
    And triage returns "continuation" at confidence 0.9
    And action returns "query" at confidence 0.9
    When the pipeline classifies "now add tests"
    Then the triage relation is "continuation"
    And the action task type is "query"
    And the directive context is not empty

  # use-case: UC-PIPE-NEW  (TC-PIPE-002)
  @unit
  Scenario: A confident "new" drops the thread context
    Given the session has a prior turn "set up auth"
    And triage returns "new" at confidence 0.9
    And action returns "query" at confidence 0.9
    When the pipeline classifies "what's the weather"
    Then the directive relation is "new"
    And the directive context is empty

  # use-case: UC-PIPE-ASIDE  (TC-PIPE-003)
  @unit
  Scenario: A related_aside carries context cautiously
    Given the session has a prior turn "set up auth"
    And triage returns "related_aside" at confidence 0.9
    And action returns "query" at confidence 0.9
    When the pipeline classifies "actually, how does the token refresh work"
    Then the directive relation is "related_aside"
    And the directive is cautious
    And the directive context is not empty

  # use-case: UC-PIPE-COLD  (TC-PIPE-004)
  @unit
  Scenario: A cold session yields empty context even on continuation
    Given triage returns "continuation" at confidence 0.9
    And action returns "query" at confidence 0.9
    When the pipeline classifies "write hello.txt"
    Then the directive context is empty
    And the action task type is "query"

  # use-case: UC-PIPE-RESPONSE  (TC-PIPE-006)  the [C] response classifier
  @unit
  Scenario: The response classifier reads what the model actually did
    Given response classify returns "produced" at confidence 0.9
    When the pipeline classifies the response "here is the file: hello world"
    Then the response classifier verdict is "produced"

  # use-case: UC-PIPE-ABSTAIN  (TC-PIPE-005)
  @unit
  Scenario: A scattered triage vote abstains and keeps context
    Given the session has a prior turn "set up auth"
    And triage variant "direct" returns "continuation" at confidence 0.5
    And triage variant "reframed" returns "new" at confidence 0.5
    And action returns "query" at confidence 0.9
    When the pipeline classifies "hmm about that"
    Then the directive abstained
    And the directive context is not empty
