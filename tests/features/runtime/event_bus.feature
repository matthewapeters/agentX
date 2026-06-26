# Source contracts:
#   - docs/architecture/channel_registry.md (channels + Processing State Contract)
#   - docs/architecture/runtime_contracts/event-envelope.schema.json
#   - docs/architecture/runtime_contracts/processing-state.schema.json
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-A3)
#
# Behavior: an in-process event bus fans events out to every subscriber in order
# (a slow subscriber must not block others), and a session-level processing-state
# feed exposes the canonical working/idle status.

@integration @arch:event-bus
Feature: In-process event bus and processing-state
  As the agentx runtime
  I want one canonical event/state layer
  So that every surface renders consistent output without private orchestration

  # use-case: UC-BUS-FANOUT
  Scenario: All subscribers receive every event in order
    Given a running event bus
    And two subscribers are attached
    When 5 ordered events are published
    Then each subscriber receives all 5 events in published order

  # use-case: UC-BUS-FANOUT
  # variant: slow-subscriber
  Scenario: A slow subscriber does not block others
    Given a running event bus
    And a fast subscriber and a slow subscriber are attached
    When 3 ordered events are published
    Then the fast subscriber receives all 3 events without waiting for the slow one

  # use-case: UC-BUS-ENVELOPE
  Scenario: Published events carry the required envelope fields
    Given a running event bus
    When an agent_content event is published for the active session
    Then the received event has a session id, event type, content type, and payload

  # use-case: UC-PROCESSING-STATE
  Scenario: Processing state transitions through a prompt cycle
    Given a processing-state publisher for the active session
    Then the current state is "idle" with phase "none"
    When the state is set to working in phase "respond"
    Then the current state is "working" with phase "respond"
    When the state is set to completed
    Then the current state is "completed" with phase "none"
