# Source contracts:
#   - docs/implementation/08_go_module_layout.md (internal/llm package family)
#   - Design under discussion: parallel model-invocation infrastructure
#     ("fan-out / fan-in"), the enabling capability for self-consistency voting,
#     tool-selection-by-commonality, parallel async jobs, and parallel
#     confirmations. This feature specs the PRIMITIVE only — a bounded-concurrency
#     pool that runs N invocations and folds their results through a pluggable
#     aggregator. Specific classifiers are built ON this, in later features.
#
# Behavior: `llm.Pool.Run` fans a batch of invocations out to the model backend
# under a concurrency cap, streams results back as they land (with error and
# timeout isolation), and folds them through an Aggregator. Each invocation
# carries an OUTPUT CONTRACT (structure / length / cardinality bounds); results
# that violate the contract are quarantined OUT of the fold rather than poisoning
# it — this is what makes fan-in results comparable and bounds the blast radius.
#
# The model is stubbed throughout: these scenarios pin the pool's orchestration
# and fold semantics, not Ollama's real concurrency (measured separately).

@llm @fanout @arch:invocation-pool
Feature: Parallel model-invocation pool (fan-out / fan-in)
  As the agentX orchestration server
  I want to run many bounded, contract-checked model invocations concurrently and
  fold their results through a pluggable aggregator
  So that accuracy-first behaviors (voting, commonality, confirmations) compose on
  one primitive without each reinventing concurrency, isolation, or aggregation

  # ---------------------------------------------------------------------------
  # Core fan-out / fan-in
  # ---------------------------------------------------------------------------

  # use-case: UC-FANOUT-COLLECT  (TC-FAN-001)
  @unit
  Scenario: A fan-out batch returns every result, tagged to its invocation
    Given a fan-out pool backed by a stub model
    And a batch of 4 invocations each tagged with its purpose
    When the pool runs the batch with a collect-all aggregator
    Then the fan-out returns 4 results
    And each fan-out result carries its invocation tag

  # use-case: UC-FANOUT-VARIANT  (TC-FAN-002)
  # Voting needs diversity: the pool must honor each invocation's own params so
  # temperature/seed/prompt variants actually differ.
  @unit
  Scenario: Each invocation is dispatched with its own parameters
    Given a fan-out pool backed by a stub model
    And 3 invocations with temperatures 0.0, 0.5, and 1.0
    When the pool runs the batch with a collect-all aggregator
    Then each invocation is dispatched with its own temperature

  # ---------------------------------------------------------------------------
  # Bounded concurrency, isolation, cancellation
  # ---------------------------------------------------------------------------

  # use-case: UC-FANOUT-BOUND  (TC-FAN-003)
  @integration
  Scenario: In-flight invocations never exceed the concurrency cap
    Given a fan-out pool with a concurrency cap of 2
    And 6 invocations that each block until released
    When the pool runs the batch
    Then no more than 2 invocations are in flight at any moment

  # use-case: UC-FANOUT-ISOLATE  (TC-FAN-004)
  @integration
  Scenario: One failing invocation does not fail the batch
    Given a fan-out pool backed by a stub model
    And a batch of 3 invocations where the second fails with "model error"
    When the pool runs the batch with a collect-all aggregator
    Then 2 fan-out results succeed
    And 1 fan-out result carries the error "model error"
    And the fan-out batch itself does not error

  # use-case: UC-FANOUT-TIMEOUT  (TC-FAN-005)
  @integration
  Scenario: A slow invocation times out without blocking its siblings
    Given a fan-out pool backed by a stub model
    And an invocation with a 50ms timeout whose model answers after 200ms
    And a sibling invocation that answers promptly
    When the pool runs the batch
    Then the slow fan-out result carries a timeout error
    And the sibling fan-out result succeeds

  # use-case: UC-FANOUT-CANCEL  (TC-FAN-006)
  @integration
  Scenario: Cancelling the caller context cancels in-flight invocations
    Given a fan-out pool backed by a stub model with invocations in flight
    When the caller cancels the fan-out context
    Then the in-flight invocations are cancelled
    And the pool returns without waiting for the full timeout

  # ---------------------------------------------------------------------------
  # Aggregation: vote, quorum early-exit, abstention
  # ---------------------------------------------------------------------------

  # use-case: UC-FANOUT-VOTE  (TC-FAN-007)
  @unit
  Scenario: A majority-vote fold picks the modal verdict with a confidence
    Given a majority-vote aggregator
    And fan-out invocations returning verdicts "write,write,write,chat,write"
    When the pool folds the results
    Then the fold decision is "write"
    And the fold confidence is 0.8

  # use-case: UC-FANOUT-QUORUM  (TC-FAN-008)
  # Early-exit reconciles accuracy-first with not-gratuitously-slow.
  @integration
  Scenario: A quorum decision cancels the stragglers once it is reached
    Given a majority-vote aggregator with a quorum of 3
    And 5 invocations where 3 quickly agree on "write" and 2 are slow
    When the pool folds the results
    Then the fold decision is "write"
    And the 2 slow invocations are cancelled

  # use-case: UC-FANOUT-ABSTAIN  (TC-FAN-009)
  # Knowing when it isn't sure: scatter must abstain, not force a pick.
  @unit
  Scenario: A scattered vote abstains rather than guessing
    Given a majority-vote aggregator with an abstain threshold of 0.6
    And fan-out invocations returning verdicts "write,chat,query,write"
    When the pool folds the results
    Then the fold abstains
    And the abstention reason is "no quorum"

  # ---------------------------------------------------------------------------
  # Output contracts — the requirement that makes fan-in comparable and bounded
  # ---------------------------------------------------------------------------

  # use-case: UC-FANOUT-CONTRACT  (TC-FAN-010)
  @unit
  Scenario: A result missing the required structure is quarantined
    Given an output contract requiring a "verdict" field
    And 4 fan-out results where one omits the "verdict" field
    When the pool validates the results against the contract
    Then 3 fan-out results conform
    And 1 fan-out result is quarantined as "malformed"
    And only conforming results are counted in the fold

  # use-case: UC-FANOUT-CONTRACT  (TC-FAN-011)
  @unit
  Scenario: A result over the word bound is quarantined
    Given an output contract bounding the answer to 250 words
    And a fan-out result of 300 words
    When the pool validates the result against the contract
    Then the fan-out result is quarantined as "over length"

  # use-case: UC-FANOUT-CONTRACT  (TC-FAN-012)
  # "Decompose into no more than 5 milestones" — cardinality is a first-class bound.
  @unit
  Scenario: A decomposition over the milestone cap is quarantined
    Given an output contract allowing at most 5 milestones
    And a fan-out result decomposing the goal into 6 milestones
    When the pool validates the result against the contract
    Then the fan-out result is quarantined as "too many milestones"

  # use-case: UC-FANOUT-CONTRACT  (TC-FAN-013)
  @unit
  Scenario: Too few conforming results forces the fold to abstain
    Given a majority-vote aggregator with a quorum of 3
    And 5 fan-out results where only 2 conform to the output contract
    When the pool folds the results
    Then the fold abstains
    And the abstention reason is "insufficient conforming results"

  # ---------------------------------------------------------------------------
  # Budget guard + provenance
  # ---------------------------------------------------------------------------

  # use-case: UC-FANOUT-BUDGET  (TC-FAN-014)
  # Bounded fan-out width caps cost AND the blast radius of a runaway batch.
  @unit
  Scenario: A batch wider than the budget is rejected
    Given a fan-out pool with a maximum width of 8
    When a batch of 12 invocations is submitted
    Then the fan-out is rejected with reason "width exceeds budget"

  # use-case: UC-FANOUT-DEFAULTS  (TC-FAN-016)
  # Bound width to the server's parallel-slot count; wider only queues (see the
  # fan-out concurrency spike — throughput is flat past the slots).
  @unit
  Scenario: The pool sizes itself to the Ollama slot count
    Given the Ollama parallel-slot count is "6"
    When a pool is built with server defaults
    Then the pool default concurrency is 6
    And the pool default width budget is 12

  # use-case: UC-FANOUT-DEFAULTS  (TC-FAN-017)
  @unit
  Scenario: The pool falls back to a default slot count when none is advertised
    Given the Ollama parallel-slot count is unset
    When a pool is built with server defaults
    Then the pool default concurrency is 4
    And the pool default width budget is 8

  # use-case: UC-FANOUT-PROV  (TC-FAN-015)
  # Every vote and the aggregate are answerable from the event log later.
  @integration
  Scenario: Each invocation and the aggregate decision are recorded
    Given a fan-out pool with provenance recording enabled
    And a batch of 3 invocations folded by majority vote
    When the pool folds the results
    Then each invocation is recorded as an event
    And the aggregate decision is recorded with its vote spread
