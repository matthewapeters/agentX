"""Unit tests for cached max token resolution in ToolLoopRunner.

GIVEN ToolLoopRunner instances
WHEN a cached model context length is available
THEN the tool loop reuses it instead of triggering another live lookup.
"""

from unittest.mock import patch

import pytest

from agentix.agentix_config import AgentixConfig
from agentix.bridge.tool_loop import ToolLoopRunner


@pytest.mark.unit
def test_tool_loop_passes_cached_max_tokens_to_get_model() -> None:
    """GIVEN cached model_max_tokens WHEN _get_max_tokens runs THEN get_model receives the cached value."""
    config = AgentixConfig(model="llama3.2", tools=[], model_max_tokens=12288)
    runner = ToolLoopRunner(config)

    with patch("agentix.bridge.tool_loop.get_model", return_value=12288) as mock_get_model:
        resolved = runner._get_max_tokens()

    assert resolved == 12288
    mock_get_model.assert_called_once_with(config, max_tokens=12288)


@pytest.mark.unit
def test_tool_loop_caches_first_max_token_lookup() -> None:
    """GIVEN repeated max token requests WHEN _get_max_tokens is called twice THEN get_model runs only once."""
    config = AgentixConfig(model="llama3.2", tools=[], model_max_tokens=8192)
    runner = ToolLoopRunner(config)

    with patch("agentix.bridge.tool_loop.get_model", return_value=8192) as mock_get_model:
        first = runner._get_max_tokens()
        second = runner._get_max_tokens()

    assert first == 8192
    assert second == 8192
    mock_get_model.assert_called_once_with(config, max_tokens=8192)


@pytest.mark.unit
def test_invalidate_max_tokens_forces_fresh_lookup() -> None:
    """GIVEN an invalidated runner cache WHEN _get_max_tokens is called again THEN get_model reruns."""
    config = AgentixConfig(model="llama3.2", tools=[], model_max_tokens=8192)
    runner = ToolLoopRunner(config)

    with patch("agentix.bridge.tool_loop.get_model", side_effect=[8192, 16384]) as mock_get_model:
        first = runner._get_max_tokens()
        runner.invalidate_max_tokens()
        config.model_max_tokens = 16384
        second = runner._get_max_tokens()

    assert first == 8192
    assert second == 16384
    assert mock_get_model.call_count == 2
