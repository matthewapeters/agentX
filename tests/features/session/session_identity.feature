# Source contracts:
#   - docs/implementation/03_configuration_and_storage.md (Session Storage Root)
#   - docs/implementation/01_runtime_blueprint.md (Session identity policy)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-A2)
#
# Behavior: each session gets a canonical session_id and a human-readable
# session_name (adjective-noun) with deterministic collision suffixing, and its
# storage directory + metadata are initialized.

@integration @arch:session-identity
Feature: Session identity and storage initialization
  As the agentx runtime
  I want every session to have stable identity and storage
  So that routing, persistence, and replay have a canonical anchor

  # use-case: UC-SESSION-CREATE
  Scenario: New session gets id, name, and storage
    Given an empty session root
    When a session is created
    Then the session has a non-empty session_id
    And the session has a non-empty session_name
    And the session directory exists with an events folder
    And session.json records the same id and name

  # use-case: UC-SESSION-NAME-HELPER
  # source: internal/session/names.go — the `agentx session new-name` helper mints
  # names in the same vocabulary the runtime uses, so a launcher's session names
  # match the app's instead of an ad-hoc random string.
  Scenario: A default session name uses the adjective-noun style
    When a default session name is generated
    Then the resulting name has the adjective-noun form

  # use-case: UC-SESSION-NAME-HELPER
  # variant: a minted name avoids a session already on disk
  Scenario: A minted unique name avoids an existing session
    Given an empty session root
    And a session is created
    When a unique session name is minted
    Then the resulting name has the adjective-noun form
    And the minted name differs from the existing session's name

  # use-case: UC-SESSION-CREATE
  # variant: name-collision
  Scenario: Session name collisions get deterministic suffixes
    Given an empty session root
    And the name generator always yields "brave-otter"
    When a session is created
    And another session is created
    Then the first session_name is "brave-otter"
    And the second session_name is "brave-otter-2"
