"""Unit tests for OllamaServiceProvider.

GIVEN an Ollama-backed provider
WHEN listing models and reading context metadata
THEN it should normalize responses and apply safe fallbacks.
"""

import os
import sys
from unittest.mock import Mock, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentix.constants import FALLBACK_CONTEXT_WINDOW
from agentx.providers import ILLMServiceProvider, OllamaServiceProvider


def _mock_response(payload: dict, status_code: int = 200) -> Mock:
    response = Mock()
    response.json.return_value = payload
    response.status_code = status_code
    response.raise_for_status.return_value = None
    return response


def test_list_models_returns_names() -> None:
    """GIVEN /api/tags payload WHEN list_models is called THEN names are returned."""
    provider = OllamaServiceProvider("localhost:11434")
    payload = {"models": [{"name": "llama3.2"}, {"name": "mistral:7b"}]}
    with patch("agentx.providers.ollama_provider.requests.get", return_value=_mock_response(payload)):
        assert provider.list_models() == ["llama3.2", "mistral:7b"]


def test_get_context_length_key_probe_priority() -> None:
    """GIVEN model_info keys WHEN reading context length THEN highest-priority key is used."""
    provider = OllamaServiceProvider("localhost:11434")
    payload = {"model_info": {"llama.context_length": 8192, "num_ctx": 2048}}
    with patch("agentx.providers.ollama_provider.requests.post", return_value=_mock_response(payload)):
        assert provider.get_context_length("llama3.2") == 8192


def test_get_context_length_fallback_on_error() -> None:
    """GIVEN provider network failure WHEN reading context length THEN fallback is returned."""
    provider = OllamaServiceProvider("localhost:11434")
    with patch("agentx.providers.ollama_provider.requests.post", side_effect=RuntimeError("boom")):
        assert provider.get_context_length("missing") == FALLBACK_CONTEXT_WINDOW


def test_protocol_runtime_check() -> None:
    """GIVEN provider instance WHEN checked against protocol THEN isinstance returns True."""
    provider = OllamaServiceProvider("localhost:11434")
    assert isinstance(provider, ILLMServiceProvider)
