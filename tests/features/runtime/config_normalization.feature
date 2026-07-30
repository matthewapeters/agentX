# Source contracts:
#   - internal/config.Config.Normalize
#
# Behavior: deprecated config keys are rewritten to their canonical names on
# save. The PD-CONFIG surface uses this to warn the user that `chat_backend` is
# deprecated and `provider` should be used instead (PD-CONFIG-AF-011).

@unit @arch:config-normalization
Feature: Deprecated key normalization
  As the config surface
  I want deprecated keys normalized to canonical names on save
  So that the user is warned about deprecated configuration

  # use-case: UC-NORMALIZE-CHAT-BACKEND
  Scenario: chat_backend is normalized to provider
    Given a config with chat_backend "ollama" and no provider
    When the config is normalized
    Then the provider is "ollama"
    And a normalization is recorded from "chat_backend" to "provider"

  # use-case: UC-NORMALIZE-NOOP
  Scenario: provider already set skips normalization
    Given a config with provider "ollama" and chat_backend "ollama"
    When the config is normalized
    Then no normalization is recorded

  # use-case: UC-NORMALIZE-BOTH-EMPTY
  Scenario: both keys empty produces no normalization
    Given a config with no provider and no chat_backend
    When the config is normalized
    Then no normalization is recorded

  # use-case: UC-NORMALIZE-DEPRECATED-FLAG
  Scenario: hasDeprecatedKeys reports chat_backend without provider
    Given a config with chat_backend "ollama" and no provider
    When the config is normalized
    Then the config has deprecated keys

  # use-case: UC-NORMALIZE-DEPRECATED-FLAG
  Scenario: hasDeprecatedKeys is false when provider is set
    Given a config with provider "ollama" and no chat_backend
    When the config is normalized
    Then the config has no deprecated keys
