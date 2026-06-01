from __future__ import annotations

import importlib.util
import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from typing import Any

import pytest


@pytest.fixture(scope="module")
def template_module():
    module_path = Path(__file__).resolve().parents[1] / "applets" / "template.py"
    spec = importlib.util.spec_from_file_location("agentx_applet_template_pipeline", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError("failed to load applets/template.py")

    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class _ContextState:
    def __init__(self, context_payload: dict[str, Any], activity_payload: dict[str, Any]):
        self._context_payload = context_payload
        self._activity_payload = activity_payload
        self._lock = threading.Lock()

    def set_context_payload(self, payload: dict[str, Any]) -> None:
        with self._lock:
            self._context_payload = payload

    def set_activity_payload(self, payload: dict[str, Any]) -> None:
        with self._lock:
            self._activity_payload = payload

    def get_context_payload(self) -> dict[str, Any]:
        with self._lock:
            return json.loads(json.dumps(self._context_payload))

    def get_activity_payload(self) -> dict[str, Any]:
        with self._lock:
            return json.loads(json.dumps(self._activity_payload))


class _ContextHandler(BaseHTTPRequestHandler):
    state: _ContextState

    def do_GET(self) -> None:  # noqa: N802 - stdlib handler signature
        if self.path == "/context":
            payload = self.state.get_context_payload()
        elif self.path == "/activity":
            payload = self.state.get_activity_payload()
        else:
            self.send_response(404)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b"{}")
            return

        body = json.dumps(payload).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: Any) -> None:  # noqa: A002, D401 - quiet test server
        return


def _start_context_server(
    payload: dict[str, Any],
    activity_payload: dict[str, Any],
) -> tuple[HTTPServer, threading.Thread, _ContextState]:
    state = _ContextState(payload, activity_payload)
    _ContextHandler.state = state
    server = HTTPServer(("127.0.0.1", 0), _ContextHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server, thread, state


def _wait_for_render(buffer: str, marker: str, timeout_sec: float = 8.0) -> str:
    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        if marker in buffer:
            return buffer
        time.sleep(0.1)
    raise AssertionError(f"timed out waiting for render marker: {marker!r}\nlast output:\n{buffer}")


def test_context_applet_renders_tabs_from_live_context_payload(tmp_path, monkeypatch, template_module):
    project_dir = tmp_path / "project"
    project_dir.mkdir()
    (project_dir / "alpha.txt").write_text("alpha", encoding="utf-8")
    (project_dir / "beta.txt").write_text("beta", encoding="utf-8")
    (project_dir / ".agentx").mkdir()
    state_file = project_dir / ".agentx" / "system-panel-tab.txt"

    payload = {
        "session_id": "sess-integration",
        "turn_count": 1,
        "turns": [{"prompt": "what is 2+2?", "response": "4"}],
        "prompt_cycle": {
            "classify": {"state": "done", "elapsed_ms": 12},
            "thinking": {"state": "running", "elapsed_ms": 25},
            "tool": {"state": "pending", "elapsed_ms": 0},
            "respond": {"state": "pending", "elapsed_ms": 0},
        },
    }

    activity_payload = {
        "session_id": "sess-integration",
        "state": "working",
        "phase": "thinking",
        "prompt_cycle": payload["prompt_cycle"],
    }

    server, thread, state = _start_context_server(payload, activity_payload)
    monkeypatch.setattr(template_module, "PROJECT_DIR", str(project_dir))
    monkeypatch.setattr(template_module, "CORE_BASE_URL", f"http://127.0.0.1:{server.server_port}")
    monkeypatch.setenv("AGENTX_SYSTEM_PANEL_TAB_STATE_FILE", str(state_file))
    monkeypatch.setenv("AGENTX_SYSTEM_PANEL_TAB", "files")

    try:
        state_file.write_text("files\n", encoding="utf-8")
        live_payload = template_module._get_json("/context")
        live_activity = template_module._get_json("/activity")
        latest = template_module._render_system_surface(
            live_payload,
            template_module._resolve_selected_system_tab(),
            live_activity,
        )
        assert live_payload["turn_count"] == 1
        assert "== FILES ==" in latest
        assert "== CONFIGURATION ==" not in latest
        assert "== CONTEXT WINDOW ==" not in latest

        state_file.write_text("configuration\n", encoding="utf-8")
        monkeypatch.delenv("AGENTX_SYSTEM_PANEL_TAB", raising=False)
        latest = template_module._render_system_surface(
            live_payload,
            template_module._resolve_selected_system_tab(),
            live_activity,
        )
        assert "== CONFIGURATION ==" in latest
        assert "model: " in latest
        assert "== FILES ==" not in latest

        state.set_context_payload(
            {
                "session_id": "sess-integration",
                "turn_count": 2,
                "turns": [
                    {"prompt": "what is 2+2?", "response": "4"},
                    {"prompt": "how many letters in agentx?", "response": "6"},
                ],
                "prompt_cycle": payload["prompt_cycle"],
            }
        )
        state_file.write_text("context-history\n", encoding="utf-8")
        live_payload = template_module._get_json("/context")
        live_activity = template_module._get_json("/activity")
        latest = template_module._render_system_surface(
            live_payload,
            template_module._resolve_selected_system_tab(),
            live_activity,
        )
        assert "== CONTEXT HISTORY ==" in latest
        assert "recent_prompt: what is 2+2?" in latest
        assert "== CONTEXT ==" not in latest

        state.set_context_payload(
            {
                "session_id": "sess-integration",
                "turn_count": 2,
                "turns": [
                    {"prompt": "what is 2+2?", "response": "4"},
                    {"prompt": "how many letters in agentx?", "response": "6"},
                ],
                "prompt_cycle": {
                    "classify": {"state": "done", "elapsed_ms": 12},
                    "thinking": {"state": "done", "elapsed_ms": 25},
                    "tool": {"state": "done", "elapsed_ms": 0},
                    "respond": {"state": "done", "elapsed_ms": 0},
                },
            }
        )
        state.set_activity_payload(
            {
                "session_id": "sess-integration",
                "state": "completed",
                "phase": "respond",
                "prompt_cycle": {
                    "classify": {"state": "done", "elapsed_ms": 12},
                    "thinking": {"state": "done", "elapsed_ms": 25},
                    "tool": {"state": "done", "elapsed_ms": 0},
                    "respond": {"state": "done", "elapsed_ms": 0},
                },
            }
        )
        state_file.write_text("context-visualizer\n", encoding="utf-8")
        live_payload = template_module._get_json("/context")
        live_activity = template_module._get_json("/activity")
        latest = template_module._render_system_surface(
            live_payload,
            template_module._resolve_selected_system_tab(),
            live_activity,
        )
        assert "== CONTEXT WINDOW ==" in latest
        assert "== ACTIVITY ==" in latest
        assert "state: completed" in latest
        assert "phase: respond" in latest
        assert "== CONTEXT VISUALIZER ==" in latest
        assert "== PROMPT CYCLE ==" in latest
        assert "🤖 Respond" in latest
        assert "consumed:" in latest

    finally:
        server.shutdown()
        thread.join(timeout=5)
