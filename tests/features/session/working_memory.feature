@unit
Feature: Working memory persistence and bootstrap
  Working memory is the durable, user-controlled set of session facts. The agent
  seeds bootstrap facts (userid, cwd) on startup when absent, but thereafter the
  user owns them: edits and disabled state survive across restarts, and enabled
  facts fold into the assembled context.
  (docs/ux/03_PANEL_DETAILS.md §PD-03, docs/ux/02_USER_FLOWS.md UF-09)

  Scenario: Bootstrap seeds userid and cwd as user-owned facts
    Given a fresh session with no working memory file
    When bootstrap facts are seeded
    Then the working memory has a "userid" fact owned by "user" and enabled
    And the working memory has a "cwd" fact owned by "user" and enabled

  Scenario: Bootstrap seeds the stable environment facts as user-owned
    Given a fresh session with no working memory file
    When bootstrap facts are seeded
    Then the working memory has a "project" fact owned by "user" and enabled
    And the working memory has a "home" fact owned by "user" and enabled
    And the working memory has a "os" fact owned by "user" and enabled
    And the working memory has a "arch" fact owned by "user" and enabled

  Scenario: Re-seeding never clobbers a user-edited fact
    Given a working memory with a disabled "cwd" fact valued "/pinned"
    When bootstrap facts are seeded
    Then the "cwd" fact is still valued "/pinned" and disabled

  Scenario: Bootstrap seeds live date and git-status pins, enabled by default
    Given a fresh session with no working memory file
    When bootstrap facts are seeded
    Then the working memory has a pin-owned "date" fact that is live and enabled
    And the working memory has a pin-owned "git_status" fact that is live and enabled

  Scenario: Bootstrap seeds a static tree pin, disabled by default
    Given a fresh session with no working memory file
    When bootstrap facts are seeded
    Then the working memory has a pin-owned "tree" fact that is static and disabled

  Scenario: A missing working memory file loads as empty
    Given a fresh session with no working memory file
    When the working memory is loaded
    Then the working memory has 0 facts

  Scenario: Saved working memory round-trips from disk
    Given a working memory with a disabled "cwd" fact valued "/pinned"
    When the working memory is saved and reloaded
    Then the "cwd" fact is still valued "/pinned" and disabled

  Scenario: Only enabled facts assemble into the context
    Given a working memory with an enabled "userid" fact valued "alice"
    And a disabled "cwd" fact valued "/tmp"
    When the working memory context message is built
    Then the context message includes "userid: alice"
    And the context message excludes "cwd"

  Scenario: No enabled facts produces no context message
    Given a working memory with a disabled "cwd" fact valued "/tmp"
    When the working memory context message is built
    Then no context message is produced
