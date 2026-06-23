# Feature: Persona Identification, Expert Selection, and Context Loading

Schema links:

- `docs/architecture/schemas/policy-decision.schema.json`
- `docs/architecture/design/06_persona_skill_and_tools_canon.md`

```gherkin
@persona @expert_selection @positive
Scenario: Persona identification resolves to correct expert when intent is unambiguous
  Given a triage classification returns intent=moderate complexity
  And orchestrator routing rules map that complexity to a specific expert
  When persona identification is executed
  Then the selected persona ID has a corresponding file in .agentx/agents/
  And persona input contract is validated successfully
  And selected expert matches expected roster mapping

@persona @expert_selection @negative
Scenario: Expert selection fails explicitly when persona file does not exist
  Given a routing decision selects persona ID "hypothetical_expert"
  And no file exists at .agentx/agents/hypothetical_expert.agent.md
  When expert invocation is attempted
  Then decision is deny with reason code persona_not_found
  And routing falls back to clarification/escalation path
  And no silent runtime invention of expert behavior occurs

@persona @context_loading @positive
Scenario: Context loader includes only enabled user/assistant messages
  Given session context contains mixed enabled/disabled messages
  And context includes system and internal role messages
  When context is prepared for expert invocation
  Then only enabled user and assistant messages are included
  And system/internal roles are explicitly excluded
  And working-memory facts are included if enabled

@persona @context_loading @positive
Scenario: Context truncation by token budget is deterministic
  Given fixed seed and token budget of N tokens
  And message history exceeds N tokens
  When context is truncated deterministically
  Then truncation point is the same for identical message sequences
  And truncation index and excluded message IDs are recorded
  And context fingerprint is computed for replay validation

@persona @context_loading @positive
Scenario: Context fingerprint enables replay determinism
  Given context fingerprint recorded during original execution
  And same enabled messages in same order
  And same token budget and seed
  When context is prepared again
  Then new context fingerprint matches original
  And determinism check passes for replay parity gate

@persona @policy @negative
Scenario: Policy boundary is enforced before expert context is prepared
  Given a destructive action proposed for expert invocation
  And confirmation artifact is missing
  When policy boundary checks execute
  Then decision is deny with reason code destructive_confirmation_missing
  And expert is never invoked
  And no context is prepared (fail-closed)

@persona @skill_routing @positive
Scenario: Skill ID resolves against .agentx/skills/ registry before dispatch
  Given expert selects a skill to accomplish a task
  When skill routing validates the skill
  Then skill ID maps to a file in .agentx/skills/
  And required tools are cross-checked against .agentx/tools/
  And all required tools are available/enabled
  And skill is marked executable

@persona @skill_routing @negative
Scenario: Missing skill ID fails with explicit reason code
  Given expert attempts to invoke skill_id "nonexistent_skill"
  And no file exists at .agentx/skills/nonexistent_skill.skill.md
  When skill validation executes
  Then decision is deny with reason code skill_not_found
  And expert receives explicit reason in return packet
  And no silent fallback or error-suppression occurs

@persona @tools_reference @positive
Scenario: Tool reference documentation is used for capability discovery
  Given a skill author reviewing .agentx/tools/ directory
  When they read tool documentation (for example file-system.tools.md)
  Then they understand available tools, inputs, outputs, and constraints
  And they can correctly author required_tools list in skill definition

@persona @tools_reference @positive
Scenario: Tool availability is checked before skill dispatch
  Given skill declares required_tools = [tool_a, tool_b]
  When skill validation checks tool availability
  Then each tool is validated against .agentx/tools/ documentation
  And missing/disabled tools result in skill rejection with reason code
  And expert is informed of missing tool and suggested fallback

@persona @instructions @positive
Scenario: Applicable instructions are identified and loaded before expert invocation
  Given persona "go-staff-programmer" is selected for a Go implementation task
  And .agentx/instructions/ contains expert-contract.instructions.md and go-staff-programmer.instructions.md
  When instructions are loaded
  Then both instruction files are found and loaded
  And instructions are loaded in priority order (persona-specific first)
  And no silent omission of applicable instructions occurs

@persona @instructions @positive
Scenario: Instructions are prepended to expert context as immutable Layer 0
  Given instructions have been loaded for the selected persona
  And persona definition exists
  And session context contains enabled messages
  When context is assembled for expert invocation
  Then Layer 0 (Instructions) is prepended first
  Then Layer 1 (Persona definition) follows
  Then Layer 2 (Session context) follows
  And instructions are marked immutable during expert execution
  And instructions cannot be modified or overridden by expert

@persona @instructions @positive
Scenario: Instruction content hash is included in context fingerprint
  Given instructions are loaded for original execution
  And content hash is computed from instruction files
  When context fingerprint is calculated
  Then fingerprint includes (instruction_hash + all context layers)
  And fingerprinting enables deterministic replay validation

@persona @instructions @negative
Scenario: Missing required instruction fails explicitly
  Given persona "subutai" is selected for orchestration task
  And orchestrator determines that subutai-orchestrator.instructions.md is required
  And file does not exist at .agentx/instructions/subutai-orchestrator.instructions.md
  When instruction loading is attempted
  Then decision is deny with reason code instruction_not_found
  And expert is never invoked
  And orchestrator receives explicit failure for fallback/retry logic

@persona @instructions @positive
Scenario: Instructions enable deterministic expert behavior
  Given instructions + persona + context loaded in original execution with seed S
  And same instructions + persona + context loaded in replay with seed S
  When expert decision is computed for both executions
  Then decisions are deterministically identical
  And decision reasoning references active instructions
  And context fingerprints match (instruction hash + context hash)

@persona @traceability @positive
Scenario: Expert return packet includes traceability fields for replay
  Given expert has completed a task
  When return packet is assembled
  Then packet includes persona_id, selected_skill_id (if any), context_fingerprint
  And decision_reason_code, policy_decision, and audit_id are present
  And instruction_hash is included (for replay/audit parity)
  And all fields enable deterministic replay and compliance validation

@persona @traceability @positive
Scenario: Disabled expert results in deterministic fallback with reason code
  Given expert persona is disabled or unavailable
  When orchestrator attempts expert selection
  Then routing explicitly selects fallback expert or clarification path
  And reason code (expert_unavailable) is recorded with disabled expert ID
  And no silent behavior change or implicit expert substitution occurs
```
