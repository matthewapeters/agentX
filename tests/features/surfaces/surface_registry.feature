# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md
#       (Surface Registration Contract, Attach security contract, Registration Mechanics TRN-1)
#   - docs/architecture/runtime_contracts/surface-registration.schema.json
#   - docs/build-plan/05_transport_backlog.md (TRN-1)
#
# Behavior: the orchestrator mints one ephemeral attach token per session; the
# surface registry validates that token, records registrations against the frozen
# schema, manages lifecycle (ready/stopped), rejects bad tokens and id conflicts,
# and never exposes the raw token.

@functional @arch:surface-registry
Feature: Surface registry and attach tokens
  As the orchestrator
  I want external surfaces to register only with a valid per-session token
  So that the canonical state is shared safely across separate processes

  # use-case: UC-REG-ACCEPT  (TC-M1-registry-001)
  Scenario: Register a surface with a valid attach token
    Given a session registry with a minted attach token
    When a "files" surface registers with the valid token
    Then the registration is accepted
    And the surface lifecycle state is "ready"
    And the stored record carries the token fingerprint not the raw token

  # use-case: UC-REG-SCHEMA  (TC-M1-registry-002)
  Scenario: An accepted registration carries the frozen required fields
    Given a session registry with a minted attach token
    When a "files" surface registers with the valid token
    Then the registration is accepted
    And the record has the required surface-registration fields

  # use-case: UC-REG-AUTH  (TC-M1-registry-003)
  Scenario: Reject registration with a wrong token
    Given a session registry with a minted attach token
    When a "files" surface registers with the token "not-the-token"
    Then the registration is rejected with category "auth"
    And the registry has 0 surfaces

  # use-case: UC-REG-AUTH  (TC-M1-registry-004)
  # variant: empty token
  Scenario: Reject registration with a missing token
    Given a session registry with a minted attach token
    When a "files" surface registers with the token ""
    Then the registration is rejected with category "auth"
    And the registry has 0 surfaces

  # use-case: UC-REG-CONFLICT  (TC-M1-registry-005)
  Scenario: Reject a conflicting surface id
    Given a session registry with a minted attach token
    And a "files" surface "files-1" is registered with the valid token
    When a surface "files-1" registers with the valid token
    Then the registration is rejected with category "conflict"
    And the registry has 1 surface

  # use-case: UC-REG-LIFECYCLE  (TC-M1-registry-006)
  Scenario: Shut a surface down and re-register it
    Given a session registry with a minted attach token
    And a "files" surface "files-1" is registered with the valid token
    When surface "files-1" is shut down
    Then the surface "files-1" lifecycle state is "stopped"
    When a surface "files-1" registers with the valid token
    Then the registration is accepted
    And the surface "files-1" lifecycle state is "ready"

  # use-case: UC-REG-LIST  (TC-M1-registry-007)
  Scenario: The registry lists surfaces deterministically by id
    Given a session registry with a minted attach token
    And a "files" surface "files-b" is registered with the valid token
    And a "config" surface "config-a" is registered with the valid token
    Then the registry lists surfaces in order "config-a, files-b"
