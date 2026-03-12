"""
Unit tests for agentix.tools.codeagent.

These tests verify that model name and Ollama host are read from parameters
rather than being hardcoded, and that error handling returns None on failure.
All LiteLLMModel / CodeAgent calls are mocked so no live service is required.
"""

import sys
import os
from pathlib import Path
from unittest.mock import MagicMock, patch, call

import pytest

# ---------------------------------------------------------------------------
# Path setup — allow imports from src/
# ---------------------------------------------------------------------------
project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

# ---------------------------------------------------------------------------
# Module-level mocks for optional heavy dependencies (smolagents, litellm).
# These are registered before importing codeagent so the module loads cleanly
# in environments where the packages are not installed.
# ---------------------------------------------------------------------------

_mock_litellm = MagicMock()
_mock_smolagents = MagicMock()

# Provide the names that codeagent.py imports from smolagents
_mock_smolagents.CodeAgent = MagicMock
_mock_smolagents.DuckDuckGoSearchTool = MagicMock
_mock_smolagents.LiteLLMModel = MagicMock
_mock_smolagents.RunResult = MagicMock

sys.modules.setdefault("litellm", _mock_litellm)
sys.modules.setdefault("smolagents", _mock_smolagents)

# Now safe to import the module under test
from agentix.tools.codeagent import webquery, codeagent  # noqa: E402


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_mock_agent(run_return_value="mock result"):
    """Return a mock CodeAgent whose .run() returns run_return_value."""
    agent = MagicMock()
    agent.run.return_value = run_return_value
    agent.tools = []
    return agent


# ---------------------------------------------------------------------------
# webquery tests
# ---------------------------------------------------------------------------


class TestWebquery:
    """Tests for the webquery() function."""

    def test_passes_model_to_litellm(self):
        """LiteLLMModel is constructed with model_id derived from the model param."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent()
            webquery("test query", model="my-model:7b")

            mock_llm_cls.assert_called_once()
            _, kwargs = mock_llm_cls.call_args
            assert kwargs["model_id"] == "ollama_chat/my-model:7b"

    def test_passes_host_to_litellm(self):
        """LiteLLMModel api_base is derived from the ollama_host param."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent()
            webquery("test query", model="m", ollama_host="myhost:11435")

            _, kwargs = mock_llm_cls.call_args
            assert kwargs["api_base"] == "http://myhost:11435"

    def test_default_host_is_localhost(self):
        """When ollama_host is omitted, api_base defaults to localhost:11434."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent()
            webquery("test query", model="m")

            _, kwargs = mock_llm_cls.call_args
            assert kwargs["api_base"] == "http://localhost:11434"

    def test_different_models_produce_different_model_ids(self):
        """Each distinct model name produces the correct ollama_chat/ prefix."""
        models = ["llama3.2:latest", "phi4-mini:3.8b", "gpt-oss"]
        for model_name in models:
            with (
                patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
                patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
            ):
                mock_agent_cls.return_value = _make_mock_agent()
                webquery("q", model=model_name)
                _, kwargs = mock_llm_cls.call_args
                assert kwargs["model_id"] == f"ollama_chat/{model_name}"

    def test_returns_agent_result_on_success(self):
        """webquery returns the value from agent.run()."""
        expected = "search result"
        with (
            patch("agentix.tools.codeagent.LiteLLMModel"),
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent(run_return_value=expected)
            result = webquery("q", model="m")
            assert result == expected

    def test_returns_none_on_exception(self):
        """webquery returns None when agent.run() raises an exception."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel"),
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            agent = MagicMock()
            agent.run.side_effect = RuntimeError("connection refused")
            agent.tools = []
            mock_agent_cls.return_value = agent
            result = webquery("q", model="m")
            assert result is None

    def test_no_hardcoded_model_strings(self):
        """Regression: the old hardcoded model string must not appear in calls."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent()
            webquery("q", model="custom-model")
            _, kwargs = mock_llm_cls.call_args
            assert "llama3.2" not in kwargs["model_id"]
            assert "localhost:11434" not in kwargs["model_id"]


# ---------------------------------------------------------------------------
# codeagent tests
# ---------------------------------------------------------------------------


class TestCodeagent:
    """Tests for the codeagent() function."""

    def test_passes_model_to_litellm(self):
        """LiteLLMModel is constructed with model_id derived from the model param."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent()
            codeagent("write a sort function", model="phi4:latest")

            mock_llm_cls.assert_called_once()
            _, kwargs = mock_llm_cls.call_args
            assert kwargs["model_id"] == "ollama_chat/phi4:latest"

    def test_passes_host_to_litellm(self):
        """LiteLLMModel api_base is derived from the ollama_host param."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent()
            codeagent("q", model="m", ollama_host="remotehost:8080")

            _, kwargs = mock_llm_cls.call_args
            assert kwargs["api_base"] == "http://remotehost:8080"

    def test_default_host_is_localhost(self):
        """When ollama_host is omitted, api_base defaults to localhost:11434."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent()
            codeagent("q", model="m")

            _, kwargs = mock_llm_cls.call_args
            assert kwargs["api_base"] == "http://localhost:11434"

    def test_returns_agent_result_on_success(self):
        """codeagent returns the value from agent.run()."""
        expected = "def sort(lst): ..."
        with (
            patch("agentix.tools.codeagent.LiteLLMModel"),
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent(run_return_value=expected)
            result = codeagent("q", model="m")
            assert result == expected

    def test_returns_none_on_exception(self):
        """codeagent returns None when agent.run() raises an exception."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel"),
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            agent = MagicMock()
            agent.run.side_effect = ValueError("model not found")
            agent.tools = []
            mock_agent_cls.return_value = agent
            result = codeagent("q", model="m")
            assert result is None

    def test_no_hardcoded_model_strings(self):
        """Regression: the old hardcoded model string must not appear in calls."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent()
            codeagent("q", model="custom-model")
            _, kwargs = mock_llm_cls.call_args
            assert "llama3.2" not in kwargs["model_id"]
            assert "localhost:11434" not in kwargs["model_id"]

    def test_model_id_format(self):
        """model_id is always prefixed with ollama_chat/."""
        with (
            patch("agentix.tools.codeagent.LiteLLMModel") as mock_llm_cls,
            patch("agentix.tools.codeagent.CodeAgent") as mock_agent_cls,
        ):
            mock_agent_cls.return_value = _make_mock_agent()
            codeagent("q", model="some-model:tag")
            _, kwargs = mock_llm_cls.call_args
            assert kwargs["model_id"].startswith("ollama_chat/")
