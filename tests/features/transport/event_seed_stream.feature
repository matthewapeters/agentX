# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md (Seed + Resume SS-1)
#   - docs/architecture/runtime_contracts/event-envelope.schema.json (ordinal)
#   - docs/build-plan/06_system_surfaces_backlog.md (SS-1)
#
# Behavior: the durable log is the seed; the live stream resumes after a cursor.
# The event bus stamps a per-session monotonic ordinal; seed + resume meet at a
# boundary ordinal so the handover has no gap and no duplicate.

@integration @arch:transport
Feature: Disk-seeded event stream with cursor resume
  As an attaching surface
  I want to seed from the durable log then resume the live stream by cursor
  So that I render the whole session exactly once, with no gap or duplicate

  # use-case: UC-SEED-ORDINAL  (TC-M2-seed-001)
  Scenario: The bus stamps a monotonic ordinal on each event
    Given a running transport server
    And 3 events are recorded
    When a client seeds the session events
    Then the seed has 3 events with ordinals "1, 2, 3"

  # use-case: UC-SEED-SNAPSHOT  (TC-M2-seed-002)
  Scenario: The seed returns the durable log with enabled state
    Given a running transport server
    And a recorded user_prompt event "hello"
    When a client seeds the session events
    Then the seed contains "hello"
    And the seed event "hello" is enabled

  # use-case: UC-SEED-RESUME  (TC-M2-seed-003)
  Scenario: Subscribing after the seed cursor delivers only newer events
    Given a running transport server
    And 2 events are recorded
    And a client seeds the session events
    When the client subscribes after the seed cursor
    And 2 more events are recorded
    Then the live stream delivers exactly 2 events
    And the live stream delivers no event at or before the cursor

  # use-case: UC-SEED-FULL  (TC-M2-seed-004)
  Scenario: Subscribing from zero yields the full stream
    Given a running transport server
    And 2 events are recorded
    When the client subscribes from the beginning
    And 1 more event is recorded
    Then the live stream delivers exactly 3 events
