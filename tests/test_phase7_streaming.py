"""
Phase 7 Tests: Streaming & Real-time Features

Tests for streaming tool execution, progress tracking, and UI components.
"""

import sys
import os
from pathlib import Path
from unittest.mock import Mock, MagicMock, patch
import tkinter as tk
import time

# Add src to path
project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentx.integration import (
    StreamingExecutor,
    ProgressUpdate,
    ProgressType,
    ProgressTracker,
    StreamingToolChain,
    create_progress_stream,
    ProgressIndicator,
    ProgressPanel,
    ResultStreamWidget,
    StreamingExecutionUI,
)


class TestStreamingExecutor:
    """Tests for streaming tool execution."""

    def setup_method(self):
        """Setup test environment."""
        self.executor = StreamingExecutor(max_chunk_size=100)
        self.mock_tool_executor = MagicMock()

    def test_executor_initialization(self):
        """Test executor initialization."""
        assert self.executor.max_chunk_size == 100
        assert self.executor.is_cancelled() == False
        print("✅ StreamingExecutor initialization works")

    def test_chunk_size_configuration(self):
        """Test custom chunk size."""
        executor = StreamingExecutor(max_chunk_size=512)
        assert executor.max_chunk_size == 512
        print("✅ Custom chunk size works")

    def test_cancellation(self):
        """Test cancellation mechanism."""
        self.executor.cancel_execution()
        assert self.executor.is_cancelled() == True
        print("✅ Cancellation works")

    def test_progress_updates_generation(self):
        """Test streaming execution generates progress updates."""
        self.mock_tool_executor.execute.return_value = "test result"

        updates = list(self.executor.execute_with_streaming(self.mock_tool_executor, "test_tool", {}))

        # Should have started, chunks, and complete
        assert len(updates) > 0
        assert updates[0].type == ProgressType.STARTED.value
        assert updates[-1].type == ProgressType.COMPLETE.value
        print("✅ Progress updates generation works")

    def test_progress_callback(self):
        """Test progress callbacks."""
        callback_count = [0]

        def callback(update):
            callback_count[0] += 1

        self.mock_tool_executor.execute.return_value = "test"

        list(self.executor.execute_with_streaming(self.mock_tool_executor, "test_tool", {}, progress_callback=callback))

        assert callback_count[0] > 0
        print("✅ Progress callbacks work")


class TestProgressUpdate:
    """Tests for progress update objects."""

    def test_progress_update_creation(self):
        """Test creating progress updates."""
        update = ProgressUpdate(
            type=ProgressType.PROGRESS.value, tool_name="test_tool", current=50, total=100, percent=50.0
        )

        assert update.tool_name == "test_tool"
        assert update.percent == 50.0
        assert update.timestamp is not None
        print("✅ ProgressUpdate creation works")

    def test_progress_update_to_dict(self):
        """Test converting progress updates to dict."""
        update = ProgressUpdate(type=ProgressType.CHUNK.value, tool_name="test_tool", data="test data")

        result = update.to_dict()

        assert isinstance(result, dict)
        assert result["tool_name"] == "test_tool"
        assert result["data"] == "test data"
        print("✅ ProgressUpdate.to_dict() works")

    def test_progress_update_to_json(self):
        """Test converting progress updates to JSON."""
        update = ProgressUpdate(type=ProgressType.COMPLETE.value, tool_name="test_tool")

        json_str = update.to_json()

        assert isinstance(json_str, str)
        assert "test_tool" in json_str
        assert "complete" in json_str
        print("✅ ProgressUpdate.to_json() works")


class TestProgressTracker:
    """Tests for progress tracking."""

    def setup_method(self):
        """Setup test environment."""
        self.tracker = ProgressTracker()

    def test_start_operation(self):
        """Test starting an operation."""
        self.tracker.start_operation("op1", "test_tool")

        progress = self.tracker.get_progress("op1")

        assert progress is not None
        assert progress.tool_name == "test_tool"
        print("✅ Start operation works")

    def test_update_progress(self):
        """Test updating progress."""
        self.tracker.start_operation("op1", "test_tool")

        update = ProgressUpdate(type=ProgressType.PROGRESS.value, tool_name="test_tool", percent=50.0)

        self.tracker.update_progress("op1", update)
        progress = self.tracker.get_progress("op1")

        assert progress.percent == 50.0
        print("✅ Update progress works")

    def test_get_all_progress(self):
        """Test getting all operations."""
        self.tracker.start_operation("op1", "tool1")
        self.tracker.start_operation("op2", "tool2")

        all_progress = self.tracker.get_all_progress()

        assert len(all_progress) == 2
        assert "op1" in all_progress
        assert "op2" in all_progress
        print("✅ Get all progress works")

    def test_complete_operation(self):
        """Test completing an operation."""
        self.tracker.start_operation("op1", "test_tool")
        self.tracker.complete_operation("op1")

        progress = self.tracker.get_progress("op1")

        assert progress.type == ProgressType.COMPLETE.value
        print("✅ Complete operation works")

    def test_callback_registration(self):
        """Test registering progress callbacks."""
        callback_called = [False]

        def callback(update):
            callback_called[0] = True

        self.tracker.start_operation("op1", "test_tool")
        self.tracker.register_callback("op1", callback)

        update = ProgressUpdate(type=ProgressType.PROGRESS.value, tool_name="test_tool")
        self.tracker.update_progress("op1", update)

        assert callback_called[0] == True
        print("✅ Callback registration works")


class TestStreamingToolChain:
    """Tests for tool chaining."""

    def setup_method(self):
        """Setup test environment."""
        self.chain = StreamingToolChain()
        self.executor = StreamingExecutor()
        self.mock_tool_executor = MagicMock()

    def test_add_tool_to_chain(self):
        """Test adding tools to chain."""
        self.chain.add_tool("tool1", {"arg": "value"})
        self.chain.add_tool("tool2", {"arg2": "value2"})

        assert len(self.chain._chain) == 2
        print("✅ Add tool to chain works")

    def test_chain_builder_pattern(self):
        """Test chain building with fluent API."""
        chain = StreamingToolChain()  # Create fresh chain
        result = chain.add_tool("t1", {}).add_tool("t2", {})

        assert result is chain
        assert len(chain._chain) == 2
        print("✅ Chain builder pattern works")

    def test_execute_chain(self):
        """Test executing tool chain."""
        self.mock_tool_executor.execute.return_value = "result"

        self.chain.add_tool("tool1", {})
        self.chain.add_tool("tool2", {})

        updates = list(self.chain.execute_chain(self.executor, self.mock_tool_executor))

        # Should have updates for multiple tools
        assert len(updates) > 0
        print("✅ Execute chain works")


class TestProgressWidgets:
    """Tests for GUI progress widgets."""

    def setup_method(self):
        """Setup test environment."""
        self.root = tk.Tk()
        self.root.withdraw()  # Hide window

    def teardown_method(self):
        """Cleanup."""
        try:
            self.root.destroy()
        except:
            pass

    def test_progress_indicator_creation(self):
        """Test creating progress indicator."""
        indicator = ProgressIndicator(self.root, "test_tool")

        assert indicator.tool_name == "test_tool"
        assert indicator.progress_var.get() == 0
        print("✅ ProgressIndicator creation works")

    def test_progress_indicator_update(self):
        """Test updating progress indicator."""
        indicator = ProgressIndicator(self.root, "test_tool")

        update = ProgressUpdate(
            type=ProgressType.PROGRESS.value, tool_name="test_tool", percent=75.0, current=75, total=100
        )

        indicator.update_progress(update)

        assert indicator.progress_var.get() == 75.0
        print("✅ ProgressIndicator update works")

    def test_result_stream_widget_creation(self):
        """Test creating result stream widget."""
        widget = ResultStreamWidget(self.root)

        assert widget.text_widget is not None
        print("✅ ResultStreamWidget creation works")

    def test_result_stream_append(self):
        """Test appending to result stream."""
        widget = ResultStreamWidget(self.root)

        widget.append_chunk("test chunk")

        content = widget.text_widget.get("1.0", "end")
        assert "test chunk" in content
        print("✅ ResultStreamWidget append works")

    def test_streaming_execution_ui(self):
        """Test complete streaming execution UI."""
        ui = StreamingExecutionUI(self.root)

        assert ui.progress_panel is not None
        assert ui.result_stream is not None
        print("✅ StreamingExecutionUI creation works")


def test_phase7_integration():
    """Full integration test for Phase 7."""
    print("\n" + "=" * 60)
    print("Phase 7 Integration Test")
    print("=" * 60)

    # Test 1: Streaming execution
    executor = StreamingExecutor()
    assert executor.max_chunk_size > 0

    # Test 2: Progress updates
    update = ProgressUpdate(type=ProgressType.PROGRESS.value, tool_name="test")
    assert update.timestamp is not None

    # Test 3: Progress tracker
    tracker = ProgressTracker()
    tracker.start_operation("op1", "tool1")
    assert tracker.get_progress("op1") is not None

    # Test 4: Tool chain
    chain = StreamingToolChain()
    chain.add_tool("t1", {}).add_tool("t2", {})
    assert len(chain._chain) == 2

    print("✅ Phase 7 integration test passed")


# ---------------------------------------------------------------------------
# Coverage uplift: cancellation, error paths, unregister, create_progress_stream
# ---------------------------------------------------------------------------


class TestStreamingExecutorCoverageUplift:
    """Targeted coverage tests for uncovered streaming_executor paths."""

    def setup_method(self):
        self.executor = StreamingExecutor(max_chunk_size=50)
        self.mock_executor = MagicMock()

    def test_cancellation_mid_execution_yields_cancelled_update(self):
        """Cancelling *before* execute returns causes a CANCELLED chunk."""

        def slow_execute(tool_name, arguments):
            # Cancel inside the tool so post-execute check fires
            self.executor.cancel_execution()
            return "result"

        self.mock_executor.execute.side_effect = slow_execute

        updates = list(self.executor.execute_with_streaming(self.mock_executor, "tool", {}))
        types = [u.type for u in updates]
        assert ProgressType.CANCELLED.value in types

    def test_error_path_yields_error_update(self):
        """Exceptions from execute are caught and yielded as ERROR chunks."""
        self.mock_executor.execute.side_effect = RuntimeError("boom")

        updates = list(self.executor.execute_with_streaming(self.mock_executor, "tool", {}))
        assert updates[-1].type == ProgressType.ERROR.value
        assert "boom" in updates[-1].message

    def test_error_path_triggers_callback(self):
        """Error path calls progress_callback with the ERROR update."""
        self.mock_executor.execute.side_effect = ValueError("oops")
        received = []

        list(
            self.executor.execute_with_streaming(
                self.mock_executor, "tool", {}, progress_callback=lambda u: received.append(u)
            )
        )

        assert any(u.type == ProgressType.ERROR.value for u in received)

    def test_unregister_callback_removes_entry(self):
        """unregister_callback removes the callback from the tracker."""
        tracker = ProgressTracker()
        tracker.start_operation("op1", "tool1")
        tracker.register_callback("op1", lambda u: None)
        assert "op1" in tracker._callbacks
        tracker.unregister_callback("op1")
        assert "op1" not in tracker._callbacks

    def test_unregister_callback_noop_for_unknown_id(self):
        """unregister_callback on an unknown operation ID does not raise."""
        tracker = ProgressTracker()
        tracker.unregister_callback("nonexistent")  # must not raise

    def test_chain_is_cancelled_breaks_early(self):
        """Chain execution stops when executor is cancelled."""
        executor = StreamingExecutor(max_chunk_size=50)
        executor.cancel_execution()  # pre-cancel

        tool_exc = MagicMock()
        tool_exc.execute.return_value = "data"

        chain = StreamingToolChain()
        chain.add_tool("t1", {}).add_tool("t2", {})

        updates = list(chain.execute_chain(executor, tool_exc))
        # No tools should run because executor is already cancelled
        tool_exc.execute.assert_not_called()
        assert updates == []

    def test_chain_get_results_after_execution(self):
        """get_results returns accumulated chunk data after execute_chain."""
        executor = StreamingExecutor(max_chunk_size=1024)
        tool_exc = MagicMock()
        tool_exc.execute.return_value = "hello"

        chain = StreamingToolChain()
        chain.add_tool("greet", {})

        list(chain.execute_chain(executor, tool_exc))
        results = chain.get_results()
        assert "greet" in results

    def test_create_progress_stream_yields_updates(self):
        """create_progress_stream convenience wrapper yields at least one update."""
        tool_exc = MagicMock()
        tool_exc.execute.return_value = "streamed result"

        updates = list(create_progress_stream(tool_exc, "my_tool", {}))
        assert len(updates) > 0
        assert updates[0].type == ProgressType.STARTED.value
        assert updates[-1].type == ProgressType.COMPLETE.value


if __name__ == "__main__":
    print("\n" + "=" * 60)
    print("Running Phase 7 Streaming & Real-time Tests")
    print("=" * 60 + "\n")

    # Test StreamingExecutor
    print("Testing StreamingExecutor...")
    tester = TestStreamingExecutor()

    tester.setup_method()
    tester.test_executor_initialization()
    tester.test_chunk_size_configuration()
    tester.test_cancellation()
    tester.test_progress_updates_generation()
    tester.test_progress_callback()

    # Test ProgressUpdate
    print("\nTesting ProgressUpdate...")
    test_update = TestProgressUpdate()
    test_update.test_progress_update_creation()
    test_update.test_progress_update_to_dict()
    test_update.test_progress_update_to_json()

    # Test ProgressTracker
    print("\nTesting ProgressTracker...")
    test_tracker = TestProgressTracker()

    test_tracker.setup_method()
    test_tracker.test_start_operation()
    test_tracker.test_update_progress()
    test_tracker.test_get_all_progress()
    test_tracker.test_complete_operation()
    test_tracker.test_callback_registration()

    # Test StreamingToolChain
    print("\nTesting StreamingToolChain...")
    test_chain = TestStreamingToolChain()

    test_chain.setup_method()
    test_chain.test_add_tool_to_chain()
    test_chain.test_chain_builder_pattern()
    test_chain.test_execute_chain()

    # Test Widgets
    print("\nTesting Progress Widgets...")
    test_widgets = TestProgressWidgets()

    test_widgets.setup_method()
    test_widgets.test_progress_indicator_creation()
    test_widgets.test_progress_indicator_update()
    test_widgets.test_result_stream_widget_creation()
    test_widgets.test_result_stream_append()
    test_widgets.test_streaming_execution_ui()
    test_widgets.teardown_method()

    # Integration test
    test_phase7_integration()

    print("\n" + "=" * 60)
    print("🎉 All Phase 7 tests passed!")
    print("=" * 60)
