# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md
#       (Surface Model; Launch behavior; Serve-Alongside Lifecycle TRN-6)
#   - docs/implementation/01_runtime_blueprint.md (Runtime Lifecycle)
#   - docs/build-plan/05_transport_backlog.md (TRN-6)
#
# Behavior: a started orchestrator serves the HTTP/SSE transport in lifecycle
# order; an external surface attaches with the launch CLI and round-trips a prompt;
# shutdown stops the server and marks attached surfaces stopped.

@functional @arch:runtime-bootstrap @arch:transport
Feature: Serve the transport alongside the runtime
  As a user
  I want agentx to serve the transport and let surfaces attach
  So that I can drive one session from independent surface processes

  # use-case: UC-TRANSPORT-LIFECYCLE  (TC-M1-transport-020)
  Scenario: Health, attach, prompt round-trip, and shutdown
    Given a running orchestrator serving the transport
    Then the transport health endpoint responds ok
    When I attach a "files" surface with the launch CLI
    Then the attach succeeds
    When a prompt is submitted over the transport
    Then the response streams back over the event stream
    When the serving orchestrator is shut down
    Then the transport endpoint is unreachable
    And the attached surface is stopped

  # use-case: UC-TRANSPORT-DISABLED  (TC-M1-transport-021)
  Scenario: Disabling the transport keeps the in-process runtime
    Given a running orchestrator with the transport disabled
    Then the orchestrator publishes no transport endpoint
