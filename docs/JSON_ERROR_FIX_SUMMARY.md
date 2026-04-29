# JSON Parsing Error Fix Summary

**Date:** March 22, 2026  
**Version:** 0.10.2 → 0.10.3  
**Status:** ✅ All fixes applied and tested

## Original Problem

Production error:

```
2026-03-22 12:24:58,794 [ERROR] agentix.api_client: JSON parse error
json.decoder.JSONDecodeError: Unterminated string starting at: line 1 column 28 (char 27)
```

**Critical issue:** Error logs didn't show the actual string that failed to parse, making diagnosis impossible.

---

## Root Cause Analysis

After comprehensive debugging using `notebooks/debug_complex.ipynb`, we identified **4 distinct issues**:

### 1. Logging Visibility (Fix #1)

**Problem:** Production logs used standard `logging.Formatter` which ignores structured logging `extra` dict fields.

**Impact:** When JSON parsing failed, we couldn't see:

- Raw LLM response (`raw_answer_repr`)
- Cleaned payload (`cleaned_payload_full`)
- Error position details
- Extracted text previews

### 2. JSON Format Variations (Fix #2)

**Problem:** Extraction logic was too strict and rejected valid JSON that had:

- Leading newlines: `\n{"key": "value"}`
- Markdown fences: ` ```json\n{...}\n``` `
- Combined variations: `\n```json{...}``` `
- Pretty-printed output with whitespace

**Impact:** Models returning correctly-structured JSON were failing extraction.

### 3. Format Enforcement (Fix #3)

**Problem:** No `format='json'` parameter being sent to Ollama API.

**Impact:** Models could return conversational text, markdown documentation, or code examples instead of strict JSON.

### 4. System Message Trimming ⭐ **ROOT CAUSE** (Fix #4)

**Problem:** `trim_context()` was removing the 7688-character classification system prompt to stay under token limits.

**Impact:** Without the classification prompt containing instructions like "Output exactly one JSON object and nothing else", models reverted to conversational responses. **This was the actual root cause of all JSON parsing errors.**

---

## Fixes Applied

### Fix #1: Enhanced Logging ✅

**File:** `src/agentix/logging_config.py`  
**Lines:** 63-105

Created `DetailedFormatter` class that:

- Displays all structured logging `extra` dict fields
- Shows `repr()` of raw strings to expose hidden characters
- Intelligently truncates long strings (first/last 250 chars)
- Displays total string length

**File:** `src/agentx/main.py`  
**Lines:** 15-50

Integrated `DetailedFormatter` into production:

- Console handler: Uses `DetailedFormatter` (human-readable + extra fields)
- File handler: Uses standard `Formatter` (structured logs)
- Fallback: Basic config if import fails

**Testing:** ✅ Verified logs now show `error_pos`, `raw_answer_repr`, `cleaned_payload_full`

---

### Fix #2: Flexible JSON Extraction ✅

**File:** `src/agentix/api_client.py`  
**Lines:** 20-145

Rewrote `_extract_json_payload()` function with:

**Multi-stage extraction:**

1. Strip ALL leading/trailing whitespace (including `\n`)
2. Handle markdown fences with flexible language detection
3. Remove standalone "json"/"jsonc" lines
4. Extract JSON from surrounding text (preamble/postfix)
5. Final aggressive whitespace strip
6. Validation that result starts with `{` or `[`

**Code block detection:**

- Skips `bash`, `python`, `sh`, `javascript` blocks
- Only processes `json`, `jsonc`, or unlabeled blocks
- Logs which block types are skipped

**Testing:** ✅ 8 edge cases tested, all pass:

- Pretty-printed with leading newline
- Markdown fence with newline
- Markdown fence without newline after opening
- Combined - newline + markdown
- Raw JSON, no decoration
- JSON with preamble text
- Multiple leading newlines
- Markdown fence without language identifier

---

### Fix #3: Format Enforcement ✅

**File:** `src/agentix/agentix_config.py`  
Added `response_format: str | None` field

**File:** `src/agentix/query_payload.py`  
Added `format` parameter to `__init__` and `to_dict()`

**File:** `src/agentix/context/sessions.py`  
Reads `response_format` from config and passes to `QueryPayload`

**File:** `src/agentix/bridge/classify_prompt.py`  
Sets `classification_config.response_format = "json"`

**Testing:** ✅ Verified `"format": "json"` appears in API payload

---

### Fix #4: System Message Preservation ⭐ ✅

**File:** `src/agentix/context/sessions.py`  
**Function:** `trim_context()`

**Changes:**

- Separate system messages from other messages
- Apply token-based trimming only to non-system messages
- Prepend system messages at beginning of final message list

**Before:**

```python
# Trimmed ALL messages by token count
# Result: System prompt (7688 chars) was removed
```

**After:**

```python
system_messages = [msg for msg in messages if msg.get("role") == "system"]
non_system_messages = [msg for msg in messages if msg.get("role") != "system"]
# ... trim non_system_messages only ...
return system_messages + trimmed_history
```

**Testing:** ✅ Debug output confirmed:

```
History before trim_context: 2 messages
  Message 0: role=system, content_length=7688
  Message 1: role=user, content_length=95
History after trim_context: 2 messages  ← System message preserved!
  Message 0: role=system, content_length=7688
  Message 1: role=user, content_length=95
```

---

## Test Results

### Classification Tests with phi4-mini:3.8b

All scenarios tested successfully:

| Test | Intent | Next Step | Status |
|------|--------|-----------|--------|
| "What is Python?" | `conversation` | `respond_directly` | ✅ |
| "list files in working directory" | `simple_action` | `single_tool` | ✅ |
| "Help me build a web scraper" | `complex_action` | `invoke_planner` | ✅ |

**Results:** 3/3 passed 🎯

### Edge Case Extraction Tests

| Format | Description | Status |
|--------|-------------|--------|
| `\n{"key":"value"}` | Newline prefix | ✅ |
| ` ```json\n{...}\n``` ` | Markdown fence | ✅ |
| ` ```json{...}``` ` | No newline after fence | ✅ |
| `\n```json\n{...}\n``` ` | Combined | ✅ |
| `{"key":"value"}` | Raw JSON | ✅ |
| `Here is: {...}` | Preamble text | ✅ |
| `\n\n\n{...}` | Multiple newlines | ✅ |
| ` ```\n{...}\n``` ` | No language identifier | ✅ |

**Results:** 8/8 passed ✅

---

## Key Learnings

1. **Token-based trimming can remove critical instructions**
   - System messages contain essential formatting instructions
   - Always preserve system messages regardless of token limits
   - Consider system message tokens separately from conversation history

2. **LLMs return JSON in many valid formats**
   - Pretty-printed with newlines
   - Wrapped in markdown fences
   - With language identifiers or without
   - Multi-stage extraction needed, not single regex

3. **Structured logging requires custom formatters**
   - Standard `logging.Formatter` ignores `extra` dict entirely
   - Need custom formatter to display additional context
   - Essential for debugging serialization errors

4. **format='json' helps but isn't sufficient alone**
   - Ollama's `format='json'` parameter enforces output format
   - But models can ignore it without proper system prompts
   - Use both format parameter AND explicit instructions in system prompt

---

## Files Modified

### Core Application Code

- ✅ `src/agentix/logging_config.py` - Added `DetailedFormatter`
- ✅ `src/agentx/main.py` - Integrated `DetailedFormatter`
- ✅ `src/agentix/api_client.py` - Rewrote `_extract_json_payload()`
- ✅ `src/agentix/agentix_config.py` - Added `response_format` field
- ✅ `src/agentix/query_payload.py` - Added `format` parameter support
- ✅ `src/agentix/context/sessions.py` - Fixed `trim_context()`, added format passthrough
- ✅ `src/agentix/bridge/classify_prompt.py` - Set `response_format='json'`

### Documentation & Testing

- ✅ `pyproject.toml` - Version bump 0.10.2 → 0.10.3
- ✅ `notebooks/debug_complex.ipynb` - 12-cell debug workflow
- ✅ `docs/JSON_ERROR_FIX_SUMMARY.md` - This document

---

## Verification Commands

```bash
# Check DetailedFormatter exists
grep -n "class DetailedFormatter" src/agentix/logging_config.py

# Check production integration
grep -n "DetailedFormatter" src/agentx/main.py

# Check flexible extraction
grep -n "def _extract_json_payload" src/agentix/api_client.py

# Check system message preservation
grep -A 10 "def trim_context" src/agentix/context/sessions.py

# Check version
grep "version =" pyproject.toml
```

## Quick Test

```python
from agentix.agentix_config import AgentixConfig
from agentix.bridge.classify_prompt import classify_prompt
from shared.models.context import Context
from shared.models.working_memory import WorkingMemory, FactOwner

wm = WorkingMemory()
wm.add_fact(FactOwner.USER, "use_tools", True)

config = AgentixConfig()
config.classification_model = "phi4-mini:3.8b"

result = classify_prompt(
    config=config,
    prompt="What is Python?",
    context=Context(),
    history=[],
    working_memory=wm
)

print(f"✅ {result.intent.name} → {result.next_step.name}")
# Expected: ✅ conversation → respond_directly
```

---

## Deployment Status

- ✅ All fixes committed to codebase
- ✅ All tests passing (3/3 classification scenarios)
- ✅ Edge case handling verified (8/8 extraction scenarios)
- ✅ Version updated to 0.10.3
- ✅ Production-ready

## Future Recommendations

1. **Monitoring:**
   - Continue using `DetailedFormatter` in production for visibility
   - Log all JSON extraction attempts with raw input
   - Track classification success rates by model

2. **Model Selection:**
   - `phi4-mini:3.8b` (2.5GB) - Lightweight but requires all fixes
   - `gpt-oss:latest` (13GB) - More reliable but larger
   - Consider testing with `llama3` or other Ollama models

3. **System Prompt Optimization:**
   - Current classification prompt is 7688 chars (effective)
   - Could potentially trim non-essential sections
   - Keep the "Output exactly one JSON object" instruction prominent

4. **Token Management:**
   - Consider separate token budgets for system vs. conversation messages
   - Add warnings when system messages exceed expected size
   - Monitor token usage in production

---

**Status:** All issues resolved. Production classification working correctly with full error visibility.
