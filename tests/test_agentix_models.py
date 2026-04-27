"""Unit tests for agentix model selection helpers.

GIVEN Agentix model-selection utilities
WHEN Ollama responses and arguments vary
THEN failures are handled safely and cached context lengths are reused.
"""

from __future__ import annotations

from types import SimpleNamespace
from unittest.mock import Mock, patch

import pytest
import requests

from agentix.models import get_model, get_models, parse_parameter_size


def _args(model: str = "llama", debug: bool = False, ollama_host: str = "localhost:11434") -> SimpleNamespace:
    """Build a lightweight argument namespace for model-selection tests."""
    return SimpleNamespace(model=model, debug=debug, ollama_host=ollama_host)


def _response(payload: object) -> Mock:
    """Create a response double with JSON payload support."""
    response = Mock()
    response.raise_for_status.return_value = None
    response.json.return_value = payload
    return response


@pytest.mark.unit
def test_get_models_filters_by_prefix() -> None:
    """GIVEN multiple Ollama models WHEN filtering by prefix THEN only matching models are returned."""
    args = _args(model="llama")
    payload = {"models": [{"name": "llama3.2"}, {"name": "mistral:7b"}]}

    with patch("agentix.models.requests.get", return_value=_response(payload)):
        models = get_models(args)

    assert models == [{"name": "llama3.2"}]


@pytest.mark.unit
def test_get_models_debug_mode_without_filter_returns_dict_models() -> None:
    """GIVEN debug mode with filtering disabled WHEN get_models is called THEN valid dict models are returned."""
    args = _args(model="llama", debug=True)
    payload = {"models": [{"name": "llama3.2"}, {"name": "mistral:7b"}]}

    with patch("agentix.models.requests.get", return_value=_response(payload)):
        models = get_models(args, filter_by_model=False)

    assert models == [{"name": "llama3.2"}, {"name": "mistral:7b"}]


@pytest.mark.unit
def test_get_models_returns_empty_on_request_failure() -> None:
    """GIVEN an Ollama request failure WHEN get_models is called THEN an empty list is returned."""
    args = _args()

    with patch("agentix.models.requests.get", side_effect=requests.RequestException("boom")):
        assert get_models(args) == []


@pytest.mark.unit
def test_get_models_returns_empty_on_invalid_json() -> None:
    """GIVEN a JSON decode failure WHEN get_models is called THEN an empty list is returned."""
    args = _args()
    response = Mock()
    response.raise_for_status.return_value = None
    response.json.side_effect = ValueError("bad json")

    with patch("agentix.models.requests.get", return_value=response):
        assert get_models(args) == []


@pytest.mark.unit
def test_get_models_returns_empty_on_non_dict_payload() -> None:
    """GIVEN a non-dict payload WHEN get_models is called THEN an empty list is returned."""
    args = _args()

    with patch("agentix.models.requests.get", return_value=_response(["bad"])):
        assert get_models(args) == []


@pytest.mark.unit
def test_get_models_returns_empty_on_non_list_models_field() -> None:
    """GIVEN a non-list models field WHEN get_models is called THEN an empty list is returned."""
    args = _args()

    with patch("agentix.models.requests.get", return_value=_response({"models": "bad"})):
        assert get_models(args) == []


@pytest.mark.unit
def test_get_models_returns_all_valid_dict_models_without_filter() -> None:
    """GIVEN mixed model entries WHEN filtering is disabled THEN only dict models are returned."""
    args = _args(model="")
    payload = {"models": [{"name": "llama3.2"}, "bad-entry", {"name": "mistral:7b"}]}

    with patch("agentix.models.requests.get", return_value=_response(payload)):
        models = get_models(args, filter_by_model=False)

    assert models == [{"name": "llama3.2"}, {"name": "mistral:7b"}]


@pytest.mark.unit
@pytest.mark.parametrize(
    ("param_size", "expected"),
    [("1.5B", 1500000000), ("7M", 7000000), ("12K", 12000)],
)
def test_parse_parameter_size_valid_inputs(param_size: str, expected: int) -> None:
    """GIVEN supported parameter suffixes WHEN parsed THEN integer capacities are returned."""
    assert parse_parameter_size(param_size) == expected


@pytest.mark.unit
@pytest.mark.parametrize("param_size", ["", "7", "7T", "abc", "1.2Q"])
def test_parse_parameter_size_invalid_inputs(param_size: str) -> None:
    """GIVEN malformed parameter sizes WHEN parsed THEN ValueError is raised."""
    with pytest.raises(ValueError):
        parse_parameter_size(param_size)


@pytest.mark.unit
def test_get_model_raises_when_no_models_available() -> None:
    """GIVEN no matching models WHEN get_model is called THEN a RuntimeError is raised."""
    args = _args(model="missing")

    with patch("agentix.models.get_models", return_value=[]):
        with pytest.raises(RuntimeError, match="No models available"):
            get_model(args)


@pytest.mark.unit
def test_get_model_uses_cached_max_tokens_without_provider_lookup() -> None:
    """GIVEN a cached max token count WHEN get_model is called THEN provider lookup is skipped."""
    args = _args(model="llama")

    with patch("agentix.models.get_models") as mock_get_models:
        with patch("shared.providers.ollama_provider.OllamaServiceProvider.get_context_length") as mock_context_length:
            resolved = get_model(args, max_tokens=16384)

    assert resolved == 16384
    mock_get_models.assert_not_called()
    mock_context_length.assert_not_called()
    assert args.model == "llama"


@pytest.mark.unit
def test_get_model_uses_provider_when_cache_missing() -> None:
    """GIVEN a selected model without cached context WHEN get_model is called THEN provider context length is used."""
    args = _args(model="llama")

    with patch("agentix.models.get_models", return_value=[{"name": "llama3.2"}]):
        with patch("shared.providers.ollama_provider.OllamaServiceProvider.get_context_length", return_value=8192):
            assert get_model(args) == 8192


@pytest.mark.unit
def test_get_model_logs_multiple_matches_in_debug_mode() -> None:
    """GIVEN multiple matching models in debug mode WHEN get_model is called THEN the first model is selected."""
    args = _args(model="llama", debug=True)
    models = [{"name": "llama3.2"}, {"name": "llama3.1"}]

    with patch("agentix.models.get_models", return_value=models):
        with patch("shared.providers.ollama_provider.OllamaServiceProvider.get_context_length", return_value=12288):
            assert get_model(args) == 12288
    assert args.model == "llama3.2"


@pytest.mark.unit
def test_get_model_preserves_fallback_context_value() -> None:
    """GIVEN provider fallback context WHEN get_model is called THEN the fallback value is returned."""
    args = _args(model="llama", debug=True)

    with patch("agentix.models.get_models", return_value=[{"name": "llama3.2"}]):
        with patch("shared.providers.ollama_provider.OllamaServiceProvider.get_context_length", return_value=4096):
            assert get_model(args) == 4096
