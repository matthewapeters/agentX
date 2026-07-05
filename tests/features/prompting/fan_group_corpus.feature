# Source contracts:
#   - docs/architecture/prompt_fan_groups.md (fan-group corpus + load/validation rules)
#   - config/seed/prompts.toml (the shipped corpus this loader consumes)
#
# Behavior: internal/prompting/corpus parses the machine-readable fan-group corpus,
# validates it structurally (a broken corpus is refused, not silently misused),
# compiles a group's output contract to a fanout.Contract, and renders a group into
# the []fanout.Invocation the pool runs — one per width, placeholders substituted,
# all sharing the group's contract.

@prompting @corpus @arch:fan-groups
Feature: Load and render the fan-group prompt corpus
  As the AgentX classifier
  I want to load fan-groups from a validated corpus and render them into invocations
  So that the prompts that vote are user-exposed, checked, and comparable

  # use-case: UC-CORPUS-LOAD  (TC-CORP-001)
  @unit
  Scenario: A valid corpus loads its fan-groups
    Given a corpus with a "triage" fan-group of width 3 quorum 2
    When the corpus is parsed
    Then the parse succeeds
    And the corpus has a fan-group "triage"
    And the fan-group "triage" has 3 variants

  # use-case: UC-CORPUS-CONTRACT  (TC-CORP-002)
  @unit
  Scenario: A fan-group compiles its output contract
    Given a corpus with a "triage" fan-group of width 3 quorum 2
    When the corpus is parsed
    Then the fan-group "triage" contract requires "relation" and "confidence"
    And the fan-group "triage" contract bounds words to 40

  # use-case: UC-CORPUS-RENDER  (TC-CORP-003)
  @unit
  Scenario: Rendering fans a group into one invocation per width with context filled
    Given a corpus with a "triage" fan-group of width 3 quorum 2
    And a render context with turn "add a login page"
    When the fan-group "triage" is rendered
    Then 3 invocations are produced
    And every invocation carries the compiled contract
    And every invocation votes on "relation"
    And every invocation prompt substitutes the turn
    And no invocation prompt has an unfilled placeholder
    And the invocations carry more than one distinct temperature

  # use-case: UC-CORPUS-AGG  (TC-CORP-004)
  @unit
  Scenario: A fan-group builds its majority-vote aggregator
    Given a corpus with a "triage" fan-group of width 3 quorum 2
    When the corpus is parsed
    Then the fan-group "triage" aggregator has quorum 2

  # use-case: UC-CORPUS-REJECT  (TC-CORP-005)
  @unit
  Scenario: A corpus with quorum greater than width is rejected
    Given a corpus with a "triage" fan-group of width 3 quorum 5
    When the corpus is parsed
    Then the parse fails mentioning "quorum"

  # use-case: UC-CORPUS-REJECT  (TC-CORP-006)
  @unit
  Scenario: A corpus whose coarse variant is undefined is rejected
    Given a corpus whose coarse variant names a nonexistent variant
    When the corpus is parsed
    Then the parse fails mentioning "coarse_variant"

  # use-case: UC-CORPUS-REJECT  (TC-CORP-007)
  @unit
  Scenario: A corpus with an unknown template placeholder is rejected
    Given a corpus with an unknown placeholder in a template
    When the corpus is parsed
    Then the parse fails mentioning "placeholder"

  # use-case: UC-CORPUS-SEED  (TC-CORP-008)
  # Guards the shipped seed against drifting from the loader's rules.
  @integration
  Scenario: The shipped seed corpus loads and validates
    When the seed corpus at "../../config/seed/prompts.toml" is loaded
    Then the load succeeds
    And the corpus has a fan-group "relatedness_triage"
    And the corpus has a fan-group "action_classify"
