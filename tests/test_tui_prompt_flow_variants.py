"""Integration/functional/E2E coverage for TUI prompt submit and output routing flows."""

from __future__ import annotations

import os
import select
import threading
import time
import tkinter as tk
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest

from agentx.integration.tui_bridge import SUBMIT_SENTINEL
from agentx.session import AgentXSession
from shared.models.response import ChunkType, ResponseChunk


def _build_config(output_fifo: str, input_fifo: str, classify_prompts: bool) -> dict:
    """Build a minimal valid config for headless TUI prompt-flow testing.

    Args:
        output_fifo: Absolute path to the TUI output FIFO.
        input_fifo: Absolute path to the TUI input FIFO.
        classify_prompts: Whether prompt classification is enabled.

    Returns:
        Config dict accepted by AgentXSession.
    """
    return {
        "agentx": {
            "ollama_model": "gpt-oss:latest",
            "ollama_host": "localhost:11434",
            "enable_gui_chat": False,
        },
        "tui": {
            "enable": True,
            "output_fifo": output_fifo,
            "input_fifo": input_fifo,
            "write_timeout_sec": 0.2,
            "show_thinking": True,
        },
        "agentix": {
            "host": "localhost:8000",
            "classify_prompts": classify_prompts,
            "available_tools": [],
        },
    }


def _write_fifo_payload(path: str, payload: str) -> None:
    """Write payload to a FIFO, retrying briefly while reader endpoint appears.

    Args:
        path: FIFO path.
        payload: UTF-8 payload to write.
    """
    deadline = time.time() + 2.0
    while time.time() < deadline:
        try:
            fd = os.open(path, os.O_WRONLY | os.O_NONBLOCK)
            try:
                os.write(fd, payload.encode("utf-8"))
                return
            finally:
                os.close(fd)
        except OSError:
            time.sleep(0.01)
    raise AssertionError(f"Timed out opening fifo for write: {path}")


def _start_nonblocking_fifo_reader(path: str, sink: list[str], stop_event: threading.Event) -> threading.Thread:
    """Start a background reader that accumulates output FIFO text.

    Args:
        path: FIFO path to read from.
        sink: Mutable list receiving decoded text chunks.
        stop_event: Stop signal for the reader loop.

    Returns:
        Started reader thread.
    """

    def _reader() -> None:
        fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK)
        try:
            while not stop_event.is_set():
                readable, _, _ = select.select([fd], [], [], 0.05)
                if not readable:
                    continue
                data = os.read(fd, 4096)
                if not data:
                    time.sleep(0.01)
                    continue
                sink.append(data.decode("utf-8", errors="replace"))
        finally:
            try:
                os.close(fd)
            except OSError:
                pass

    thread = threading.Thread(target=_reader, daemon=True)
    thread.start()
    return thread


@pytest.mark.integration
def test_tui_submit_routes_prompt_to_classification_and_processing(tmp_path: Path) -> None:
    """
    GIVEN a user input buffer in the TUI
    AND an AgentX output buffer in the TUI
    AND the AgentX application and LLM wiring
    WHEN the user submit payload arrives through the TUI input FIFO
    THEN AgentX routes the prompt to classification
    AND processing continues through the prompt generator
    """
    output_fifo = tmp_path / "tui.output.fifo"
    input_fifo = tmp_path / "tui.input.fifo"
    os.mkfifo(output_fifo)
    os.mkfifo(input_fifo)

    captured_output: list[str] = []
    reader_stop = threading.Event()
    reader_thread = _start_nonblocking_fifo_reader(str(output_fifo), captured_output, reader_stop)

    root = tk.Tk(useTk=False)
    config = _build_config(str(output_fifo), str(input_fifo), classify_prompts=True)

    with patch("agentx.session.create_adapter") as mock_create_adapter:
        adapter = MagicMock()
        adapter.get_models.return_value = []
        adapter.classify_prompt_sync.return_value = SimpleNamespace(
            intent="conversation",
            next_step="respond_directly",
            reasoning_summary="ok",
            needs_clarification=False,
            missing_fields=[],
        )
        adapter.process_prompt_generator.return_value = iter(
            [
                ResponseChunk(type=ChunkType.CONTENT, content="INTEGRATION_TUI_OK"),
                ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
            ]
        )
        mock_create_adapter.return_value = adapter

        session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
        # Execute scheduled callbacks synchronously so background submit path is deterministic.
        session._safe_root_after = lambda callback: callback()

        try:
            prompt = "classify this please"
            _write_fifo_payload(str(input_fifo), f"{prompt}{SUBMIT_SENTINEL}")

            deadline = time.time() + 3.0
            while time.time() < deadline:
                if adapter.classify_prompt_sync.called and adapter.process_prompt_generator.called:
                    break
                time.sleep(0.02)

            output_deadline = time.time() + 4.0
            while time.time() < output_deadline:
                joined = "".join(captured_output)
                if "###USER" in joined and "INTEGRATION_TUI_OK" in joined and "###DONE" in joined:
                    break
                time.sleep(0.02)

            assert adapter.classify_prompt_sync.called
            assert adapter.process_prompt_generator.called
            assert adapter.classify_prompt_sync.call_args.args[0] == prompt
            assert adapter.process_prompt_generator.call_args.args[0] == prompt
            joined = "".join(captured_output)
            assert "###USER" in joined
            assert "INTEGRATION_TUI_OK" in joined
            assert "###DONE" in joined
        finally:
            session.close()
            reader_stop.set()
            reader_thread.join(timeout=1.0)


@pytest.mark.functional
def test_llm_response_routes_to_tui_output_buffer(tmp_path: Path) -> None:
    """
    GIVEN a user input buffer in the TUI
    AND an AgentX output buffer in the TUI
    AND the AgentX application and LLM wiring
    WHEN the LLM responds to a submitted prompt
    THEN AgentX routes the streamed response to the TUI output buffer FIFO
    """
    output_fifo = tmp_path / "tui.output.fifo"
    input_fifo = tmp_path / "tui.input.fifo"
    os.mkfifo(output_fifo)
    os.mkfifo(input_fifo)

    root = tk.Tk(useTk=False)
    config = _build_config(str(output_fifo), str(input_fifo), classify_prompts=False)

    captured_output: list[str] = []
    reader_stop = threading.Event()
    reader_thread = _start_nonblocking_fifo_reader(str(output_fifo), captured_output, reader_stop)

    with patch("agentx.session.create_adapter") as mock_create_adapter:
        adapter = MagicMock()
        adapter.get_models.return_value = []
        adapter.classify_prompt_sync.return_value = None
        adapter.process_prompt_generator.return_value = iter(
            [
                ResponseChunk(type=ChunkType.CONTENT, content="Hello from mocked LLM"),
                ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
            ]
        )
        mock_create_adapter.return_value = adapter

        session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
        session._safe_root_after = lambda callback: callback()

        try:
            prompt = "hello through tui"
            _write_fifo_payload(str(input_fifo), f"{prompt}{SUBMIT_SENTINEL}")

            deadline = time.time() + 4.0
            while time.time() < deadline:
                joined = "".join(captured_output)
                if "###USER" in joined and "Hello from mocked LLM" in joined and "###DONE" in joined:
                    break
                time.sleep(0.02)

            joined = "".join(captured_output)
            assert "###USER" in joined
            assert "Hello from mocked LLM" in joined
            assert "###DONE" in joined
        finally:
            session.close()
            reader_stop.set()
            reader_thread.join(timeout=1.0)


@pytest.mark.live
def test_e2e_live_tui_submit_and_output_flow(tmp_path: Path) -> None:
    """
    GIVEN a live AgentX application and reachable LLM services
    AND a TUI input buffer and output buffer
    WHEN a prompt is submitted through the TUI input FIFO
    THEN AgentX processes the prompt end-to-end
    AND a response appears on the TUI output buffer FIFO
    """
    if os.getenv("AGENTX_RUN_LIVE_TUI_E2E", "").strip() != "1":
        pytest.skip("Set AGENTX_RUN_LIVE_TUI_E2E=1 to run live TUI E2E coverage.")

    output_fifo = tmp_path / "tui.output.fifo"
    input_fifo = tmp_path / "tui.input.fifo"
    os.mkfifo(output_fifo)
    os.mkfifo(input_fifo)

    root = tk.Tk(useTk=False)
    config = _build_config(str(output_fifo), str(input_fifo), classify_prompts=True)

    captured_output: list[str] = []
    reader_stop = threading.Event()
    reader_thread = _start_nonblocking_fifo_reader(str(output_fifo), captured_output, reader_stop)

    session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
    session._safe_root_after = lambda callback: callback()

    try:
        prompt = "Reply with exactly: TUI_E2E_OK"
        _write_fifo_payload(str(input_fifo), f"{prompt}{SUBMIT_SENTINEL}")

        deadline = time.time() + 90.0
        while time.time() < deadline:
            joined = "".join(captured_output)
            if "###USER" in joined and "###DONE" in joined:
                break
            time.sleep(0.1)

        joined = "".join(captured_output)
        assert "###USER" in joined
        assert "###DONE" in joined
    finally:
        session.close()
        reader_stop.set()
        reader_thread.join(timeout=1.0)
