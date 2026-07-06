# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md (Surface Host SS-2)
#   - docs/build-plan/06_system_surfaces_backlog.md (SS-2)
#
# Behavior: the shared surface host drives a SurfaceModel through the attach
# lifecycle — apply the seed before live events, append live events, resize, quit
# (with shutdown), and quit when the stream closes; non-quit keys go to the surface.

@functional @arch:surface-client
Feature: Surface-client host
  As a surface author
  I want a host that drives the attach lifecycle
  So that each surface only implements its projection and rendering

  # use-case: UC-HOST-SEED  (TC-M2-host-001)
  Scenario: The host applies the seed then live events
    Given a surface host seeded with 2 events
    When the host initializes
    Then the surface has applied 2 events
    When a live event arrives
    Then the surface has applied 3 events

  # use-case: UC-HOST-RESIZE  (TC-M2-host-002)
  Scenario: The host sizes the surface inside its title strip
    Given a surface host seeded with 0 events
    When the host receives a window size 100 by 40
    Then the surface size is 100 by 39

  # use-case: UC-HOST-QUIT  (TC-M2-host-003)
  Scenario: A quit key shuts the surface down and quits
    Given a surface host seeded with 0 events
    When the host receives the "q" key
    Then the surface shutdown was requested
    And the host signals quit

  # use-case: UC-HOST-STREAMCLOSE  (TC-M2-host-004)
  Scenario: A closed event stream quits the host
    Given a surface host seeded with 1 events
    When the host initializes
    And the event stream closes
    Then the host signals quit

  # use-case: UC-HOST-KEY  (TC-M2-host-005)
  Scenario: A non-quit key is forwarded to the surface
    Given a surface host seeded with 0 events
    When the host receives the "j" key
    Then the surface received key "j"
