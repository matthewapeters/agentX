"""Integration-style tests for active model meter wiring.

GIVEN AgentXSession.active_model changes
WHEN setter executes
THEN denominator and breakdown are pushed to GUI update hook.
"""

from unittest.mock import Mock

import pytest

from agentx.providers.constants import FALLBACK_CONTEXT_WINDOW
from agentx.session import AgentXSession
from shared.models.context import Context


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


@pytest.mark.unit
def test_active_model_change_triggers_meter_redraw() -> None:
    """GIVEN new active model WHEN setter runs THEN GUI meter update is called once."""
    session = _build_session_stub()

    session.active_model = "new-model"

    session.gui.update_context_meter.assert_called_once_with(max_tokens=8192, breakdown={"assistant": 10})
    assert session.config["agentx"]["ollama_model"] == "new-model"
    assert session.agentix_adapter.agentix_config.model == "new-model"
    assert session.agentix_adapter.agentix_config.model_max_tokens == 8192


@pytest.mark.unit
def test_active_model_noop_when_unchanged() -> None:
    """GIVEN identical active model WHEN setter runs THEN no redraw happens."""
    session = _build_session_stub()
    session._active_model = "same-model"

    session.active_model = "same-model"

    session.gui.update_context_meter.assert_not_called()


@pytest.mark.unit
def test_active_model_uses_fallback_value() -> None:
    """GIVEN missing model capacity WHEN setter runs THEN fallback denominator is used."""
    session = _build_session_stub()
    session._model_store.get_context_length.return_value = FALLBACK_CONTEXT_WINDOW

    session.active_model = "missing-model"

    session.gui.update_context_meter.assert_called_once_with(
        max_tokens=FALLBACK_CONTEXT_WINDOW,
        breakdown={"assistant": 10},
    )


@pytest.mark.unit
def test_on_context_assembled_updates_meter() -> None:
    """GIVEN a shared context WHEN on_context_assembled is called THEN meter is redrawn using shared_context breakdown."""
    session = _build_session_stub()
    shared_ctx = Mock(spec=Context)
    shared_ctx.token_breakdown.return_value = {"user": 50, "assistant": 20}
    session._model_store.get_context_length.return_value = 16384

    session.on_context_assembled(shared_ctx)

    session.gui.update_context_meter.assert_called_once_with(
        max_tokens=16384,
        breakdown={"user": 50, "assistant": 20},
    )


@pytest.mark.unit
def test_public_meter_methods_delegate_to_gui() -> None:
    """GIVEN a session stub WHEN public meter helpers are called THEN they compute payload and redraw correctly."""
    session = _build_session_stub()

    max_tokens, breakdown = session.context_meter_payload(model_name="new-model")
    session.schedule_meter_redraw(max_tokens=max_tokens, breakdown=breakdown)

    session.gui.update_context_meter.assert_called_once_with(max_tokens=8192, breakdown={"assistant": 10})


@pytest.mark.unit
def test_compatibility_meter_wrappers_delegate_to_public_api() -> None:
    """GIVEN a session stub WHEN compatibility wrappers are used THEN the public meter API still drives redraw."""
    session = _build_session_stub()

    max_tokens, breakdown = session._context_meter_payload(model_name="new-model")
    session._schedule_meter_redraw(max_tokens=max_tokens, breakdown=breakdown)

    session.gui.update_context_meter.assert_called_once_with(max_tokens=8192, breakdown={"assistant": 10})
