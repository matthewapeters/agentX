# Feature: ADR 0003 Dispatcher Priority Heap and Fairness

Schema links:

- `docs/architecture/schemas/trace-event.schema.json`
- `docs/architecture/schemas/execution-outcome.schema.json`
- `docs/architecture/schemas/quality-gate-report.schema.json`

```gherkin
@adr0003 @dispatcher @fairness
Scenario: ADR-0003-AF-001 Interactive channel gets lower dispatch delay
  Given mixed interactive, orchestration, and maintenance jobs
  And Gate B thresholds include max_wait_ms_p95 and min_service_share
  When scheduler selects jobs by heap score
  Then Gate B observed.max_wait_ms_p95.interactive is lower than observed.max_wait_ms_p95.maintenance

@adr0003 @dispatcher @fairness
Scenario: ADR-0003-AF-002 Non-interactive channels are not starved
  Given sustained interactive load with ready jobs in all channels
  When scheduler runs for the configured fairness service window
  Then Gate B observed.starvation.max_consecutive_misses is less than or equal to thresholds.starvation_max_consecutive_misses
  And Gate B observed.service_share.orchestration is greater than or equal to thresholds.min_service_share.orchestration
  And Gate B observed.service_share.maintenance is greater than or equal to thresholds.min_service_share.maintenance

@adr0003 @dispatcher @determinism
Scenario: ADR-0003-AF-003 Aging increases effective priority
  Given two equal-priority jobs with different wait ages
  When scores are computed
  Then older job receives higher total score due to age boost

@adr0003 @dispatcher @determinism
Scenario: ADR-0003-AF-004 Deadline proximity influences selection
  Given two equal-priority jobs with different deadlines
  When scheduler chooses next dispatch
  Then earlier-deadline job is selected first

@adr0003 @dispatcher @retry
Scenario: ADR-0003-AF-005 Retriable failure re-enqueues with backoff
  Given a retriable execution outcome
  When dispatcher re-enqueues the job
  Then attempt count increments
  And delay is within configured backoff bounds and deterministic for fixed seed plus attempt

@adr0003 @dispatcher @negative
Scenario: ADR-0003-AF-006 Budget guard blocks over-budget dispatch
  Given channel concurrency or token budget is exhausted
  When scheduler attempts dispatch
  Then dispatch is deferred when replenishment is possible in-window
  And dispatch is denied when policy or budget constraints are non-recoverable in-window
  And decision is trace-visible in reason_codes

@adr0003 @dispatcher @determinism
Scenario: ADR-0003-AF-007 Fixed-seed scheduler sequence is reproducible
  Given fixed seed and synthetic clock input
  When scheduler selection runs twice over equivalent queue snapshots
  Then selected job sequence is identical including jitter-derived tie breaks
```
