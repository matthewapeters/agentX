# Source contract: docs/implementation/03_configuration_and_storage.md (Phase 1b)
#
# Behavior: config writes are transactional — a semaphore (config.lock in
# ~/.cache/agentx/) prevents concurrent writes, a timestamped temp file in the
# cache dir is renamed atomically onto the deployment path, and stale temps
# left by a crashed writer are purged at orchestrator startup.

@integration @arch:config-write
Feature: Transactional config write infrastructure
  As the AgentX runtime
  I want config writes to be atomic and race-free
  So that a crash or concurrent writer never corrupts the deployment config

  # use-case: UC-CONFIG-ATOMIC-WRITE
  Scenario: WriteConfig produces a valid deployment config
    Given a writable deployment config path
    And a config with ollama_model "llama3"
    When the config is written atomically
    Then the deployment config file is created
    And the deployment config has ollama_model "llama3"

  # use-case: UC-CONFIG-ATOMIC-WRITE
  # variant: temp-file-staging
  Scenario: The write stages through a temp file in the cache dir
    Given a writable deployment config path
    And a config with ollama_model "mistral"
    And the cache dir exists
    When the config is written atomically
    Then a temp file is created in the cache dir
    And the temp file is removed after the rename completes
    And the deployment config has ollama_model "mistral"

  # use-case: UC-CONFIG-ATOMIC-WRITE
  # variant: cross-device-fallback
  Scenario: A cross-device temp+rename falls back to copy-then-rename
    Given a writable deployment config path on a different filesystem than the cache dir
    And a config with ollama_model "phi4"
    When the config is written atomically
    Then the deployment config file is created
    And the deployment config has ollama_model "phi4"

  # use-case: UC-CONFIG-SEMAPHORE
  Scenario: Concurrent writes serialize through the semaphore
    Given a writable deployment config path
    And two configs with ollama_model "llama3" and "mistral"
    When two writes are attempted concurrently
    Then the deployment config file contains one of the two writes
    And no partial or corrupted config is written

  # use-case: UC-CONFIG-SEMAPHORE
  # variant: lock-file-created
  Scenario: The semaphore lock file is created at the cache path
    Given the cache dir exists
    And a writable deployment config path
    And a config with ollama_model "llama3"
    When the config is written atomically
    Then the lock file exists at the cache path

  # use-case: UC-CONFIG-CLEANUP
  Scenario: Stale temp files are cleaned up at startup
    Given stale temp files exist in the cache dir
    When stale temps are cleaned up
    Then no stale temp files remain in the cache dir

  # use-case: UC-CONFIG-CLEANUP
  # variant: no-stale-temps
  Scenario: Cleanup is a no-op when no stale temps exist
    Given no stale temp files exist in the cache dir
    When stale temps are cleaned up
    Then no stale temp files remain in the cache dir

  # use-case: UC-CONFIG-CLEANUP
  # variant: lock-not-stale
  Scenario: The lock file is not removed by cleanup
    Given the lock file exists in the cache dir
    And no stale temp files exist in the cache dir
    When stale temps are cleaned up
    Then the lock file still exists in the cache dir
