"""Regression tests for Issue #8 TUI emoji parity with GUI."""

from __future__ import annotations

import os
import select
import threading
import time
import tkinter as tk
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from agentx.event_broker import Event, EventType
from agentx.integration.tui_bridge import SUBMIT_SENTINEL
from agentx.integration.tui_event_subscriber import TUIEventSubscriber
from agentx.session import AgentXSession
from shared.models.response import ChunkType, ResponseChunk


def _build_tui_config(output_fifo: str, input_fifo: str) -> dict:
    """Build a minimal headless config for deterministic TUI tests.

    Args:
        output_fifo: Absolute path to the TUI output FIFO.
        input_fifo: Absolute path to the TUI input FIFO.

    Returns:
        Session config accepted by AgentXSession.
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
            "classify_prompts": False,
            "available_tools": [],
        },
    }


def _write_fifo_payload(path: str, payload: str) -> None:
    """Write payload to a FIFO, retrying while the reader endpoint appears.

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


@pytest.mark.unit
@pytest.mark.parametrize(
    "event,expected_emoji",
    [
        (Event(event_type=EventType.AGENT_HEADER, data={}), "🤖"),
        (Event(event_type=EventType.USER_MESSAGE, data={"text": "hello", "timestamp": "12:00:00"}), "👤"),
        (Event(event_type=EventType.USER_MESSAGE, data={"text": "", "timestamp": ""}), "👤"),
    ],
    ids=["happy_agent_header", "defect_user_message", "boundary_empty_user_message"],
)
def test_tui_event_formatting_includes_role_emoji(event: Event, expected_emoji: str) -> None:
    """GIVEN TUI role events WHEN formatted for output THEN role emoji indicators are present. [PD-16-AF-004]"""
    subscriber = TUIEventSubscriber()

    formatted = subscriber._format_event_for_tui(event)

    assert expected_emoji in formatted


@pytest.mark.integration
def test_tui_submit_output_includes_user_and_agent_emojis(tmp_path: Path) -> None:
    """GIVEN a headless TUI prompt flow WHEN a response is streamed THEN output includes 👤 and 🤖 indicators. [PD-16-AF-004]"""
    output_fifo = tmp_path / "tui.output.fifo"
    input_fifo = tmp_path / "tui.input.fifo"
    os.mkfifo(output_fifo)
    os.mkfifo(input_fifo)

    captured_output: list[str] = []
    reader_stop = threading.Event()
    reader_thread = _start_nonblocking_fifo_reader(str(output_fifo), captured_output, reader_stop)

    root = tk.Tk(useTk=False)
    config = _build_tui_config(str(output_fifo), str(input_fifo))

    with patch("agentx.session.create_adapter") as mock_create_adapter:
        adapter = MagicMock()
        adapter.get_models.return_value = []
        adapter.classify_prompt_sync.return_value = None
        adapter.process_prompt_generator.return_value = iter(
            [
                ResponseChunk(type=ChunkType.CONTENT, content="EMOJI_REGRESSION_OK"),
                ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
            ]
        )
        mock_create_adapter.return_value = adapter

        session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
        session._safe_root_after = lambda callback: callback()

        try:
            prompt = "emoji parity check"
            _write_fifo_payload(str(input_fifo), f"{prompt}{SUBMIT_SENTINEL}")

            deadline = time.time() + 4.0
            while time.time() < deadline:
                joined = "".join(captured_output)
                if "###USER" in joined and "EMOJI_REGRESSION_OK" in joined and "###DONE" in joined:
                    break
                time.sleep(0.02)

            joined = "".join(captured_output)
            assert "###USER" in joined
            assert "EMOJI_REGRESSION_OK" in joined
            assert "###DONE" in joined
            assert "👤" in joined
            assert "🤖" in joined
        finally:
            session.close()
            reader_stop.set()
            reader_thread.join(timeout=1.0)
