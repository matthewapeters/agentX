"""Integration-style tests for active model meter wiring.

GIVEN AgentXSession.active_model changes
WHEN setter executes
THEN denominator and breakdown are pushed to GUI update hook.
"""

import os
import sys
from unittest.mock import Mock

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentix.constants import FALLBACK_CONTEXT_WINDOW
from agentx.session import AgentXSession


def _build_session_stub() -> AgentXSession:
    session = AgentXSession.__new__(AgentXSession)
    session._active_model = "old-model"
    session.config = {"agentx": {"ollama_model": "old-model"}}
    session.agentix_adapter = Mock()
    session.agentix_adapter.agentix_config = Mock()
    session.context = Mock()
    session.context.token_breakdown.return_value = {"assistant": 10}
    session._model_store = Mock()
    session._model_store.get_context_length.return_value = 8192
    session.gui = Mock()
    session._safe_root_after = lambda cb: cb()
    return session


def test_active_model_change_triggers_meter_redraw() -> None:
    """GIVEN new active model WHEN setter runs THEN GUI meter update is called once."""
    session = _build_session_stub()

    session.active_model = "new-model"

    session.gui.update_context_meter.assert_called_once_with(max_tokens=8192, breakdown={"assistant": 10})
    assert session.config["agentx"]["ollama_model"] == "new-model"
    assert session.agentix_adapter.agentix_config.model == "new-model"


def test_active_model_noop_when_unchanged() -> None:
    """GIVEN identical active model WHEN setter runs THEN no redraw happens."""
    session = _build_session_stub()
    session._active_model = "same-model"

    session.active_model = "same-model"

    session.gui.update_context_meter.assert_not_called()


def test_active_model_uses_fallback_value() -> None:
    """GIVEN missing model capacity WHEN setter runs THEN fallback denominator is used."""
    session = _build_session_stub()
    session._model_store.get_context_length.return_value = FALLBACK_CONTEXT_WINDOW

    session.active_model = "missing-model"

    session.gui.update_context_meter.assert_called_once_with(
        max_tokens=FALLBACK_CONTEXT_WINDOW,
        breakdown={"assistant": 10},
    )
