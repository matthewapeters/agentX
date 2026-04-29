# Classification Logging Analysis & Implementation

**Date:** 2026-03-21  
**Session Analyzed:** `/Projects/agentX/sessions/mpeters/session_2026-03-21_23-59-06`  
**Issue:** Prompt classifier failed to choose correct path despite WM entry `use_tools: true`; system hallucinated non-existent tools  
**Status:** ✅ **IMPLEMENTED** — All P1-P5 improvements completed

---

## Implementation Summary

**Version:** 0.9.0 (minor version bump for new feature)

All proposed logging improvements have been implemented:

### ✅ Completed Tasks

1. **Logging Configuration** — [src/agentix/logging_config.py](../src/agentix/logging_config.py)
   - Created JSONFormatter for structured logging
   - Configured rotating file handler for `logs/classification.jsonl`
   - Integrated with AgentX and Agentix startup

2. **Classification Logging** — [src/agentix/bridge/classify_prompt.py](../src/agentix/bridge/classify_prompt.py)
   - Added structured logging at all decision points
   - Logs: request start, payload, raw LLM result, final decision, duration
   - Enhanced exception handling with KeyError and JSONDecodeError differentiation

3. **Working Memory Integration**
   - Added `working_memory` parameter to classification pipeline
   - Implemented `_format_working_memory_for_classification()` helper
   - Updated bridge, adapter, and session to pass WM through
   - WM facts now visible to classification LLM

4. **Enhanced Exception Handling** — [src/agentx/integration/agentix_bridge_adapter.py](../src/agentx/integration/agentix_bridge_adapter.py)
   - Differentiated exception types (JSON parse, enum error, generic)
   - Full traceback logging with context
   - Diagnostic hints in fallback reasoning_summary

5. **Session Log Integration** — [src/agentx/session.py](../src/agentx/session.py)
   - Added `_log_classification()` method
   - Classification results written to `session.log`
   - Shows intent, reasoning, WM context, and routing path

6. **System Prompt Updates** — [system_prompts/prompt_classification.md](../system_prompts/prompt_classification.md)
   - Added "WORKING MEMORY CONTEXT" section
   - Explicit rules for using WM facts in classification
   - Examples for context-aware routing

7. **CLI Testing Tool** — [src/agentix/main.py](../src/agentix/main.py)
   - New `--classify` command for isolated testing
   - Usage: `python -m agentix --classify --user "prompt text"`
   - Outputs classification decision in JSON format

### 📂 Files Modified

```
src/agentix/
├── logging_config.py           [NEW] Structured logging configuration
├── main.py                     [MODIFIED] Added classify command
├── agentix_config.py           [MODIFIED] Added classify CLI option
└── bridge/
    └── classify_prompt.py      [MODIFIED] Added logging + WM support

src/agentx/
├── main.py                     [MODIFIED] Configure Agentix logging
├── session.py                  [MODIFIED] Pass WM, log classifications
└── integration/
    └── agentix_bridge_adapter.py [MODIFIED] Enhanced exception handling

system_prompts/
└── prompt_classification.md    [MODIFIED] Added WM context rules

pyproject.toml                  [MODIFIED] Version 0.8.4 → 0.9.0
```

### 🧪 Testing the Changes

**Test classification for the regression scenario:**

```bash
python -m agentix --classify --user "Create a markdown file ./agentx/agentx-application-flow.md which contains machine-readable markdown that explains the major workflow of the agentX application, including how agentix works. To complete this, review the application code. This is a complex task that will require multi-phase steps. You should explore and make a todo list to track your progress."
```

**Expected output:**

```json
{
  "intent": "complex_action",
  "next_step": "invoke_planner",
  "needs_clarification": false,
  "missing_fields": [],
  "reasoning_summary": "Multi-step: review code (search/read) + create file (write)"
}
```

**Check structured logs:**

```bash
tail -f sessions/_logs/classification.jsonl | jq .
```

### 🔍 What Changed for the Failure Scenario

**Before (Session 2026-03-21_23-59-06):**

```
🤔 intent: conversation
   reasoning: Classification unavailable
💡 path: respond_directly
```

- Exception swallowed silently
- No WM context visible to classifier
- Fallback to conversation → respond_directly
- LLM hallucinated non-existent tools

**After (With Implementation):**

```
🤔 intent: complex_action
   reasoning: Multi-step: review code + create file + exploration
   🏛️  WM context: use_tools=true, cwd=/Projects/agentX, project=agentx
💡 path: invoke_planner
```

- Classification succeeds with WM context
- Correct routing to planner path
- Tool schemas available to LLM
- Full diagnostic logging if failure occurs

---

## Executive Summary

The prompt classification system failed to correctly route a multi-step user request that explicitly required tool usage. The system:

1. **Misclassified** a complex action requiring code exploration & file creation as "conversation"
2. **Failed silently** with reasoning "Classification unavailable" (exception fallback)
3. **Did not see Working Memory context** — the `use_tools: true` fact was never injected into the classification prompt
4. **Hallucinated non-existent tools** (`list_directory`, `container.exec`) instead of using registered tools
5. **Provided no debugging trail** to diagnose the failure

---

## Session Evidence

### User Request (Message 2)

```
Create a markdown file `./agentx/agentx-application-flow.md` which contains 
machine-readable markdown that explains the major workflow of the agentX 
application, including how agentix works. To complete this, review the 
application code. This is a complex task that will require multi-phase steps. 
You should explore and make a todo list to track your progress.
```

**Expected Classification:**

- **intent:** `complex_action` (multi-step: code review + file creation + exploration)
- **next_step:** `invoke_planner` (explicitly stated as multi-phase)

**Actual Classification:**

```json
{
  "intent": "conversation",
  "reasoning_summary": "Classification unavailable",
  "needs_clarification": false,
  "missing_fields": [],
  "next_step": "respond_directly"
}
```

**Working Memory Context (Not Visible to Classifier):**

```json
{
  "user:use_tools": {
    "value": true,
    "enabled": true,
    "timestamp": "2026-03-21T17:03:42.341071"
  }
}
```

**Observed Behavior:**

- System proceeded with `respond_directly` path
- LLM attempted to call `list_directory` tool (doesn't exist)
- LLM attempted to call `container.exec` tool (doesn't exist)
- Available tools were `read_file`, `write_file`, `search_files` (per error message)

---

## Root Cause Analysis

### 1. Classification Exception Swallowed Silently

**Location:** `src/agentx/integration/agentix_bridge_adapter.py:112-145`

```python
def classify_prompt_sync(
    self, 
    prompt: str, 
    context: Context
) -> Optional[PromptClassificationResponse]:
    if not self.agentix_config.classify_prompts:
        return None
    
    try:
        return self.bridge.classify_prompt(prompt, context)
    except Exception as e:
        print(f"Classification error: {e}")  # ⚠️ ONLY LOGGING
        return PromptClassificationResponse(
            intent=Intent.conversation,
            needs_clarification=False,
            missing_fields=[],
            reasoning_summary="Classification unavailable",  # ⚠️ GENERIC FALLBACK
            next_step=NextStep.respond_directly,
        )
```

**Issues:**

- Exception printed to stderr with no context (traceback, prompt text, config)
- Fallback response loses all diagnostic information
- No correlation ID to trace through logs
- No indication to user that classification failed

---

### 2. Working Memory Not Injected into Classification Context

**Classification Prompt Assembly:** `src/agentix/bridge/classify_prompt.py:21-99`

The classification uses `PROMPT_CLASSIFICATION` system prompt + user prompt + limited history. **Working Memory facts are never serialized into the classification payload.**

**Evidence from system prompt** (`system_prompts/prompt_classification.md`):

- System prompt mentions Working Memory operations (remember/forget/list)
- But no actual WM facts are injected into the prompt for context-aware classification
- The LLM classifier cannot see `use_tools: true` or `cwd: /Projects/agentX`

**Expected Behavior:**
Classification prompt should include a `<working_memory>` block:

```
<working_memory>
👤 UserName: mpeters
👤 cwd: /Projects/agentX
👤 project: agentx
👤 use_tools: true
👤 agentx-instructions: # AgentX Instructions ... [truncated]
</working_memory>
```

---

### 3. No Logging of Classification Request/Response

**API Client:** `src/agentix/api_client.py:111-136`

```python
def query_classification(args: AgentixConfig, payload: QueryPayload | dict) -> dict:
    backend = getattr(args, "classification_backend", "ollama")
    if backend == "torch":
        # ... torch path with custom logging ...
        return classify_intent_with_torch(...)
    
    return query_api(args, payload)  # ⚠️ NO CLASSIFICATION-SPECIFIC LOGGING
```

**Logging only happens if `args.debug=True`:**

- Payload logged in `query_api()` line 75
- Raw response logged in line 92
- But these are generic API logs, not classification-specific

**Missing:**

- Classification-specific log entries (always on, not just debug)
- Structured log format (JSON) for programmatic analysis
- Request/response correlation IDs
- Timestamp and duration metrics

---

### 4. JSON Parsing Failures Not Logged

**JSON Extraction:** `src/agentix/api_client.py:18-45`

```python
def _extract_json_payload(text: str) -> str:
    # ... complex heuristics to extract JSON from LLM response ...
    return cleaned
```

**In `query_api()` line 102:**

```python
agent_content_clean = _extract_json_payload(answer)
return json.loads(agent_content_clean)  # ⚠️ CAN RAISE JSONDecodeError
```

**Issue:**

- If `json.loads()` fails, exception propagates to `classify_prompt_sync()` which swallows it
- We never see what the LLM actually returned (raw text with natural language?)
- No log entry for JSON extraction/parsing failure

---

### 5. Tool Hallucination from `respond_directly` Path

**Bridge Routing:** `src/agentix/bridge/bridge.py`

When classification returns `next_step: respond_directly`, the bridge calls `_stream_direct_response()` which queries the LLM **without tool schemas**. The LLM:

1. Saw a complex request requiring tools
2. Received no tool schemas in its context
3. Hallucinated tool names based on common naming patterns (`list_directory`, `container.exec`)
4. AgentX tried to execute these non-existent tools and failed

**Root Cause:** Misclassification caused wrong routing path (direct response instead of planner).

---

## Decision Tree Analysis

### Expected Flow (Multi-Step Tool Request)

```
User Prompt: "Create markdown file by reviewing application code..."
    ↓
[classify_prompt]
    ├─ Intent: complex_action (multi-step: review + create + explore)
    ├─ Next Step: invoke_planner
    ├─ Working Memory Context: use_tools=true, cwd=/Projects/agentX
    └─ Missing Fields: [] (cwd available from WM)
    ↓
[invoke_planner route]
    ├─ Load planner_prompt.md
    ├─ Include tool schemas (read_file, write_file, search_files, CST, AST)
    ├─ Generate step-by-step plan
    └─ Execute tool loop with proper tool dispatch
```

### Actual Flow (Classification Failure)

```
User Prompt: "Create markdown file by reviewing application code..."
    ↓
[classify_prompt]
    ├─ Exception raised (unknown cause — not logged)
    ├─ Fallback: Intent=conversation, Next Step=respond_directly
    └─ Working Memory Context: NOT INCLUDED
    ↓
[respond_directly route]
    ├─ No tool schemas in context
    ├─ LLM generates conversational response with hallucinated tool calls
    ├─ AgentX attempts to execute "list_directory" → tool not found
    └─ LLM attempts "container.exec" → tool not found
```

---

## Proposed Logging Improvements

### Priority 1: Classification Request/Response Logging

**Goal:** Always log classification attempts with full context  
**Scope:** All classification calls, not just when `debug=True`

#### Changes Required

**File:** `src/agentix/bridge/classify_prompt.py`

Add structured logging at key decision points:

```python
import logging
import json
from datetime import datetime

logger = logging.getLogger("agentix.classification")

def classify_prompt(
    config,
    prompt: str,
    context: Context,
    history,
    max_tokens: int = 500,
) -> PromptClassificationResponse:
    start_time = datetime.now()
    
    # LOG: Classification attempt started
    logger.info(
        "Classification started",
        extra={
            "prompt_preview": prompt[:100] if prompt else None,
            "prompt_length": len(prompt) if prompt else 0,
            "context_message_count": len(context.get_enabled_messages()),
            "model": config.classification_model or config.model,
            "backend": getattr(config, "classification_backend", "ollama"),
        }
    )
    
    # ... existing code to build classification_config ...
    
    effective_history = # ... existing history filtering ...
    
    # LOG: Classification payload assembled
    logger.debug(
        "Classification payload",
        extra={
            "effective_history_length": len(effective_history),
            "system_prompt_preview": PROMPT_CLASSIFICATION[:200],
            "user_prompt": prompt,
        }
    )
    
    classification_payload = assemble_prompts(...)
    
    # LOG: Payload before sending to LLM
    if logger.isEnabledFor(logging.DEBUG):
        payload_dict = classification_payload if isinstance(classification_payload, dict) else classification_payload.to_dict()
        logger.debug(
            "Classification payload (full)",
            extra={"payload": json.dumps(payload_dict, indent=2)}
        )
    
    # Query API for classification
    try:
        result = query_classification(config, classification_payload)
        
        # LOG: Raw LLM result before parsing
        logger.info(
            "Classification raw result",
            extra={
                "result": result,
                "duration_ms": (datetime.now() - start_time).total_seconds() * 1000,
            }
        )
        
        # Parse result into PromptClassificationResponse
        response = PromptClassificationResponse(
            intent=Intent[result.get("intent", "conversation")],
            needs_clarification=result.get("needs_clarification", False),
            missing_fields=result.get("missing_fields", []),
            reasoning_summary=result.get("reasoning_summary", ""),
            next_step=NextStep[result.get("next_step", "respond_directly")],
        )
        
        # LOG: Final classification decision
        logger.info(
            "Classification complete",
            extra={
                "intent": response.intent.name,
                "next_step": response.next_step.name,
                "needs_clarification": response.needs_clarification,
                "missing_fields": response.missing_fields,
                "reasoning": response.reasoning_summary,
            }
        )
        
        return response
        
    except KeyError as e:
        # Enum lookup failed — invalid intent/next_step value from LLM
        logger.error(
            "Classification enum parse error",
            extra={
                "error": str(e),
                "raw_result": result,
                "valid_intents": [i.name for i in Intent],
                "valid_next_steps": [n.name for n in NextStep],
            },
            exc_info=True
        )
        raise
    except Exception as e:
        logger.error(
            "Classification failed",
            extra={"error": str(e)},
            exc_info=True
        )
        raise
```

---

### Priority 2: Working Memory Injection

**Goal:** Make Working Memory facts visible to classification LLM  
**Scope:** Modify classification prompt assembly to include WM context

#### Changes Required

**File:** `src/agentix/bridge/classify_prompt.py`

Extend classification payload to include WM facts:

```python
def classify_prompt(
    config,
    prompt: str,
    context: Context,
    history,
    max_tokens: int = 500,
    working_memory: Optional[WorkingMemory] = None,  # NEW PARAMETER
) -> PromptClassificationResponse:
    # ... existing config setup ...
    
    # Inject Working Memory into user prompt if available
    enhanced_prompt = prompt
    if working_memory and working_memory.get_facts():
        wm_context = _format_working_memory_for_classification(working_memory)
        enhanced_prompt = f"{wm_context}\n\n{prompt}"
    
    classification_config.user = [enhanced_prompt] if enhanced_prompt else (config.user or [])
    
    # ... rest of existing code ...


def _format_working_memory_for_classification(wm: WorkingMemory) -> str:
    """Format Working Memory facts for classification prompt."""
    lines = ["<working_memory>"]
    for fact in wm.get_facts(enabled_only=True):
        owner_icon = fact.owner.icon
        lines.append(f"{owner_icon} {fact.key}: {fact.value}")
    lines.append("</working_memory>")
    return "\n".join(lines)
```

**File:** `src/agentix/bridge/bridge.py`

Update `classify_prompt()` method to accept and pass WM:

```python
def classify_prompt(
    self,
    prompt: str,
    context: Context,
    working_memory: Optional[WorkingMemory] = None,  # NEW
) -> PromptClassificationResponse:
    return classifier(
        self.config,
        prompt,
        context,
        self._context_to_history(context),
        self._get_max_tokens(),
        working_memory=working_memory,  # NEW
    )
```

**File:** `src/agentx/integration/agentix_bridge_adapter.py`

Update adapter to pass WM through:

```python
def classify_prompt_sync(
    self, 
    prompt: str, 
    context: Context,
    working_memory: Optional[WorkingMemory] = None,  # NEW
) -> Optional[PromptClassificationResponse]:
    if not self.agentix_config.classify_prompts:
        return None
    
    try:
        return self.bridge.classify_prompt(prompt, context, working_memory)  # PASS WM
    except Exception as e:
        # ... enhanced logging (see Priority 3) ...
```

**File:** `src/agentx/session.py`

Pass session WM to classifier:

```python
def process_prompt(self, prompt: str) -> Iterator[ResponseChunk]:
    shared_context = self._build_shared_context_from_context()
    classification = None
    
    if self.config.get("agentix", {}).get("classify_prompts", False):
        classification = self.agentix_adapter.classify_prompt_sync(
            prompt,
            shared_context,
            working_memory=self.working_memory  # NEW
        )
    
    # ... rest of method ...
```

---

### Priority 3: Exception Handling & Diagnostics

**Goal:** Never swallow classification exceptions; provide full diagnostic context  
**Scope:** Improve exception handler in adapter to log actionable diagnostics

#### Changes Required

**File:** `src/agentx/integration/agentix_bridge_adapter.py`

```python
import traceback
import logging

logger = logging.getLogger("agentx.adapter")

def classify_prompt_sync(
    self, 
    prompt: str, 
    context: Context,
    working_memory: Optional[WorkingMemory] = None,
) -> Optional[PromptClassificationResponse]:
    if not self.agentix_config.classify_prompts:
        return None
    
    try:
        return self.bridge.classify_prompt(prompt, context, working_memory)
    except json.JSONDecodeError as e:
        # JSON parsing failed — log raw LLM output
        logger.error(
            "Classification JSON parse error",
            extra={
                "error": str(e),
                "prompt_preview": prompt[:200],
                "context_size": len(context.get_enabled_messages()),
                "model": self.agentix_config.classification_model or self.agentix_config.model,
            },
            exc_info=True
        )
        # Return fallback with diagnostic reasoning
        return PromptClassificationResponse(
            intent=Intent.conversation,
            needs_clarification=False,
            missing_fields=[],
            reasoning_summary=f"JSON parse error: {str(e)[:50]}",
            next_step=NextStep.respond_directly,
        )
    except KeyError as e:
        # Enum lookup failed — invalid intent/next_step from LLM
        logger.error(
            "Classification enum error",
            extra={
                "error": str(e),
                "prompt_preview": prompt[:200],
                "valid_intents": [i.name for i in Intent],
                "valid_next_steps": [n.name for n in NextStep],
            },
            exc_info=True
        )
        return PromptClassificationResponse(
            intent=Intent.conversation,
            needs_clarification=False,
            missing_fields=[],
            reasoning_summary=f"Invalid enum: {str(e)[:50]}",
            next_step=NextStep.respond_directly,
        )
    except Exception as e:
        # Unknown error — log full traceback
        logger.error(
            "Classification unexpected error",
            extra={
                "error": str(e),
                "error_type": type(e).__name__,
                "prompt_preview": prompt[:200],
                "traceback": traceback.format_exc(),
            },
            exc_info=True
        )
        return PromptClassificationResponse(
            intent=Intent.conversation,
            needs_clarification=False,
            missing_fields=[],
            reasoning_summary=f"Error: {type(e).__name__}",
            next_step=NextStep.respond_directly,
        )
```

---

### Priority 4: Session Log Integration

**Goal:** Mirror classification decisions to `session.log` for user visibility  
**Scope:** Write classification results to session transcript

#### Changes Required

**File:** `src/agentx/session.py`

Add classification logging to session transcript:

```python
def _log_classification(self, classification: Optional[PromptClassificationResponse], prompt: str):
    """Log classification decision to session.log."""
    if not classification:
        self._session_log.write("🤔 intent: (classification disabled)\n")
        return
    
    self._session_log.write(f"🤔 intent: {classification.intent.name}\n")
    self._session_log.write(f"   reasoning: {classification.reasoning_summary}\n")
    
    if classification.needs_clarification:
        self._session_log.write(f"   ⚠️  needs clarification\n")
        if classification.missing_fields:
            self._session_log.write(f"   missing: {', '.join(classification.missing_fields)}\n")
    
    self._session_log.write(f"💡 path: {classification.next_step.name}\n\n")
    self._session_log.flush()


def process_prompt(self, prompt: str) -> Iterator[ResponseChunk]:
    shared_context = self._build_shared_context_from_context()
    classification = None
    
    if self.config.get("agentix", {}).get("classify_prompts", False):
        classification = self.agentix_adapter.classify_prompt_sync(
            prompt,
            shared_context,
            working_memory=self.working_memory
        )
        self._log_classification(classification, prompt)  # NEW
    
    # ... rest of method ...
```

---

### Priority 5: Add Classification Debug Endpoint

**Goal:** Provide a way to test classification without full agent pipeline  
**Scope:** Add CLI command to test classification in isolation

#### Changes Required

**File:** `src/agentix/main.py` (new subcommand)

```python
def classify_command(args: AgentixConfig) -> None:
    """Test classification for a single prompt."""
    from agentix.bridge.classify_prompt import classify_prompt
    from shared.models.context import Context
    import json
    
    prompt = " ".join(args.user or [])
    context = Context()
    
    print("=" * 80)
    print("PROMPT:")
    print(prompt)
    print("=" * 80)
    
    result = classify_prompt(
        args,
        prompt,
        context,
        history=[],
        max_tokens=500,
    )
    
    print("\nCLASSIFICATION RESULT:")
    print(json.dumps({
        "intent": result.intent.name,
        "next_step": result.next_step.name,
        "needs_clarification": result.needs_clarification,
        "missing_fields": result.missing_fields,
        "reasoning_summary": result.reasoning_summary,
    }, indent=2))
```

**Usage:**

```bash
python -m agentix classify --user "Create a markdown file by reviewing code"
```

---

## Recommended Logging Configuration

### Structured JSON Logging

Configure Python logging to output structured JSON for programmatic analysis:

**File:** `src/agentix/logging_config.py` (new file)

```python
import logging
import logging.config
import json
from datetime import datetime

class JSONFormatter(logging.Formatter):
    def format(self, record):
        log_obj = {
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }
        
        if hasattr(record, "extra"):
            log_obj.update(record.extra)
        
        if record.exc_info:
            log_obj["exception"] = self.formatException(record.exc_info)
        
        return json.dumps(log_obj)


LOGGING_CONFIG = {
    "version": 1,
    "disable_existing_loggers": False,
    "formatters": {
        "json": {
            "()": JSONFormatter,
        },
        "console": {
            "format": "%(asctime)s [%(levelname)s] %(name)s: %(message)s"
        }
    },
    "handlers": {
        "console": {
            "class": "logging.StreamHandler",
            "formatter": "console",
            "stream": "ext://sys.stderr",
        },
        "classification_file": {
            "class": "logging.handlers.RotatingFileHandler",
            "filename": "logs/classification.jsonl",
            "formatter": "json",
            "maxBytes": 10485760,  # 10MB
            "backupCount": 5,
        },
    },
    "loggers": {
        "agentix.classification": {
            "handlers": ["console", "classification_file"],
            "level": "INFO",
            "propagate": False,
        },
        "agentx.adapter": {
            "handlers": ["console"],
            "level": "INFO",
            "propagate": False,
        },
    },
    "root": {
        "handlers": ["console"],
        "level": "WARNING",
    }
}


def configure_logging():
    import os
    os.makedirs("logs", exist_ok=True)
    logging.config.dictConfig(LOGGING_CONFIG)
```

---

## System Prompt Improvements

### Update `prompt_classification.md`

Add explicit guidance about Working Memory context:

```markdown
## [WORKING MEMORY CONTEXT]

When the user prompt is preceded by a `<working_memory>` block, use those
facts to inform your classification decisions:

<working_memory>
👤 cwd: /Projects/agentX
👤 use_tools: true
</working_memory>

**Classification Rules:**

- If `use_tools` is `true` and the request could be answered with tools,
  classify as `simple_action` or `complex_action` (NOT `conversation`).

- If `cwd` is present and the user says "working directory" or "current
  directory", do NOT flag `directory_path` as missing — it's available
  from context.

- User-owned facts (👤) are authoritative preferences; prioritize them
  over default heuristics.

- Agent-owned facts (🤖) are session state; use them for disambiguation.
```

---

## Testing Strategy

### Unit Tests

**File:** `tests/test_classification_logging.py` (new)

```python
import logging
from unittest.mock import MagicMock, patch
from agentix.bridge.classify_prompt import classify_prompt
from shared.models.context import Context
from shared.models.working_memory import WorkingMemory

def test_classification_logs_request():
    """Verify classification logs request with prompt preview."""
    logger = logging.getLogger("agentix.classification")
    with patch.object(logger, "info") as mock_info:
        classify_prompt(
            config=...,
            prompt="Test prompt",
            context=Context(),
            history=[],
        )
        
        # Check that logger.info was called with "Classification started"
        assert mock_info.call_count >= 1
        call_args = mock_info.call_args_list[0]
        assert "Classification started" in call_args[0][0]


def test_classification_includes_working_memory():
    """Verify WM facts are injected into classification prompt."""
    wm = WorkingMemory()
    wm.remember("user", "use_tools", True)
    wm.remember("user", "cwd", "/Projects/test")
    
    with patch("agentix.api_client.query_classification") as mock_query:
        mock_query.return_value = {
            "intent": "simple_action",
            "next_step": "single_tool",
            "needs_clarification": False,
            "missing_fields": [],
            "reasoning_summary": "ok",
        }
        
        classify_prompt(
            config=...,
            prompt="List the working directory",
            context=Context(),
            history=[],
            working_memory=wm,
        )
        
        # Verify payload contains WM context
        payload = mock_query.call_args[0][1]
        user_message = next(m for m in payload["messages"] if m["role"] == "user")
        assert "<working_memory>" in user_message["content"]
        assert "use_tools: True" in user_message["content"]
        assert "cwd: /Projects/test" in user_message["content"]
```

### Integration Tests

**File:** `tests/integration/test_classification_failures.py` (new)

Test actual failure scenarios from `session_2026-03-21_23-59-06`:

```python
def test_complex_action_classification():
    """Regression test for session_2026-03-21_23-59-06 misclassification."""
    wm = WorkingMemory()
    wm.remember("user", "use_tools", True)
    wm.remember("user", "cwd", "/Projects/agentX")
    
    prompt = """Create a markdown file `./agentx/agentx-application-flow.md` 
    which contains machine-readable markdown that explains the major workflow 
    of the agentX application, including how agentix works. To complete this, 
    review the application code. This is a complex task that will require 
    multi-phase steps. You should explore and make a todo list to track your 
    progress."""
    
    classification = classify_prompt(
        config=test_config,
        prompt=prompt,
        context=Context(),
        history=[],
        working_memory=wm,
    )
    
    assert classification.intent == Intent.complex_action
    assert classification.next_step == NextStep.invoke_planner
    assert classification.reasoning_summary != "Classification unavailable"
```

---

## Implementation Checklist

- [ ] **P1:** Add structured logging to `classify_prompt.py`
- [ ] **P1:** Create `logging_config.py` with JSON formatter
- [ ] **P1:** Configure classification logger to write to `logs/classification.jsonl`
- [ ] **P2:** Add `working_memory` parameter to classification pipeline
- [ ] **P2:** Implement `_format_working_memory_for_classification()`
- [ ] **P2:** Update all call sites to pass WM through
- [ ] **P3:** Enhance exception handling in `agentix_bridge_adapter.py`
- [ ] **P3:** Add exception-type-specific handling (JSON, KeyError, generic)
- [ ] **P4:** Add `_log_classification()` method to session
- [ ] **P4:** Call `_log_classification()` after each classification
- [ ] **P5:** Add `classify` CLI subcommand for testing
- [ ] Update `prompt_classification.md` with WM context rules
- [ ] Write unit tests for logging behavior
- [ ] Write integration test for regression scenario
- [ ] Update architecture docs to reflect logging flow

---

## Expected Outcomes

After implementing these improvements:

1. **Root cause identification:** Classification failures will be logged with full context (prompt, model, exception type, raw LLM output)
2. **Context-aware classification:** Working Memory facts will inform classification decisions (e.g., `use_tools: true` will trigger tool-based routing)
3. **Debugging visibility:** Session logs will show classification reasoning for every user prompt
4. **Failure recovery:** Fallback classifications will include diagnostic clues in `reasoning_summary`
5. **Testing capability:** New CLI command will allow isolated classification testing
6. **Audit trail:** Structured JSON logs will enable post-mortem analysis of classification patterns

---

## Additional Recommendations

### 1. Add Classification Metrics

Track classification accuracy over time:

```python
# In session.py
self.classification_stats = {
    "total": 0,
    "by_intent": defaultdict(int),
    "by_next_step": defaultdict(int),
    "fallback_count": 0,
}

def _update_classification_stats(self, classification):
    self.classification_stats["total"] += 1
    self.classification_stats["by_intent"][classification.intent.name] += 1
    self.classification_stats["by_next_step"][classification.next_step.name] += 1
    if classification.reasoning_summary.startswith(("Error:", "JSON parse")):
        self.classification_stats["fallback_count"] += 1

# Log stats at session end
def close(self):
    logger.info("Session classification stats", extra=self.classification_stats)
```

### 2. Add GUI Indicator for Classification Failures

Show a warning icon in the GUI when classification falls back to default:

```python
# In session.py
if classification and "unavailable" in classification.reasoning_summary.lower():
    self.gui.show_classification_warning(
        "⚠️ Classification unavailable — using fallback routing"
    )
```

### 3. Expose Classification in Settings

Add a "View Classification Details" button to the settings panel that shows:

- Current classification backend (ollama / torch)
- Model used for classification
- Last 10 classification results with reasoning

---

## Appendix: Session Log Excerpts

### Successful Classification (Message 1)

```
👤 User: Based on your identity and the current working directory, what can you infer about the work we are doing?
🤔 intent: conversation
   reasoning: The user is asking for an inference based on context which does not require a tool call.
💡 path: respond_directly
```

### Failed Classification (Message 2)

```
👤 User: Create a markdown file `./agentx/agentx-application-flow.md` which contains machine-readable markdown that explains the major workflow of the agentX application, including how agentix works.  To complete this, review the application code.  This is a complex task that will require multi-phase steps.  You should explore and make a todo list to track your progress.
🤔 intent: conversation
   reasoning: Classification unavailable
💡 path: respond_directly

💭 (gpt-oss) (The agent is thinking...)
We need to create a markdown file. We need to review the application code. So first, list directory. Use list_directory.
[🔧 Calling tool [round 1]: list_directory]

[📋 Tool result [round 1]: [DIR]  .agentx
[FILE] .agentx/agentx-instructions.md (400 bytes)
...]
```

**Expected Session Log Entry (After Improvements):**

```
👤 User: Create a markdown file `./agentx/agentx-application-flow.md` which contains machine-readable markdown that explains the major workflow of the agentX application, including how agentix works.  To complete this, review the application code.  This is a complex task that will require multi-phase steps.  You should explore and make a todo list to track your progress.
🤔 intent: complex_action
   reasoning: Multi-step request requiring code review (read_file/search_files) + file creation (write_file) + exploration; user explicitly states "multi-phase steps"
   🏛️ Working Memory context: use_tools=true, cwd=/Projects/agentX
💡 path: invoke_planner
```

---

**End of Analysis**
