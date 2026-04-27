"""Unit tests for OllamaServiceProvider.

GIVEN an Ollama-backed provider
WHEN listing models and reading context metadata
THEN it should normalize responses and apply safe fallbacks.
"""

from unittest.mock import Mock, patch

import pytest

from agentx.providers import ILLMServiceProvider, OllamaServiceProvider
from agentx.providers.constants import FALLBACK_CONTEXT_WINDOW


def _mock_response(payload: dict, status_code: int = 200) -> Mock:
    response = Mock()
    response.json.return_value = payload
    response.status_code = status_code
    response.raise_for_status.return_value = None
    return response


@pytest.mark.unit
def test_list_models_returns_names() -> None:
    """GIVEN /api/tags payload WHEN list_models is called THEN names are returned."""
    provider = OllamaServiceProvider("localhost:11434")
    payload = {"models": [{"name": "llama3.2"}, {"name": "mistral:7b"}]}
    with patch("shared.providers.ollama_provider.requests.get", return_value=_mock_response(payload)):
        assert provider.list_models() == ["llama3.2", "mistral:7b"]


@pytest.mark.unit
def test_get_context_length_key_probe_priority() -> None:
    """GIVEN model_info keys WHEN reading context length THEN highest-priority key is used."""
    provider = OllamaServiceProvider("localhost:11434")
    payload = {"model_info": {"llama.context_length": 8192, "num_ctx": 2048}}
    with patch("shared.providers.ollama_provider.requests.post", return_value=_mock_response(payload)):
        assert provider.get_context_length("llama3.2") == 8192


@pytest.mark.unit
def test_get_context_length_fallback_on_error() -> None:
    """GIVEN provider network failure WHEN reading context length THEN fallback is returned."""
    provider = OllamaServiceProvider("localhost:11434")
    with patch("shared.providers.ollama_provider.requests.post", side_effect=RuntimeError("boom")):
        assert provider.get_context_length("missing") == FALLBACK_CONTEXT_WINDOW


@pytest.mark.unit
def test_list_models_returns_empty_on_invalid_payload() -> None:
    """GIVEN a non-dict tags payload WHEN list_models is called THEN an empty list is returned."""
    provider = OllamaServiceProvider("localhost:11434")
    with patch("shared.providers.ollama_provider.requests.get", return_value=_mock_response(["bad"])):
        assert provider.list_models() == []


@pytest.mark.unit
def test_get_context_length_uses_dynamic_context_length_key() -> None:
    """GIVEN a model_info dict with a provider-specific context key WHEN read THEN that key is used."""
    provider = OllamaServiceProvider("localhost:11434")
    payload = {"model_info": {"qwen.context_length": "16384"}}
    with patch("shared.providers.ollama_provider.requests.post", return_value=_mock_response(payload)):
        assert provider.get_context_length("qwen") == 16384


@pytest.mark.unit
def test_get_context_length_fallback_on_non_dict_model_info() -> None:
    """GIVEN a non-dict model_info payload WHEN read THEN fallback context length is returned."""
    provider = OllamaServiceProvider("localhost:11434")
    payload = {"model_info": ["bad"]}
    with patch("shared.providers.ollama_provider.requests.post", return_value=_mock_response(payload)):
        assert provider.get_context_length("llama3.2") == FALLBACK_CONTEXT_WINDOW


@pytest.mark.unit
def test_get_model_metadata_returns_supported_fields() -> None:
    """GIVEN provider details metadata WHEN requested THEN supported metadata fields are returned."""
    provider = OllamaServiceProvider("localhost:11434")
    payload = {"details": {"parameter_size": "7B", "family": "llama", "ignored": []}}
    with patch("shared.providers.ollama_provider.requests.post", return_value=_mock_response(payload)):
        assert provider.get_model_metadata("llama3.2") == {"parameter_size": "7B", "family": "llama"}


@pytest.mark.unit
def test_get_model_metadata_returns_empty_on_non_dict_details() -> None:
    """GIVEN non-dict details WHEN metadata is requested THEN an empty dict is returned."""
    provider = OllamaServiceProvider("localhost:11434")
    payload = {"details": ["bad"]}
    with patch("shared.providers.ollama_provider.requests.post", return_value=_mock_response(payload)):
        assert provider.get_model_metadata("llama3.2") == {}


@pytest.mark.unit
@pytest.mark.parametrize(
    ("value", "expected"),
    [(True, None), (12, 12), (12.9, 12), (" 42 ", 42), ("abc", None)],
)
def test_to_int_converts_supported_values(value: object, expected: int | None) -> None:
    """GIVEN mixed scalar values WHEN _to_int is called THEN only safe integer conversions succeed."""
    assert OllamaServiceProvider._to_int(value) == expected


@pytest.mark.unit
def test_protocol_runtime_check() -> None:
    """GIVEN provider instance WHEN checked against protocol THEN isinstance returns True."""
    provider = OllamaServiceProvider("localhost:11434")
    assert isinstance(provider, ILLMServiceProvider)


@pytest.mark.unit
def test_provider_id_is_ollama() -> None:
    """GIVEN OllamaServiceProvider instance WHEN provider_id is read THEN it is 'ollama'."""
    provider = OllamaServiceProvider("localhost:11434")
    assert provider.provider_id == "ollama"


@pytest.mark.unit
@pytest.mark.parametrize(
    "host, expected",
    [
        (None, "http://localhost:11434"),
        ("", "http://localhost:11434"),
        ("localhost:11434", "http://localhost:11434"),
        ("http://my-server:11434", "http://my-server:11434"),
        ("http://my-server:11434/", "http://my-server:11434"),
        ("https://secure:11434", "https://secure:11434"),
    ],
)
def test_normalize_host(host: str | None, expected: str) -> None:
    """GIVEN various host formats WHEN _normalize_host is called THEN a canonical URL is returned.

    Permutations:
    - None  -> default localhost URL
    - empty string -> default localhost URL
    - bare host:port -> prefixed with http://
    - already http:// URL -> returned as-is (trailing slash stripped)
    - already http:// URL with trailing slash -> trailing slash stripped
    - https:// URL -> returned unchanged
    """
    assert OllamaServiceProvider._normalize_host(host) == expected
