# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md
#       (HTTP API Baseline v1: Write endpoints; Write Server TRN-3)
#   - docs/architecture/runtime_contracts/surface-registration.schema.json
#   - docs/build-plan/05_transport_backlog.md (TRN-3)
#
# Behavior: the loopback HTTP transport accepts surface-to-orchestrator writes.
# /surface/register authorizes via the body token; other writes require a bearer
# attach token. Prompts drive an async cycle whose events flow over SSE.

@functional @arch:transport
Feature: HTTP transport write endpoints
  As an external surface process
  I want to register, submit prompts, approve tools, and shut down over HTTP
  So that I can drive the orchestrator the same way the in-process chat does

  # use-case: UC-HTTP-REGISTER  (TC-M1-transport-007)
  Scenario: Register over HTTP with a valid token
    Given a running transport server
    When a client POSTs to "/surface/register" with a valid registration
    Then the response status is 201
    And the response JSON field "lifecycle_state" is "ready"

  # use-case: UC-HTTP-REGISTER  (TC-M1-transport-008)
  # variant: bad token rejected
  Scenario: Register over HTTP with a wrong token is rejected
    Given a running transport server
    When a client POSTs to "/surface/register" with token "not-the-token"
    Then the response status is 401
    And the response JSON field "category" is "auth"

  # use-case: UC-HTTP-REGISTER  (TC-M1-transport-009)
  # variant: conflicting id rejected
  Scenario: Register over HTTP with a conflicting id is rejected
    Given a running transport server
    And a surface "files-1" is registered on the transport
    When a client POSTs to "/surface/register" for id "files-1" with a valid token
    Then the response status is 409
    And the response JSON field "category" is "conflict"

  # use-case: UC-HTTP-PROMPT  (TC-M1-transport-010)
  Scenario: Prompt over HTTP drives a cycle whose events arrive over SSE
    Given a running transport server
    And the client is authorized with the attach token
    And an open events stream
    When a client POSTs "/prompt" with text "hello there"
    Then the response status is 202
    And the events stream delivers "hello there"

  # use-case: UC-HTTP-PROMPT  (TC-M1-transport-011)
  # variant: unauthorized write rejected
  Scenario: Prompt without a bearer token is rejected
    Given a running transport server
    When a client POSTs "/prompt" with text "hello"
    Then the response status is 401
    And the response JSON field "category" is "auth"

  # use-case: UC-HTTP-PROMPT  (TC-M1-transport-012)
  # variant: not accepting
  Scenario: Prompt is rejected when the orchestrator is not accepting
    Given a running transport server that is not accepting prompts
    And the client is authorized with the attach token
    When a client POSTs "/prompt" with text "hello"
    Then the response status is 409
    And the response JSON field "category" is "conflict"

  # use-case: UC-HTTP-APPROVAL  (TC-M1-transport-013)
  Scenario: Approval over HTTP resolves the orchestrator gate
    Given a running transport server
    And the client is authorized with the attach token
    When a client POSTs "/tool/approval" with decision "approve_session"
    Then the response status is 200
    And the orchestrator received decision "approve_session"

  # use-case: UC-HTTP-SHUTDOWN  (TC-M1-transport-014)
  Scenario: Shut a surface down over HTTP
    Given a running transport server
    And a surface "files-1" is registered on the transport
    And the client is authorized with the attach token
    When a client POSTs to "/surface/files-1/shutdown"
    Then the response status is 200
    And the surface "files-1" on the transport has lifecycle "stopped"

  # use-case: UC-HTTP-MODELSWITCH  (TC-M1-transport-015)
  Scenario: Model switch is not implemented in v1
    Given a running transport server
    And the client is authorized with the attach token
    When a client POSTs "/model/switch"
    Then the response status is 501
