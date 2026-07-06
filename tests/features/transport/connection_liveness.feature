# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md (Connection liveness SS-4)
#   - docs/ux/06_OUTPUT_WIDGET.md (Launch-info widget — status emojis)
#
# Behavior: a surface counts as connected only while it holds an open event stream.
# GET /events?surface_id=<id> marks the surface live on open and dead on close (incl.
# crash, since the connection drops), and the registry reports the connected kinds.

@integration @arch:transport
Feature: Surface connection liveness over the transport
  As the orchestrator
  I want connection status driven by the live event stream
  So that presence reflects reality and a crashed surface is not shown attached

  # use-case: UC-TRN-LIVENESS  (TC-M2-liveness-002)
  # a document-based surface (e.g. working memory) holds a stream only for presence
  Scenario: A presence-only stream marks a poll-based surface connected
    Given a running transport server
    And a surface "wm-1" is registered on the transport
    When surface "wm-1" holds a presence stream
    Then the transport reports connected kinds "files"
    When surface "wm-1" drops its presence stream
    Then the transport connected kinds become ""

  # use-case: UC-TRN-LIVENESS  (TC-M2-liveness-001)
  Scenario: An open event stream marks a surface connected; closing clears it
    Given a running transport server
    And a surface "files-1" is registered on the transport
    Then the transport reports connected kinds ""
    When surface "files-1" connects its events stream
    Then the transport reports connected kinds "files"
    When surface "files-1" disconnects its events stream
    Then the transport connected kinds become ""
