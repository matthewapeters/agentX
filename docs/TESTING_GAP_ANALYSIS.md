# Why the FIFO Messaging Gap Wasn't Caught by Tests

_Last updated: 2026-05-09 (v0.39.0)_

## The Gap

Integration tests verified that `StreamingController.write_output()` **was called** with the right arguments, but **never verified** that data actually flowed end-to-end through the FIFO to a reader. The system had a critical hidden assumption: "If we call write_output(), data reaches the TUI."

This assumption was **wrong**.

## Testing Pyramid Problem

```
┌─────────────────────────────────────────────┐
│  E2E Tests (Missing)                       │
│  - Real FIFO communication                 │
│  - Multiple processes                      │
│  - Data actually flows to TUI              │
├─────────────────────────────────────────────┤
│  Integration Tests (Too Mocked)            │
│  - Mock TuiBridge entirely                 │
│  - Only verify write_output() called       │
│  - Session + GUI logic but NO FIFO testing │
├─────────────────────────────────────────────┤
│  Unit Tests (Too Isolated)                 │
│  - Mock OS layer (os.open, os.write)       │
│  - Mock select.select()                    │
│  - Don't test real streaming flow          │
└─────────────────────────────────────────────┘
```

## Examples of the Testing Gap

### 1. Integration Test: Mocks Entire Bridge

**File:** `tests/test_session_gui_integration.py` line 178

```python
def test_layout_runs_bootstrap_prompt_and_shows_only_agent_response(self):
    # ... setup ...
    self.session.tui_bridge = MagicMock()  # ❌ Entire bridge is fake!
    
    self.session.layout()
    
    # Only verifies: was write_output() called?
    self.session.tui_bridge.write_output.assert_any_call("###AGENT\nHello! I am AgentX.\n###DONE\n")
```

**Problem:**
- ✗ Doesn't verify data reaches actual FIFO
- ✗ Doesn't verify Lua TUI can read the data
- ✗ Doesn't test inter-process communication
- ✓ Only checks "was the method called?"

### 2. Streaming Controller Test: Verifies Calls, Not Flow

**File:** `tests/test_tui_bridge_output.py` lines 87-103

```python
def test_streaming_controller_writes_agent_and_tool_records_to_tui() -> None:
    session = _build_session(show_thinking=False)
    session.tui_bridge = MagicMock()  # ❌ Mock entire bridge
    controller = StreamingController(session)
    
    controller._handle_stream_content("hi")
    controller._display_tool_call("read_file", {"path": "src/app.py"})
    
    # Only checks: were these args passed to write_output()?
    calls = [str(call.args[0]) for call in session.tui_bridge.write_output.call_args_list]
    assert any(record == "###AGENT\n" for record in calls)
    assert any(record == "hi" for record in calls)
```

**Problem:**
- ✗ No actual FIFO created or written to
- ✗ Doesn't verify non-blocking write behavior
- ✗ Doesn't test "what happens if FIFO reader is slow?"
- ✓ Only verifies internal call sequence

### 3. Unit Test: OS Layer Mocked

**File:** `tests/test_tui_bridge_output.py` lines 58-72

```python
def test_tui_bridge_write_output_success() -> None:
    bridge = TuiBridge("/tmp/test-output.fifo", enabled=True, write_timeout_sec=0.1)
    bridge.start()
    
    with (
        patch("agentx.integration.tui_bridge.os.open", return_value=11),
        patch("agentx.integration.tui_bridge.select.select", return_value=([], [11], [])),
        patch("agentx.integration.tui_bridge.os.write", return_value=5) as mock_write,
        patch("agentx.integration.tui_bridge.os.close") as mock_close,
    ):
        ok = bridge.write_output("hello")
    
    assert ok is True
```

**Problem:**
- ✗ OS layer is completely mocked (os.open, os.write, select.select)
- ✗ Never tests real FIFO semantics
- ✗ Doesn't verify Lua can actually read the data
- ✓ Tests internal TuiBridge error handling logic

## What Was Never Tested

### ❌ 1. Complete Data Path with Real FIFO

```
StreamingController._write_tui_output("content")
  ↓
TuiBridge.write_output("content")  ← Mocked! Can't verify actual FIFO write
  ↓
/tmp/agentx_*.tui_output.fifo  ← Never tested if data reaches here
  ↓
Lua: vim.fn.jobstart({"bash", "-lc", "cat $FIFO"})  ← Never tested
  ↓
Output buffer appended to  ← Never verified
```

### ❌ 2. Non-Blocking Write Semantics

The FIFO bridge intentionally uses **non-blocking writes** to avoid blocking the GUI:

```python
fd = os.open(self.output_fifo, os.O_WRONLY | os.O_NONBLOCK)  # NON-BLOCKING
```

**This means:**
- If FIFO reader is slow → write silently drops
- If FIFO reader disconnects → write returns False
- If FIFO has no reader → write fails with ENXIO

**Test gap:** No integration test verified "what happens when data is dropped?"

### ❌ 3. Concurrent Multi-Process Scenario

Real usage:
1. Python session running in tmux window 1
2. Neovim TUI running in tmux window 2
3. Reading from same FIFO

**Test gap:** Never simulated actual two-process scenario where:
- Python writes to FIFO
- Lua/TUI reads from FIFO
- What if reader connects late?
- What if reader disconnects?
- What if reader is slow?

## Why This Gap Existed

### 1. Test Isolation Best Practice

Tests are designed to be **hermetic** (self-contained, no external dependencies). Testing real FIFOs requires:
- Actual OS FIFOs ✓ (system has `/tmp`)
- Two processes ✗ (hard to coordinate)
- Real timing ✗ (flaky)

So tests mocked the FIFO layer to avoid these complexities.

### 2. Point-to-Point Architecture

With direct FIFO writes, there's **no central coordination point**:

```
StreamingController → TuiBridge.write_output() → FIFO (no one else knows about it)
```

This made it hard to test the complete chain because:
- Each component tested in isolation
- No layer that aggregates or buffers events
- No way to verify "all subscribers got the data"

### 3. Non-Blocking Semantics Hidden Complexity

The non-blocking write design (intentional to avoid GUI latency) meant:
- Data could disappear silently
- No one noticed in tests (data wasn't real)
- No one noticed in live usage until user reported it

## Why the Event-Broker Fixes This

### ✅ 1. Testable Central Hub

```python
# Now there's a testable point:
broker.publish(EventType.AGENT_CONTENT, {"text": "..."})

# Multiple subscribers can be wired:
broker.subscribe(EventType.AGENT_CONTENT, gui_handler)
broker.subscribe(EventType.AGENT_CONTENT, tui_handler)
broker.subscribe(EventType.AGENT_CONTENT, logging_handler)
```

Tests can verify **all subscribers get the event**, not just "was write_output() called?"

### ✅ 2. Per-Subscriber Queuing

```python
class EventBroker:
    subscribers[EventType.AGENT_CONTENT] = [
        (callback_1, queue_1),  # GUI gets its own queue
        (callback_2, queue_2),  # TUI gets its own queue  ← Can't lose data!
    ]
```

Now tests can verify:
- Event queued for TUI subscriber
- TUI subscriber background thread processes queue
- Data written to FIFO with retry

### ✅ 3. Deterministic Test Scenarios

```python
def test_tui_subscriber_buffers_events_when_fifo_unavailable():
    subscriber = TUIEventSubscriber()
    
    # Simulate FIFO not ready
    with patch.object(subscriber, "_write_event_to_fifo", return_value=False):
        subscriber.handle_event(event1)
        subscriber.handle_event(event2)
    
    # Both events should be in queue
    assert len(subscriber._event_queue) == 2
    
    # Now FIFO is ready
    with patch.object(subscriber, "_write_event_to_fifo", return_value=True):
        subscriber._writer_loop()
    
    # Both events should have been written
    assert len(subscriber._event_queue) == 0
```

## Testing Recommendations

### 1. Add "Slow Subscriber" Tests

```python
@pytest.mark.functional
def test_streaming_continues_when_tui_subscriber_slow():
    """Verify publisher doesn't block when TUI subscriber is slow."""
    broker = EventBroker()
    
    # Simulate slow subscriber
    processed = []
    def slow_handler(event):
        time.sleep(0.1)  # Simulate slow processing
        processed.append(event)
    
    broker.subscribe(EventType.AGENT_CONTENT, slow_handler)
    
    # Publish rapidly
    start = time.time()
    for i in range(10):
        broker.publish(EventType.AGENT_CONTENT, {"text": f"msg {i}"})
    elapsed = time.time() - start
    
    # Publishing should be fast (no blocking)
    assert elapsed < 0.5, "Publisher blocked on slow subscriber"
    
    # Eventually all events should process
    time.sleep(2.0)
    assert len(processed) == 10
```

### 2. Add "FIFO Unavailable" Tests

```python
@pytest.mark.functional
def test_tui_subscriber_retries_when_fifo_unavailable():
    """Verify TUI subscriber retries and eventually writes when FIFO becomes available."""
    subscriber = TUIEventSubscriber(mock_bridge)
    subscriber.start()
    
    event = Event(EventType.AGENT_CONTENT, {"text": "hello"})
    
    # Add event to queue
    subscriber.handle_event(event)
    
    # Simulate FIFO unavailable then becomes available
    with patch.object(mock_bridge, "is_enabled", False):
        time.sleep(0.1)
    
    with patch.object(mock_bridge, "is_enabled", True):
        time.sleep(0.5)  # Give writer thread time to retry
    
    # Event should eventually be written
    assert mock_bridge.write_output.called
```

### 3. Add Process-to-Process Tests (Optional)

```python
@pytest.mark.functional
def test_lua_tui_can_read_output_from_event_broker(tmp_path):
    """Verify actual Lua TUI can read output generated by event broker."""
    output_fifo = tmp_path / "output.fifo"
    os.mkfifo(output_fifo)
    
    # Start TUI subscriber writing to real FIFO
    bridge = TuiBridge(output_fifo=str(output_fifo), enabled=True)
    subscriber = TUIEventSubscriber(tui_bridge=bridge)
    subscriber.start()
    
    # Background process to read FIFO
    received = []
    def read_fifo():
        with open(output_fifo) as f:
            received.append(f.read())
    
    reader_thread = threading.Thread(target=read_fifo)
    reader_thread.start()
    
    # Publish event
    event = Event(EventType.AGENT_CONTENT, {"text": "hello"})
    subscriber.handle_event(event)
    
    time.sleep(1.0)
    reader_thread.join(timeout=2.0)
    subscriber.stop()
    
    # Verify FIFO reader got the data
    assert len(received) > 0
    assert "hello" in received[0]
```

## Summary

| Aspect | Before (Gap) | After (EventBroker) |
|--------|------------|------------------|
| **Testability** | Mocked entire FIFO layer | Real queues, testable pub-sub |
| **Data Loss** | Silent drops if slow reader | Bounded queues, no loss |
| **Multi-subscriber** | Tightly coupled to FIFO | Central hub, easy to test |
| **Determinism** | Race conditions possible | Ordered delivery in tests |
| **Coverage** | Only "was method called?" | "Did data reach all subscribers?" |

The event-broker pattern enables **testable, robust messaging** that the old point-to-point FIFO approach could never provide.
