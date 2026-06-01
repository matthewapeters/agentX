import importlib.util
from pathlib import Path

import pytest


@pytest.fixture(scope="module")
def template_module():
    module_path = Path(__file__).resolve().parents[1] / "applets" / "template.py"
    spec = importlib.util.spec_from_file_location("agentx_applet_template", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError("failed to load applets/template.py")

    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_normalize_system_tab_aliases(template_module):
    normalize = template_module._normalize_system_tab

    assert normalize("files") == "files"
    assert normalize("context_visualizer") == "context-visualizer"
    assert normalize("history") == "context-history"
    assert normalize("all") == "full"
    assert normalize("  CONFIG  ") == "configuration"
    assert normalize("unknown") == ""


def test_selected_tab_prefers_state_file_over_env(monkeypatch, tmp_path, template_module):
    state_file = tmp_path / "system-tab.txt"
    state_file.write_text("context_visualizer\n", encoding="utf-8")

    monkeypatch.setenv("AGENTX_SYSTEM_PANEL_TAB_STATE_FILE", str(state_file))
    monkeypatch.setenv("AGENTX_SYSTEM_PANEL_TAB", "files")

    assert template_module._resolve_selected_system_tab() == "context-visualizer"


def test_selected_tab_falls_back_to_env_when_state_missing(monkeypatch, tmp_path, template_module):
    state_file = tmp_path / "missing-system-tab.txt"

    monkeypatch.setenv("AGENTX_SYSTEM_PANEL_TAB_STATE_FILE", str(state_file))
    monkeypatch.setenv("AGENTX_SYSTEM_PANEL_TAB", "configuration")

    assert template_module._resolve_selected_system_tab() == "configuration"


def test_selected_tab_defaults_to_full_when_unset(monkeypatch, tmp_path, template_module):
    state_file = tmp_path / "empty-system-tab.txt"
    state_file.write_text("unknown-tab\n", encoding="utf-8")

    monkeypatch.setenv("AGENTX_SYSTEM_PANEL_TAB_STATE_FILE", str(state_file))
    monkeypatch.delenv("AGENTX_SYSTEM_PANEL_TAB", raising=False)

    assert template_module._resolve_selected_system_tab() == "full"


def test_render_system_surface_honors_selected_tab(monkeypatch, tmp_path, template_module):
    monkeypatch.setattr(template_module, "PROJECT_DIR", str(tmp_path))

    payload = {
        "session_id": "session-123",
        "turns": [{"prompt": "hello", "response": "hi"}],
        "turn_count": 1,
    }

    rendered = template_module._render_system_surface(payload, "files")

    assert "[SYSTEM TAB] active=files" in rendered
    assert "== FILES ==" in rendered
    assert "== CONFIGURATION ==" not in rendered
    assert "== CONTEXT ==" not in rendered

    full_rendered = template_module._render_system_surface(payload, "full")
    assert "[SYSTEM TAB] active=full" in full_rendered
    assert "== FILES ==" in full_rendered
    assert "== CONFIGURATION ==" in full_rendered
    assert "== CONTEXT ==" in full_rendered


def test_render_system_surface_context_visualizer_uses_activity_payload(monkeypatch, tmp_path, template_module):
    monkeypatch.setattr(template_module, "PROJECT_DIR", str(tmp_path))

    payload = {
        "session_id": "session-123",
        "turns": [{"prompt": "hello", "response": "hi"}],
        "turn_count": 1,
        "prompt_cycle": {
            "classify": {"state": "done", "elapsed_ms": 1},
            "thinking": {"state": "pending", "elapsed_ms": 0},
            "tool": {"state": "pending", "elapsed_ms": 0},
            "respond": {"state": "pending", "elapsed_ms": 0},
        },
    }
    activity_payload = {
        "session_id": "session-123",
        "state": "working",
        "phase": "tool",
        "prompt_cycle": {
            "classify": {"state": "done", "elapsed_ms": 1},
            "thinking": {"state": "done", "elapsed_ms": 2},
            "tool": {"state": "running", "elapsed_ms": 3},
            "respond": {"state": "pending", "elapsed_ms": 0},
        },
    }

    rendered = template_module._render_system_surface(payload, "context-visualizer", activity_payload)

    assert "== ACTIVITY ==" in rendered
    assert "state: working" in rendered
    assert "phase: tool" in rendered
    assert "↻ 🔧 Tool" in rendered
