# Source contracts:
#   - docs/architecture/cascade_classifier.md (Tier 0/1/2 cascade)
#   - internal/prompting/cascade (the single fan-group cascade runner)
#
# Behavior: the runner runs a fan-group as a cascade — a coarse gate at R=1 that
# is accepted when confident and low-stakes, otherwise escalating to the bounded
# majority vote. The model is stubbed (verdict + confidence scripted per variant)
# so escalation decisions are deterministic and Ollama-free.

@prompting @cascade @arch:invocation-pool
Feature: Single fan-group cascade runner
  As the classifier
  I want a fan-group to answer cheaply when confident and vote only when unsure
  So that the common turn stays at R=1 and accuracy is spent where it matters

  # use-case: UC-CASCADE-ACCEPT  (TC-CASC-001)
  @unit
  Scenario: A confident, low-stakes coarse answer is accepted without a vote
    Given a "triage" cascade group
    And the coarse gate returns "continuation" at confidence 0.9
    When the cascade runs
    Then the cascade does not escalate
    And the cascade verdict is "continuation"

  # use-case: UC-CASCADE-UNSURE  (TC-CASC-002)
  @unit
  Scenario: An unsure coarse answer escalates to a vote
    Given a "triage" cascade group
    And the coarse gate returns "continuation" at confidence 0.4
    And the vote agrees on "continuation"
    When the cascade runs
    Then the cascade escalates
    And the cascade verdict is "continuation"

  # use-case: UC-CASCADE-STAKES  (TC-CASC-003)
  @unit
  Scenario: A high-stakes coarse verdict escalates even when confident
    Given a high-stakes cascade group escalating "artifact"
    And the coarse gate returns "artifact" at confidence 0.95
    And the vote agrees on "artifact"
    When the cascade runs
    Then the cascade escalates
    And the cascade verdict is "artifact"

  # use-case: UC-CASCADE-ABSTAIN  (TC-CASC-004)
  @unit
  Scenario: An escalated vote that scatters abstains rather than guessing
    Given a "triage" cascade group
    And the coarse gate returns "continuation" at confidence 0.4
    And the vote scatters across "new", "orthogonal", "continuation"
    When the cascade runs
    Then the cascade escalates
    And the cascade abstains
