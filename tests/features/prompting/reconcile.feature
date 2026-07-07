# Source contracts:
#   - docs/architecture/cascade_classifier.md (§ Reconciliation — the [B]+[C] fold)
#   - internal/prompting/reconcile
#
# Behavior: the reconciler folds the turn signal (did the user ask for an action?)
# and the response signal (did the model produce or execute one?) into a route.
# Neither decides alone; an abstain on either side routes to Ask. The vivid-willow
# case (requested + produced-not-executed) routes to Reify.

@prompting @reconcile @arch:classifier
Feature: reconcile the turn and response classifications into a route
  As the routing stage
  I want the requested action and the produced action folded together
  So that a committed-but-unexecuted action is caught rather than lost

  # use-case: UC-RECON-VERIFY  (TC-RECON-001)
  @unit
  Scenario: Requested and executed routes to verify
    Given the turn is actionable
    And the response executed the action
    When the classifications are reconciled
    Then the route is "verify"

  # use-case: UC-RECON-REIFY  (TC-RECON-002)  the vivid-willow case
  @unit
  Scenario: Requested but only produced in prose routes to reify
    Given the turn is actionable
    And the response produced the action without executing it
    When the classifications are reconciled
    Then the route is "reify"

  # use-case: UC-RECON-REDISPATCH  (TC-RECON-003)
  @unit
  Scenario: Requested but dropped routes to redispatch
    Given the turn is actionable
    And the response did nothing
    When the classifications are reconciled
    Then the route is "redispatch"

  # use-case: UC-RECON-CONFIRM  (TC-RECON-004)
  @unit
  Scenario: A volunteered action routes to confirm
    Given the turn is not actionable
    And the response executed the action
    When the classifications are reconciled
    Then the route is "confirm"

  # use-case: UC-RECON-NONE  (TC-RECON-005)
  @unit
  Scenario: Pure conversation routes to none
    Given the turn is not actionable
    And the response did nothing
    When the classifications are reconciled
    Then the route is "none"

  # use-case: UC-RECON-ASK  (TC-RECON-006)
  @unit
  Scenario: An abstained turn routes to ask regardless of the response
    Given the turn classification abstained
    And the response executed the action
    When the classifications are reconciled
    Then the route is "ask"

  # use-case: UC-RECON-DECOMPOSE  (TC-RECON-007)  ADR-0008 Phase 4a
  # An actionable turn whose response scattered toward "produced" is a compound goal the
  # model narrated across steps — reify a plan (decompose), not one call, and not Ask.
  @unit
  Scenario: An actionable turn with a produced-leaning abstain routes to decompose
    Given the turn is actionable
    And the response abstained leaning toward produced
    When the classifications are reconciled
    Then the route is "decompose"

  # use-case: UC-RECON-DECOMPOSE-NOT  (TC-RECON-008)
  # The discriminator: an abstain scattered toward "none" is genuine ambiguity → Ask.
  @unit
  Scenario: An actionable turn with a none-scattered abstain routes to ask
    Given the turn is actionable
    And the response abstained with a scatter toward none
    When the classifications are reconciled
    Then the route is "ask"

  # use-case: UC-RECON-DECOMPOSE-TURN  (TC-RECON-009, clever-raven-3)
  # The turn-side mirror of TC-RECON-007: an abstained ACTION vote whose spread still
  # leaned actionable (not "none") is not genuinely ambiguous about whether it's an action —
  # only about which one. Reify a plan rather than silently punting to Ask, regardless of
  # what the response classifier says (it may resolve cleanly, as it did here — "neither").
  @unit
  Scenario: An abstained turn leaning actionable routes to decompose even when the response is clean
    Given the turn abstained leaning toward actionable
    And the response did nothing
    When the classifications are reconciled
    Then the route is "decompose"

  # use-case: UC-RECON-DECOMPOSE-TURN-NOT  (TC-RECON-010)
  # The discriminator's negative case on the turn side: an action abstain scattered toward
  # "none" is genuine ambiguity about whether this is an action at all → still Ask.
  @unit
  Scenario: An abstained turn scattered toward none still routes to ask
    Given the turn abstained with a scatter toward none
    And the response did nothing
    When the classifications are reconciled
    Then the route is "ask"
