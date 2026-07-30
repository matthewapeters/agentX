# Source contracts:
#   - internal/llm/provider/validation
#
# Behavior: the type-appropriate validators accept or reject user input from the
# config TUI editor according to each key's type rules (PD-CONFIG-AF-006).

@unit @arch:config-validation
Feature: Type-appropriate config value validation
  As the config surface
  I want each field type validated against its rules before acceptance
  So that invalid input is rejected with an actionable inline error

  # use-case: UC-TYPE-VALIDATE-INT
  Scenario: Valid integers pass
    Given a value "42" validated as an integer with range [0, 100]
    When the validation runs
    Then the value is valid

  # use-case: UC-TYPE-VALIDATE-INT
  Scenario: Non-integer input is rejected
    Given a value "abc" validated as an integer with range [0, 100]
    When the validation runs
    Then the value is rejected with "must be an integer"

  # use-case: UC-TYPE-VALIDATE-INT
  Scenario: Empty integer input is rejected
    Given a value "" validated as an integer with range [0, 100]
    When the validation runs
    Then the value is rejected with "is required"

  # use-case: UC-TYPE-VALIDATE-INT
  Scenario: Out-of-range integer is rejected
    Given a value "200" validated as an integer with range [0, 100]
    When the validation runs
    Then the value is rejected with "must be ≤ 100"

  # use-case: UC-TYPE-VALIDATE-BOOL
  Scenario: Valid bool input passes
    Given a value "true" validated as a boolean
    When the validation runs
    Then the value is valid

  # use-case: UC-TYPE-VALIDATE-BOOL
  Scenario: Non-bool input is rejected
    Given a value "yes" validated as a boolean
    When the validation runs
    Then the value is rejected with "must be true or false"

  # use-case: UC-TYPE-VALIDATE-ENUM
  Scenario: Valid enum selection passes
    Given a value "ollama" validated as an enum with ["ollama", "llamacpp"]
    When the validation runs
    Then the value is valid

  # use-case: UC-TYPE-VALIDATE-ENUM
  Scenario: Invalid enum selection is rejected
    Given a value "gemini" validated as an enum with ["ollama", "llamacpp"]
    When the validation runs
    Then the value is rejected with "must be one of:"

  # use-case: UC-TYPE-VALIDATE-ENUM
  Scenario: Case-insensitive enum passes
    Given a value "OLLAMA" validated as an enum with ["ollama", "llamacpp"]
    When the validation runs
    Then the value is valid

  # use-case: UC-TYPE-VALIDATE-COLOR
  Scenario: Named color passes
    Given a value "cyan" validated as a color
    When the validation runs
    Then the value is valid

  # use-case: UC-TYPE-VALIDATE-COLOR
  Scenario: Hex color passes
    Given a value "#00afaf" validated as a color
    When the validation runs
    Then the value is valid

  # use-case: UC-TYPE-VALIDATE-COLOR
  Scenario: ANSI 256 index passes
    Given a value "240" validated as a color
    When the validation runs
    Then the value is valid

  # use-case: UC-TYPE-VALIDATE-COLOR
  Scenario: Out-of-range ANSI index is rejected
    Given a value "300" validated as a color
    When the validation runs
    Then the value is rejected with "must be 0–255"

  # use-case: UC-TYPE-VALIDATE-HOST
  Scenario: Valid host:port passes
    Given a value "localhost:11434" validated as a host
    When the validation runs
    Then the value is valid

  # use-case: UC-TYPE-VALIDATE-HOST
  Scenario: Empty host is rejected
    Given a value "" validated as a host
    When the validation runs
    Then the value is rejected with "is required"

  # use-case: UC-TYPE-VALIDATE-MODEL
  Scenario: Valid model name passes
    Given a value "phi4:latest" validated as a model name
    When the validation runs
    Then the value is valid

  # use-case: UC-TYPE-VALIDATE-MODEL
  Scenario: Empty model name is rejected
    Given a value "" validated as a model name
    When the validation runs
    Then the value is rejected with "is required"

  # use-case: UC-TYPE-VALIDATE-NONE
  Scenario: Empty non-empty string is rejected
    Given a value "" validated as a non-empty string
    When the validation runs
    Then the value is rejected with "is required"

  # use-case: UC-TYPE-VALIDATE-NONE
  Scenario: Non-empty string passes
    Given a value "some-value" validated as a non-empty string
    When the validation runs
    Then the value is valid
