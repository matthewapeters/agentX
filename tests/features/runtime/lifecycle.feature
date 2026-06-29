# Source contracts:
#   - docs/implementation/01_runtime_blueprint.md (Runtime Lifecycle)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-A5)
#
# Behavior: the orchestrator boots idle with a fresh session and shuts down
# gracefully, flushing persisted events and a final processing-state snapshot.

@integration @arch:runtime-lifecycle
Feature: Orchestrator lifecycle
  As the agentx runtime
  I want deterministic startup and graceful shutdown
  So that sessions begin idle and end with a flushed, recoverable timeline

  # use-case: UC-RUNTIME-LIFECYCLE
  Scenario: Orchestrator boots idle with a session
    Given orchestrator settings with a temp session root
    When the orchestrator starts
    Then the processing state is idle
    And the orchestrator has an active session
    And the orchestrator is accepting prompts

  # use-case: UC-RUNTIME-LIFECYCLE
  # variant: graceful-shutdown
  Scenario: Graceful shutdown flushes the timeline and stops accepting
    Given a started orchestrator
    When a user_prompt event is published and the orchestrator shuts down
    Then the orchestrator is not accepting prompts
    And the session timeline includes the user_prompt event
    And the session timeline includes a processing_state snapshot
