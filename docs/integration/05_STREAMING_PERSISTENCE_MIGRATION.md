# Streaming Persistence Migration Notes

## Summary

AgentX now uses a single canonical streaming path for interactive chat turns:

- `AgentXSession.stream_ollama_response_worker()`
- `AgentXSession._stream_via_agentix()`

The legacy `_stream_direct_ollama()` implementation has been converted into a
compatibility shim that delegates to `_stream_via_agentix()`.

## Canonical Behavior Contract

For each streamed user turn:

1. User message is added to context.
2. Streaming chunks are displayed in the GUI.
3. THINKING chunks are accumulated and persisted as one context message:
   - role: `THINKING`
   - enabled: `False`
4. CONTENT chunks are accumulated and persisted as one context message:
   - role: `ASSISTANT`
   - enabled: `True`

This behavior is now consistent for:

- GUI submit flow (`stream_ollama_response_worker`)
- Test-facing flow (`process_prompt`)

## Backward Compatibility

`_stream_direct_ollama()` remains callable for compatibility in tests and old
callers, but no longer contains an independent streaming implementation.

## Regression Coverage

Key tests validating this migration:

- `tests/test_session_stream_context_persistence.py`
  - Persists THINKING and ASSISTANT for `process_prompt`
  - Persists THINKING and ASSISTANT for GUI worker flow
- `tests/test_functional_chat.py`
  - Verifies worker uses Agentix path
- `tests/test_active_model.py`
  - Verifies deprecated direct method delegates to canonical path

## Why this change

The prior state had split-brain behavior across multiple streaming code paths,
causing inconsistent context persistence and drift between GUI/runtime and tests.
The canonical contract removes that divergence.
