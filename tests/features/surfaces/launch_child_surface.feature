# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md
#       (Normative CLI Specification; Launch Implementation TRN-5;
#        Gherkin Behavior Contracts)
#   - docs/build-plan/05_transport_backlog.md (TRN-5)
#
# Behavior: `agentx surface launch` attaches an external surface to a running
# orchestrator session over HTTP, validating the surface kind, session selector,
# local-safe endpoint, and attach token, with deterministic reason categories.

@e2e @ux:CLI @arch:surface-registry
Feature: Launch a child surface
  As a user
  I want to launch a surface in another terminal and attach it to my session
  So that I can arrange independent surfaces with my multiplexer

  # use-case: UC-LAUNCH-CANONICAL  (TC-M1-launch-001)
  Scenario: Launch with the canonical command and a valid token
    Given a running transport server
    When I launch a "files" surface for session "calm-otter" with the valid token
    Then the launch succeeds
    And the launched surface kind is "files"
    And the launched surface appears in the registry

  # use-case: UC-LAUNCH-ALIAS  (TC-M1-launch-002)
  Scenario: Launch with the compatibility alias
    Given a running transport server
    When I launch via the compatibility alias for the running server
    Then the launch succeeds
    And the launched surface kind is "files"

  # use-case: UC-LAUNCH-AUTO  (TC-M2-ss5-003)
  # source: docs/implementation/02_surface_orchestration_http.md (Flagless launch SS-5)
  Scenario: Launch auto-resolves the endpoint and token from disk
    Given a running transport server
    And the session's transport is published to a temp session root
    When I launch a "files" surface with auto-discovery
    Then the launch succeeds
    And the launched surface kind is "files"
    And the launched surface appears in the registry

  # use-case: UC-LAUNCH-MULTI  (TC-M2-ss5-004)
  # source: docs/implementation/02_surface_orchestration_http.md (Flagless launch SS-5)
  Scenario: Auto-discovery attaches to the session named by --session
    Given a running transport server for session "calm-otter"
    And a second running transport server for session "brave-fox"
    And both sessions are published to a temp session root
    When I launch a "files" surface for session "brave-fox" with auto-discovery
    Then the launch succeeds
    And the launched session is "brave-fox"

  # use-case: UC-LAUNCH-MULTI  (TC-M2-ss5-005)
  # variant: ambiguous without a selector
  Scenario: Auto-discovery refuses to guess among multiple sessions
    Given a running transport server for session "calm-otter"
    And a second running transport server for session "brave-fox"
    And both sessions are published to a temp session root
    When I launch a "files" surface with auto-discovery
    Then the launch fails with category "validation"

  # use-case: UC-LAUNCH-AUTH  (TC-M1-launch-003)
  Scenario: Reject attach without a valid token
    Given a running transport server
    When I launch a "files" surface for session "calm-otter" with token "bad-token"
    Then the launch fails with category "auth"

  # use-case: UC-LAUNCH-KIND  (TC-M1-launch-004)
  Scenario: Reject an unknown surface kind
    Given a running transport server
    When I launch a "wibble" surface for session "calm-otter" with the valid token
    Then the launch fails with category "validation"

  # use-case: UC-LAUNCH-SESSION  (TC-M1-launch-005)
  Scenario: Reject an unresolved session selector
    Given a running transport server
    When I launch a "files" surface for session "no-such-session" with the valid token
    Then the launch fails with category "validation"

  # use-case: UC-LAUNCH-TRANSPORT  (TC-M1-launch-006)
  Scenario: Reject an unreachable endpoint
    Given a running transport server
    When I launch a "files" surface for session "calm-otter" against an unreachable endpoint
    Then the launch fails with category "transport"
