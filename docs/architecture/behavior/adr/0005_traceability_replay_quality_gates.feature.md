# Feature: ADR 0005 Traceability, Replay, and Quality Gates

Schema links:

- `docs/architecture/schemas/trace-event.schema.json`
- `docs/architecture/schemas/replay-bundle.schema.json`
- `docs/architecture/schemas/quality-gate-report.schema.json`

```gherkin
@adr0005 @trace @gateE
Scenario: ADR-0005-AF-001 Trace completeness includes required linkage fields
  Given an orchestration run has completed
  When trace completeness check executes
  Then gate E thresholds include required_linkage_fields and max_missing_linkage
  And gate E observed includes linkage_coverage and missing_linkage_count
  And gate E observed.missing_linkage_count is less than or equal to thresholds.max_missing_linkage

@adr0005 @replay @gateD
Scenario: ADR-0005-AF-002 Replay parity succeeds on same seed and config fingerprint
  Given a replay bundle with fixed seed and config fingerprint
  When offline replay is executed
  Then node ordering, retry outcomes, and final state match expected baseline

@adr0005 @replay @gateD @negative
Scenario: ADR-0005-AF-003 Replay mismatch is reported with divergence evidence
  Given replay input differs from baseline configuration
  When replay parity check runs
  Then gate fails
  And gate D observed includes parity_rate and divergence_count
  And gate D observed.divergence_count is greater than thresholds.divergence_count_max
  And gate D evidence_ref points to replay_bundle.divergence_report_ref

@adr0005 @quality_gate @gateB
Scenario: ADR-0005-AF-004 Fairness gate reflects starvation checks
  Given dispatcher fairness corpus is executed
  When gate B report is produced
  Then gate B observed includes max_wait_ms_p95, service_share, starvation, and retry
  And gate B observed.starvation.max_consecutive_misses is less than or equal to thresholds.starvation_max_consecutive_misses
  And gate B observed.retry.jitter_ratio is less than or equal to thresholds.retry_jitter_ratio_max

@adr0005 @quality_gate @gateA
Scenario: ADR-0005-AF-005 Fast-path SLO gate evaluates latency and success
  Given fast-path workload scenario corpus
  When gate A report is produced
  Then gate A thresholds include latency_p95_ms and success_rate_min
  And gate A observed includes latency_p95_ms and success_rate
  And gate A observed.latency_p95_ms is less than or equal to thresholds.latency_p95_ms
  And gate A observed.success_rate is greater than or equal to thresholds.success_rate_min

@adr0005 @quality_gate @gateC
Scenario: ADR-0005-AF-006 Policy gate validates allow deny and constrained matrix
  Given policy decision matrix scenarios
  When gate C report is produced
  Then gate C thresholds include deny_rate_max and constraint_coverage_min
  And gate C observed includes deny_rate and constraint_coverage
  And gate C observed.deny_rate is less than or equal to thresholds.deny_rate_max
  And gate C observed.constraint_coverage is greater than or equal to thresholds.constraint_coverage_min
```
