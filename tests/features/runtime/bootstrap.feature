# Source contracts:
#   - docs/implementation/01_runtime_blueprint.md (runtime bootstrap)
#   - docs/implementation/08_go_module_layout.md (Godog structure)
#   - docs/implementation/07_test_and_documentation_contract.md (tag scheme)
#
# This is the harness smoke feature. It proves the Godog runner, the
# @unit|@integration|@functional|@e2e tag scheme, and the @arch:<id> linkage
# all work end-to-end. Real runtime behavior features replace/extend this in M1.

@arch:runtime-bootstrap
Feature: agentx runtime harness smoke
  As an AgentX engineer
  I want a working Godog harness
  So that behavior-driven scenarios can be authored before implementation

  # use-case: UC-HARNESS-SMOKE
  @unit
  Scenario: Harness executes a unit-tagged scenario
    Given the Godog harness is wired
    When a unit scenario runs
    Then the harness reports success

  # use-case: UC-HARNESS-SMOKE
  # variant: functional-tag
  @functional
  Scenario: Harness executes a functional-tagged scenario
    Given the Godog harness is wired
    When a functional scenario runs
    Then the harness reports success
