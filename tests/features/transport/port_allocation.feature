# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md
#       (Port Allocation; Port Allocation & Endpoint Publication TRN-4)
#   - docs/build-plan/05_transport_backlog.md (TRN-4)
#
# Behavior: the orchestrator binds one transport endpoint from a configured port
# range (lowest free port, conflict-aware fallback, exhaustion error) and
# publishes the resolved endpoint to session metadata.

@integration @arch:transport
Feature: Transport port allocation and endpoint publication
  As the orchestrator
  I want to bind a free loopback port and publish my endpoint
  So that external surfaces can discover where to attach

  # use-case: UC-PORT-LOWEST  (TC-M1-transport-016)
  Scenario: Allocates the lowest free port in the range
    Given a discovered free port
    When the allocator binds a range of size 1 at that port
    Then a listener is bound on that port

  # use-case: UC-PORT-FALLBACK  (TC-M1-transport-017)
  Scenario: Falls back past an occupied port
    Given a discovered free port
    And that port is occupied
    When the allocator binds a range from that port spanning 5
    Then a listener is bound on a different port in the range

  # use-case: UC-PORT-EXHAUSTED  (TC-M1-transport-018)
  Scenario: Fails when the range is exhausted
    Given a discovered free port
    And that port is occupied
    When the allocator binds a range of size 1 at that port
    Then allocation fails with a range-exhausted error

  # use-case: UC-PORT-PUBLISH  (TC-M1-transport-019)
  Scenario: Publishes the transport endpoint to session metadata
    Given a session store with a created session
    When the endpoint "http://127.0.0.1:8420" is published
    Then reading the session transport metadata returns endpoint "http://127.0.0.1:8420"

  # use-case: UC-TOKEN-PUBLISH  (TC-M2-ss5-001)
  # source: docs/implementation/02_surface_orchestration_http.md (Flagless launch SS-5)
  Scenario: Publishes the attach token at 0600 for same-machine discovery
    Given a session store with a created session
    And the endpoint "http://127.0.0.1:8420" is published
    When the attach token "tok-abc" is published
    Then reading the session attach token returns "tok-abc"
    And the attach token file mode is 0600

  # use-case: UC-TOKEN-DISCOVER  (TC-M2-ss5-002)
  Scenario: Discovery returns sessions that published an endpoint and a token
    Given a session store with a created session
    And the endpoint "http://127.0.0.1:8420" is published
    And the attach token "tok-abc" is published
    Then discovering transports finds endpoint "http://127.0.0.1:8420" with token "tok-abc"

  # use-case: UC-TOKEN-DISCOVER  (variant: no token → not discoverable)
  Scenario: A session that published no token is not discoverable
    Given a session store with a created session
    And the endpoint "http://127.0.0.1:8420" is published
    Then discovering transports finds nothing
