# Source contracts:
#   - docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md (Phase 2)
#   - docs/architecture/behavior/adr/0008_task_branch_context.feature.md
#   - internal/runtime/branch (Branch: fork, snapshot, plan, seal, merge)
#   - internal/session/readgrant.go (out-of-cwd read grants: once / session)
#
# Behavior: a branch is an isolated planning context forked from a parent. It inherits a
# read-only snapshot of enabled working memory, keeps its events and WM writes to itself,
# is plan-only (read-restricted catalog; out-of-cwd reads need prior approval), and returns
# only a Result that merges into the parent task DAG. No LLM — the container and its
# boundary only (Phase 2 of ADR 0008); the decomposer that runs inside arrives in Phase 4.

@runtime @task-branch @arch:adr-0008
Feature: a branch context isolates planning and returns only a plan
  As the decomposer's sandbox
  I want an isolated, plan-only fork of the parent session
  So that investigation informs a plan without leaking into or mutating the parent

  # use-case: UC-RTBRANCH-001
  @unit
  Scenario: A branch inherits a read-only snapshot of enabled working memory
    Given a parent working memory with enabled facts "cwd" and "project"
    And a disabled parent fact "secret"
    When a branch is forked from the parent
    Then the branch sees fact "cwd"
    And the branch does not see fact "secret"

  # use-case: UC-RTBRANCH-002
  @unit
  Scenario: Branch working-memory changes do not reach the parent
    Given a branch forked from the parent
    When the branch records a local fact "language" = "go"
    Then the parent working memory has no "language" fact

  # use-case: UC-RTBRANCH-003
  @unit
  Scenario: Branch events are isolated from the parent conversation
    Given a branch forked from the parent
    When the branch emits a planning event
    Then the parent conversation receives no branch event
    And the branch log contains the planning event

  # use-case: UC-RTBRANCH-004
  @unit
  Scenario: A branch catalog offers read tools but no mutating tools
    Given the branch read-restricted catalog
    Then it contains the tool "read_file"
    And it does not contain the tool "write_file"

  # use-case: UC-RTBRANCH-004c
  @unit
  Scenario: An out-of-cwd read is not allowed without a prior grant
    Given a working directory "/work" with no granted read paths
    When a read of "/data/logs/app.log" is checked
    Then the read is not allowed without approval

  # use-case: UC-RTBRANCH-004d
  @unit
  Scenario: A session-scoped grant is remembered in working memory
    Given a working directory "/work"
    When an out-of-cwd read of "/data/logs" is approved for the session
    Then working memory records a permitted read path "/data/logs"
    And a read of "/data/logs/app.log" is allowed without approval

  # use-case: UC-RTBRANCH-004e
  @unit
  Scenario: A one-time grant is not remembered
    Given a working directory "/work"
    When an out-of-cwd read of "/data/logs" is approved once
    Then working memory records no permitted read path
    And a read of "/data/logs/app.log" is not allowed without approval

  # use-case: UC-RTBRANCH-005
  @unit
  Scenario: A branch returns only child records and a synthesis
    Given a branch forked from the parent
    And the branch has added child records "s1" and "s2"
    And the branch synthesis is "two independent steps"
    When the branch is sealed
    Then the result records are exactly "s1, s2"
    And the result synthesis is "two independent steps"

  # use-case: UC-RTBRANCH-006
  @unit
  Scenario: Merging a branch result adds its records to the parent DAG
    Given a parent task DAG containing node "root"
    And a branch result with records "s1" and "s2" each depending on "root"
    When the result is merged into the parent
    Then the parent DAG has 3 nodes
    And the parent edge set is exactly "root->s1, root->s2"
    And the merge returns synthesis "two independent steps"

  # use-case: UC-RTBRANCH-007
  @unit
  Scenario: A discarded branch leaves the parent unchanged
    Given a parent task DAG containing node "root"
    And a parent working memory with 2 facts
    And a branch forked from the parent that added records and local facts
    When the branch is discarded without sealing
    Then the parent DAG has 1 node
    And the parent working memory has 2 facts

  # use-case: UC-RTBRANCH-008
  @unit
  Scenario: Branch depth increments and is bounded
    Given a branch at depth one below the max task depth
    When it forks a child branch
    Then the child branch depth equals the max task depth
    And a further fork is refused with a max-depth error

  # use-case: UC-RTBRANCH-009
  @unit
  Scenario: A branch carries parent provenance
    Given a parent session "p1"
    When a branch is forked from the parent
    Then the branch parent id is "p1"
    And the branch depth is 1
