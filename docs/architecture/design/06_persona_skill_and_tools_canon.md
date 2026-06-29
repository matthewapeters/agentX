# Design 06: Persona, Skill, and Tools Canonical Locations and Loading Pipeline

Last updated: 2026-06-15
ADR linkage: 0004 (policy/security boundaries)

## Goal

Establish authoritative locations and loading pipeline for runtime personas (expert subagents), skill specifications, and tool definitions to prevent hallucinated or inconsistent routing/context behavior.

## Canonical Directory Structure

All runtime configuration and metadata for expertise and capabilities lives under `.agentx/` at the project root:

```
.agentx/
├── agents/                    ← Persona definitions (expert subagent specs)
├── instructions/              ← Behavioral guidance (routing-aware context layer)
├── skills/                    ← Skill specifications (task/capability bundles)
├── tools/                     ← Tool reference documentation (available executables)
├── agentx-instructions.md     ← Project-specific behavioral guidance (existing)
├── agentx.toml                ← Runtime configuration (existing)
└── system-panel-tab.txt       ← System layout config (existing)
```

## Directory Contracts

### `.agentx/agents/` — Persona Home (Authoritative)

**Purpose:** Single source of truth for expert subagent persona definitions.

**Contents:**

- One YAML or Markdown file per expert persona (for example `data-architect.agent.md`, `go-staff-programmer.agent.md`).
- Each file contains:
  - `name` — human-readable expert name
  - `description` — expertise domain and primary use case
  - `focus_areas` — specific responsibilities (list)
  - `input_contract` — required fields in incoming task packet
  - `return_format` — expected output structure/format
  - `constraints` — operational limits (e.g., no code generation, read-only mode)

**Authority:**

- This directory is the source of truth for what experts exist in this project.
- Changes to persona roster must update files here; do not invent personas at runtime.
- Persona IDs must be stable across sessions and match the file stem (for example `data-architect` -> `data-architect.agent.md`).

**Loading Contract:**

1. Runtime persona identification happens via routing rules in `../../../.github/instructions/subutai-orchestrator.instructions.md` (orchestration persona selection policy).
2. Selected persona ID is validated against `.agentx/agents/` file existence before invocation.
3. If persona does not exist, routing fails with deterministic reason code and fallback path.

**Scope:** Orchestration/subagent personas only. End-user chat-style personas live in `system_prompts/` (existing).

### `.agentx/instructions/` — Behavioral Guidance for Personas (Routing-Aware Context Layer)

**Purpose:** Reusable behavioral instructions that apply to specific personas or use-case scenarios. This is the **only** mechanism for communicating routing decisions, expert contracts, and constraints to subagents.

**Contents:**

- One Markdown file per instruction category (for example `expert-contract.instructions.md`, `orchestrator-triage.instructions.md`).
- Each file contains behavioral guidance, decision rules, or process guidance.
- Files may apply to:
  - Single personas (e.g., `subutai-orchestrator.instructions.md` applies only to the `subutai` persona)
  - Multiple personas by pattern (e.g., `expert-contract.instructions.md` applies to all subagents)
  - Specific use-case scenarios (e.g., `issue-fix-close.instructions.md` may apply to multiple experts working on issue fixes)

**Authority:**

- Instructions are **not invented at runtime**; all guidance comes from files in this directory.
- Instructions are selected during context preparation based on:
  - Selected persona ID (exact match or pattern)
  - Task classification/intent
  - Scenario type or domain
- Missing instruction files result in explicit `instruction_not_found` reason code for required instructions.
- Instructions are loaded **before** expert invocation to ensure consistent behavioral baseline.

**Loading Contract:**

1. When a persona is selected, the orchestrator determines which instructions apply (via persona ID mapping or scenario routing).
2. Applicable instruction files are loaded from `.agentx/instructions/`.
3. Loaded instructions are **prepended to expert context** as system-level guidance **before** any other context.
4. If required instructions are missing, expert invocation fails with explicit reason code.
5. Instructions are **immutable during execution** (no dynamic modification by expert).
6. Instruction content is hashed and included in context fingerprint (enables replay parity).

**Scope:** Behavioral guidance only. Implementation details and code live elsewhere.

**Key Insight:** LLM awareness of instruction availability is **critical**—when an expert is invoked, they must be told which instructions are active in their context, so they can reference and apply them correctly. Instructions form the immutable baseline for all expert decision-making.

### `.agentx/skills/` — Skill Specification Home (Authoritative)

**Purpose:** Centralized registry of bundled task/capability specifications that can be executed.

**Contents:**

- One YAML or Markdown file per skill (for example `code-review.skill.md`, `architecture-design.skill.md`).
- Each file contains:
  - `name` — skill display name
  - `description` — what the skill accomplishes
  - `required_tools` — list of tool IDs this skill needs (e.g., `["read_file", "write_file"]`)
  - `required_context_fields` — context fields that must be present (e.g., `["codebase_root", "active_files"]`)
  - `policy_profile` — security profile name (tied to ADR 0004 hard boundary)
  - `success_criteria` — acceptance conditions

**Authority:**

- Skills are registered here; they are not invented at runtime.
- Routing decisions can reference skills by ID; IDs are stable file stems.
- Skills can have prerequisite/depends-on relationships documented here.

**Loading Contract:**

1. When an orchestration task invokes a skill, the skill ID is validated against `.agentx/skills/` existence.
2. If skill does not exist, the task fails with reason code and returns to orchestrator for fallback/clarification.
3. Required tools are validated against `.agentx/tools/` registry before skill dispatch.

**Scope:** Capability/task bundles only. Individual tool implementations live elsewhere (Go tools, Python modules, etc.).

### `.agentx/tools/` — Tool Reference Documentation (Authoritative)

**Purpose:** Curated reference documentation of available tools that skills can use.

**Contents:**

- One Markdown or YAML file per tool or tool category (for example `file-system.tools.md`, `bigquery.tools.md`).
- Each file describes:
  - Tool name / ID
  - Purpose and use case
  - Input contract (required/optional parameters)
  - Output contract (return shape)
  - Security/policy constraints (ADR 0004 enforcement points)
  - Example usage (if applicable)
  - Known limitations or gotchas

**Authority:**

- This is the **reference guide** for adding new skills and extending capabilities.
- It is not the source of truth for tool *implementation*; implementations live in `internal/tools/` (Go) or external services.
- This directory helps skill authors understand what tools exist and how to use them.

**Loading Contract:**

1. When a skill declares `required_tools`, each tool ID is validated against `.agentx/tools/` documentation existence.
2. Tool unavailability (not installed, disabled by policy) results in skill rejection with reason code.
3. This directory is used for discovery and documentation, not runtime dispatch (dispatch comes from actual tool implementations).

**Scope:** Reference and discovery only. Actual tool execution is governed by runtime tool registry.

## Loading and Context Pipeline

### 1. Persona Identification → Expert Selection

**Input:** User request → classification intent

**Flow:**

1. Orchestrator reads [`.github/instructions/subutai-orchestrator.instructions.md`](../../../.github/instructions/subutai-orchestrator.instructions.md) routing rules.
2. Request is triaged as trivial/moderate/complex per [`.github/instructions/orchestrator-triage.instructions.md`](../../../.github/instructions/orchestrator-triage.instructions.md).
3. Triage determines which expert persona(s) could handle the request.
4. Expert ID is resolved against `.agentx/agents/` directory.
5. If expert file exists, persona is loaded; if not, fallback/clarification path is invoked.

**Contract Enforcement Points:**

- Persona ID must have a corresponding `.agentx/agents/<id>.agent.md` file.
- Persona input contract is validated before task packet is assembled.
- Policy boundary (ADR 0004) is checked before expert context is prepared.

### 2. Instruction and Context Preparation for Expert Invocation

**Input:** Selected persona + original request + current session context

**Flow:**

1. **Load applicable instructions:**
   - Determine which instructions apply based on persona ID and task classification.
   - Resolve instruction file paths against `.agentx/instructions/` directory.
   - Load instruction files in priority order (e.g., persona-specific first, then general).
   - If any **required** instruction is missing, fail with `instruction_not_found` reason code and fallback.
   - Compute hash of loaded instruction content for fingerprinting.

2. **Prepare execution environment:**
   - Load persona from `.agentx/agents/`.
   - Read persona input contract to determine required context fields.
   - Extract those fields from current session context (messages, files, working memory).
   - Load any persona-specific overrides or constraint files.

3. **Assemble context layers (in priority order):**
   - **Layer 0 (Immutable System):** Loaded instructions from `.agentx/instructions/` (prepended first).
   - **Layer 1 (Persona):** Persona definition and focus areas from `.agentx/agents/`.
   - **Layer 2 (Session):** Enabled user/assistant messages (disabled/system roles explicitly excluded).
   - **Layer 3 (Working Memory):** Enabled working-memory facts.
   - **Layer 4 (Constrained):** Token-budget-constrained context.

4. **Apply deterministic processing:**
   - Disabled/soft-deleted messages are **explicitly excluded** (per ADR 0004 policy boundary).
   - System/internal roles are **explicitly excluded**.
   - Token budget is applied deterministically (same seed → same truncation).
   - Context fingerprint is computed from instruction hash + all context layers and recorded for replay/audit.

5. **Enforce policy boundary:**
   - ADR 0004 redaction rules are applied before any context is passed to expert.
   - Policy decision is recorded with reason codes.

**Contract Enforcement Points:**

- All applicable instructions are loaded and prepended to context **before** expert receives any other context.
- Required instructions are validated; missing files result in explicit reason code.
- Instructions form the **immutable baseline** for expert decision-making.
- Only enabled user/assistant history is included (system/internal roles excluded).
- Context fields match persona input contract; missing required fields fail with reason code.
- Token budget is applied deterministically (same seed → same truncation).
- Redaction (ADR 0004 sensitive-output handling) is applied before expert invocation.
- Context fingerprint includes hash of instruction content + all context layers (enables replay parity).

- **Instruction Immutability Enforcement:** Instructions are stored in context as read-only reference. Attempts to modify Layer 0 instruction text result in `layer_0_immutable` reason code and are rejected before transmission to expert. Original instruction hash is computed at load time and is **invariant** for the duration of expert invocation (captured in return packet for replay/audit).

### 3. Skill Routing (if applicable)

**Input:** Expert + prepared context + task

**Flow:**

1. If task can be decomposed into skills, expert evaluates available skills.
2. Each skill ID is resolved against `.agentx/skills/` directory.
3. Required tools for each skill are validated against `.agentx/tools/` directory.
4. Policy profile (ADR 0004) is checked before any skill is marked executable.
5. Expert selects appropriate skill(s) or falls back to direct response.

**Contract Enforcement Points:**

- Skill IDs are resolved deterministically; missing skills result in explicit rejection, not silent fallback.
- Tool availability is checked before skill dispatch.
- Tool policy constraints are enforced by policy boundary middleware before any tool invocation.

### 4. Expert Invocation and Return

**Input:** Expert persona + prepared context + selected skill/task

**Output:** Task packet return per [`.github/instructions/expert-contract.instructions.md`](../../../.github/instructions/expert-contract.instructions.md)

**Contract Enforcement Points:**

- Return packet must include traceability fields for replay/audit (selected skill, context fingerprint, decision rationale hash).
- Policy boundary decisions (allow/deny/constrained) are recorded with reason codes.
- All tool invocations are logged with audit IDs for forensics.

## Determinism and Replay

To support deterministic replay and audit parity (ADR 0005 Quality Gate D):

1. **Persona routing** is deterministic:
   - Same request + same triage rules → same persona selection.
   - Recorded: persona ID, triage category, routing reason code, seed/fingerprint.

2. **Context loading** is deterministic:
   - Same enabled messages + same token budget → same truncated context.
   - Recorded: context fingerprint (hash of field values), truncation index, excluded message IDs.

3. **Skill selection** is deterministic:
   - Same context + same persona + same skills available → same skill choice (if any).
   - Recorded: selected skill ID, precedence/tie-breaker applied, decision reason.

4. **Return compliance** is auditable:
   - Each expert return packet includes required traceability fields.
   - Replay validation checks that re-execution with same context produces equivalent decision.

## Implementation Guidance for Go Expert

### Loading Files

When implementing persona/skill/tools/instructions loading in Go:

1. **Instruction Loading (Load First):**

   ```go
   // Determine applicable instructions based on persona + task classification
   instructionPaths := determineApplicableInstructions(personaID, taskClassification)
   
   var instructions []string
   for _, instrPath := range instructionPaths {
       fullPath := filepath.Join(".agentx", "instructions", instrPath + ".instructions.md")
       content, err := os.ReadFile(fullPath)
       if err != nil {
           return nil, fmt.Errorf("required instruction %q not found: %w", instrPath, err)
       }
       instructions = append(instructions, string(content))
   }
   
   // Compute instruction hash for fingerprinting
   instrHash := sha256.Sum256([]byte(strings.Join(instructions, "\n---\n")))
   ```

**Instruction Loading Priority (Deterministic):**

When multiple instruction files apply to the same persona/task, load in this order (deterministic, all-or-nothing):

| Priority | Pattern | Example | Load Order |
| --- | --- | --- | --- |
| 1 (Persona-specific exact match) | `<persona_id>*.instructions.md` | `go-staff-programmer.instructions.md` | Lexical/alphabetical |
| 2 (Task-specific pattern) | `<task_classification>*.instructions.md` | `issue-fix.instructions.md`, `code-review.instructions.md` | Lexical/alphabetical |
| 3 (Scenario-specific) | `<scenario>*.instructions.md` | (if applicable) | Lexical/alphabetical |
| 4 (General) | All remaining `.instructions.md` files that haven't been loaded | `expert-contract.instructions.md`, `orchestration-artifacts.instructions.md` | Lexical/alphabetical |

**Within each priority level, load files in alphabetical order by filename.** This ensures determinism across multiple invocations.

**Example resolution** for persona=`go-staff-programmer`, task=`code-review`:

```
1. Load: go-staff-programmer.instructions.md
2. Load: code-review.instructions.md  
3. Load: expert-contract.instructions.md, orchestration-artifacts.instructions.md (alphabetical)
```

```go
2. **Persona Loading:**
// Pseudocode for deterministic resolution
func determineApplicableInstructions(personaID, taskClassification string) []string {
      var instructions []string
    
      // Priority 1: Persona-specific exact match
      personaSpecific := glob(".agentx/instructions/" + personaID + "*.instructions.md")
      sort.Strings(personaSpecific)
      instructions = append(instructions, personaSpecific...)
    
      // Priority 2: Task-specific pattern
      taskSpecific := glob(".agentx/instructions/" + taskClassification + "*.instructions.md")
      sort.Strings(taskSpecific)
      instructions = append(instructions, taskSpecific...)
    
      // Priority 4: General (all remaining)
      allInstructions := glob(".agentx/instructions/*.instructions.md")
      sort.Strings(allInstructions)
      for _, instr := range allInstructions {
            if !contains(instructions, instr) {
                  instructions = append(instructions, instr)
            }
      }
    
      return instructions
}
```

**Explicit Fingerprint Composition (Deterministic across languages):**

All context fingerprints must be computed using this exact algorithm to ensure replay parity across Go, Python, and other implementations:

```
Fingerprint(instructions, enabledMessages, workingMemory, seed) =
   SHA256(
      InstructionHash +
      MessageHash +
      MemoryHash
   )

Where:

   InstructionHash = SHA256(
      Concatenate(instruction_contents, separator="\n---\n")
   )
  
   MessageHash = SHA256(
      Concatenate(
         [SHA256(msg.id + "|" + msg.role + "|" + msg.content) for msg in enabledMessages sorted by msg.id],
         separator="|"
      )
   )
  
   MemoryHash = SHA256(
      Concatenate(
         [key + "|" + value for key, value in workingMemory.items() if value.enabled],
         sorted by key,
         separator="|"
      )
   )
```

**Go implementation example:**

```go
func ComputeContextFingerprint(
      instructions []string,
      enabledMessages []Message,
      workingMemory map[string]WorkingMemoryFact,
) [32]byte {
    
      // Step 1: Instruction hash (join with "\n---\n" separator)
      instrContent := strings.Join(instructions, "\n---\n")
      instrHash := sha256.Sum256([]byte(instrContent))
    
      // Step 2: Message hashes (sort by ID, then hash in that canonical order)
      sort.Slice(enabledMessages, func(i, j int) bool {
         return enabledMessages[i].ID < enabledMessages[j].ID
      })
      var msgHashes []string
      for _, msg := range enabledMessages {
            if !msg.Enabled {
                  continue
            }
         msgText := msg.ID + "|" + msg.Role + "|" + msg.Content
            h := sha256.Sum256([]byte(msgText))
            msgHashes = append(msgHashes, hex.EncodeToString(h[:]))
      }
      msgContent := strings.Join(msgHashes, "|")
      msgHash := sha256.Sum256([]byte(msgContent))
    
      // Step 3: Memory hashes (sort by key, then join)
      var memKeys []string
      for key, val := range workingMemory {
            if !val.Enabled {
                  continue
            }
            memKeys = append(memKeys, key)
      }
      sort.Strings(memKeys)
      var memPairs []string
      for _, key := range memKeys {
            memPairs = append(memPairs, key + "|" + workingMemory[key].Value)
      }
      memContent := strings.Join(memPairs, "|")
      memHash := sha256.Sum256([]byte(memContent))
    
      // Step 4: Combined fingerprint
      combined := hex.EncodeToString(instrHash[:]) + "|" +
                        hex.EncodeToString(msgHash[:]) + "|" +
                        hex.EncodeToString(memHash[:])
      return sha256.Sum256([]byte(combined))
}
```

### Fingerprint Conformance Fixture

Use this fixture to validate cross-language parity:

- instructions:
  - `alpha`
  - `beta`
- enabledMessages (sorted by `id` before hashing):
  - `{id: "m1", role: "user", content: "hello"}`
  - `{id: "m2", role: "assistant", content: "world"}`
- workingMemory (enabled=true):
  - `cwd=/tmp`
  - `project=agentx`

Expected final fingerprint (SHA-256 hex):

- `e724d1bd7bdcd84fa8ad14dad86f1b1c3ba447839aea8f471e879bfcbd60c363`

   ```go
   personaFile := filepath.Join(".agentx", "agents", personaID + ".agent.md")
   if _, err := os.Stat(personaFile); err != nil {
       return nil, fmt.Errorf("persona %q not found: %w", personaID, err)
   }
   // Load and parse persona definition...
   ```

1. **Skill Validation:**

   ```go
   skillFile := filepath.Join(".agentx", "skills", skillID + ".skill.md")
   if _, err := os.Stat(skillFile); err != nil {
       return nil, fmt.Errorf("skill %q not found: %w", skillID, err)
   }
   // Load and validate skill requirements...
   ```

2. **Tool Reference (Discovery):**

   ```go
   toolRefFile := filepath.Join(".agentx", "tools", toolCategory + ".tools.md")
   // Use for documentation/validation only; do not use for tool dispatch.
   // Tool dispatch comes from actual tool implementations.
   ```

### Context Preparation

When preparing context for expert invocation:

1. **Assemble context layers in order:**
   - Layer 0: Loaded instructions (already computed hash above)
   - Layer 1: Persona definition
   - Layer 2: Enabled messages only

2. **Collect enabled messages only:**

   ```go
   var enabledMessages []Message
   for _, msg := range context.AllMessages() {
       if !msg.Enabled {
           continue  // Skip disabled/soft-deleted
       }
       if msg.Role == "system" || msg.Role == "internal" {
           continue  // Skip hidden roles
       }
       enabledMessages = append(enabledMessages, msg)
   }
   ```

3. **Apply token budget deterministically:**
   - Use fixed seed for truncation logic.
   - Record truncation point and excluded message IDs.
   - Compute fingerprint hash of (instruction hash + context fields) for replay validation.

4. **Enforce policy boundary before expert dispatch:**
   - Call ADR 0004 `BoundaryMiddleware.CheckIngress()` before payload is sent.
   - Record decision (allow/deny/constrained) with reason codes.
   - Include instruction hash in fingerprint for replay/audit parity.

5. **Prepare final payload:**
   - Concatenate instructions + persona + truncated messages.
   - Include context fingerprint in return packet for traceability.

## Conformance Checklist

- [ ] `.agentx/agents/`, `.agentx/instructions/`, `.agentx/skills/`, and `.agentx/tools/` directories exist and are documented.
- [ ] Every persona used by orchestration has a corresponding `.agentx/agents/<id>.agent.md` file.
- [ ] Persona loading fails with explicit reason code if file is missing (never silent fallback).
- [ ] Every instruction file in `.agentx/instructions/` is used by at least one persona or scenario (no orphaned files).
- [ ] Instructions are loaded **first** in context assembly (Layer 0, before any other context).
- [ ] Required instructions for a persona fail with explicit `instruction_not_found` reason code if missing.
- [ ] Instruction content hash is computed and included in context fingerprint.
- [ ] Every skill referenced in task routing has a corresponding `.agentx/skills/<id>.skill.md` file.
- [ ] Skill validation checks tool availability against `.agentx/tools/` before dispatch.
- [ ] Context loading is deterministic: same enabled messages + instructions + seed → same truncated context.
- [ ] Policy boundary (ADR 0004) is checked before expert context is prepared.
- [ ] Return packets include traceability fields for replay/audit (persona ID, instruction hash, skill ID, context fingerprint, decision reason).
- [ ] Disabled messages and system roles are explicitly excluded from expert context.
- [ ] Token budget application is recorded and replayable.
- [ ] Instructions are immutable during expert execution (no dynamic modification).
