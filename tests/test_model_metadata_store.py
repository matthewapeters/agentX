"""Unit tests for ModelMetadataStore.

GIVEN a model metadata store
WHEN startup population runs
THEN it should use cache when possible and fetch missing models only.
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentix.constants import FALLBACK_CONTEXT_WINDOW
from agentx.model_metadata_store import ModelMetadataStore


class FakeProvider:
    """Simple provider test double for deterministic store tests."""

    def __init__(self, models: list[str], capacities: dict[str, int]) -> None:
        self._models = models
        self._capacities = capacities
        self.context_calls: list[str] = []

    def list_models(self) -> list[str]:
        return list(self._models)

    def get_context_length(self, model_name: str) -> int:
        self.context_calls.append(model_name)
        return self._capacities.get(model_name, FALLBACK_CONTEXT_WINDOW)

    def get_model_metadata(self, model_name: str) -> dict[str, str | int]:
        return {"family": "test", "parameter_size": "7B", "name": model_name}


def test_populate_fetches_all_without_cache(tmp_path: Path) -> None:
    """GIVEN no cache WHEN populate runs THEN all model capacities are fetched."""
    provider = FakeProvider(models=["a", "b"], capacities={"a": 1024, "b": 2048})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")

    store.populate()

    assert store.get_context_length("a") == 1024
    assert store.get_context_length("b") == 2048
    assert provider.context_calls == ["a", "b"]


def test_populate_uses_cache_when_model_set_matches(tmp_path: Path) -> None:
    """GIVEN matching cache WHEN populate runs THEN per-model fetch is skipped."""
    cache_path = tmp_path / "cache.json"
    cache_path.write_text(
        json.dumps(
            {
                "provider": "ollama",
                "models": ["a"],
                "capacities": {"a": 4096},
                "metadata": {"a": {"family": "test"}},
                "updated_at": "2026-04-26T12:00:00Z",
            }
        ),
        encoding="utf-8",
    )

    provider = FakeProvider(models=["a"], capacities={"a": 1024})
    store = ModelMetadataStore(provider=provider, cache_path=cache_path)

    store.populate()

    assert store.get_context_length("a") == 4096
    assert provider.context_calls == []


def test_get_context_length_unknown_returns_fallback(tmp_path: Path) -> None:
    """GIVEN unknown model WHEN looked up THEN fallback context window is returned."""
    provider = FakeProvider(models=["a"], capacities={"a": 1024})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    assert store.get_context_length("missing") == FALLBACK_CONTEXT_WINDOW
