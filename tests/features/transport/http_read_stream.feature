# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md
#       (HTTP API Baseline v1: Read endpoints, Streaming endpoints;
#        Transport directionality; Read & Streaming Server TRN-2)
#   - docs/architecture/runtime_contracts/processing-state.schema.json
#   - docs/architecture/runtime_contracts/event-envelope.schema.json
#   - docs/architecture/runtime_contracts/surface-registration.schema.json
#   - docs/build-plan/05_transport_backlog.md (TRN-2)
#
# Behavior: the loopback HTTP transport adapts the orchestrator's canonical state.
# Read endpoints return snapshots; GET /events streams the event bus as SSE with
# independent per-connection subscriptions.

@integration @arch:transport
Feature: HTTP transport read and streaming endpoints
  As an external surface process
  I want to read canonical session state and stream events over HTTP/SSE
  So that I can render the same conversation as the in-process chat surface

  # use-case: UC-HTTP-HEALTH  (TC-M1-transport-001)
  Scenario: Health check reports ok with the session id
    Given a running transport server
    When a client GETs "/health"
    Then the response status is 200
    And the response JSON field "status" is "ok"
    And the response JSON field "session_id" is not empty

  # use-case: UC-HTTP-SESSION  (TC-M1-transport-002)
  Scenario: Current session endpoint reports the session identity
    Given a running transport server for session "calm-otter"
    When a client GETs "/sessions/current"
    Then the response status is 200
    And the response JSON field "session_name" is "calm-otter"

  # use-case: UC-HTTP-PROCSTATE  (TC-M1-transport-003)
  Scenario: Processing-state endpoint reports the current state and phase
    Given a running transport server
    And the processing state is set to "working" phase "respond"
    When a client GETs "/processing-state"
    Then the response status is 200
    And the response JSON field "state" is "working"
    And the response JSON field "phase" is "respond"

  # use-case: UC-HTTP-SURFACES  (TC-M1-transport-004)
  Scenario: Surfaces endpoint lists registered surfaces
    Given a running transport server
    And a surface "files-1" is registered on the transport
    When a client GETs "/surfaces"
    Then the response status is 200
    And the response body contains "files-1"

  # use-case: UC-HTTP-SSE  (TC-M1-transport-005)
  Scenario: Events stream delivers a published event over SSE
    Given a running transport server
    And an open events stream
    When an agent_response event "hello there" is published
    Then the events stream delivers "hello there"

  # use-case: UC-HTTP-SSE-FANOUT  (TC-M1-transport-006)
  # variant: a slow/extra consumer does not block another
  Scenario: Two concurrent streams both receive a published event
    Given a running transport server
    And 2 open events streams
    When an agent_response event "broadcast" is published
    Then all events streams deliver "broadcast"
