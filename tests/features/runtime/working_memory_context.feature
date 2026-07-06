# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md (Working-Memory CRUD SS-6)
#   - internal/session/workmemory.go (Enabled facts fold into context)
#
# Behavior: confirms end-to-end that working-memory edits made through the
# orchestrator API actually change the next prompt's assembled context — enabled
# facts fold in, disabled facts do not. This is re-read fresh on every prompt.

@functional @arch:runtime
Feature: Working memory feeds the assembled context
  As a user curating working memory
  I want enabled facts to reach the model and disabled facts to be withheld
  So that toggling a fact actually changes what the agent sees

  # use-case: UC-WM-CONTEXT  (TC-M2-wm-003)
  Scenario: An enabled fact folds into the next prompt's context
    Given a started orchestrator with no bootstrap prompt and a capturing model that replies "ok"
    And the working memory has an enabled fact "favorite_color" valued "cerulean"
    When the user submits the prompt "hello"
    Then the model context contains "cerulean"

  # use-case: UC-WM-CONTEXT  (TC-M2-wm-004)
  Scenario: A disabled fact is withheld from the context
    Given a started orchestrator with no bootstrap prompt and a capturing model that replies "ok"
    And the working memory has an enabled fact "favorite_color" valued "cerulean"
    And the working memory fact "favorite_color" is disabled
    When the user submits the prompt "hello"
    Then the model context omits "cerulean"
