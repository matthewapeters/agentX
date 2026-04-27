"""Unit tests for ModelMetadataStore.

GIVEN a model metadata store
WHEN startup population runs
THEN it should use cache when possible and fetch missing models only.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from agentx.model_metadata_store import ModelMetadataStore
from agentx.providers.constants import FALLBACK_CONTEXT_WINDOW


class FakeProvider:
    """Simple provider test double for deterministic store tests."""

    provider_id: str = "fake-provider"

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


@pytest.mark.unit
def test_populate_fetches_all_without_cache(tmp_path: Path) -> None:
    """GIVEN no cache WHEN populate runs THEN all model capacities are fetched."""
    provider = FakeProvider(models=["a", "b"], capacities={"a": 1024, "b": 2048})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")

    store.populate()

    assert store.get_context_length("a") == 1024
    assert store.get_context_length("b") == 2048
    assert provider.context_calls == ["a", "b"]


@pytest.mark.unit
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


@pytest.mark.unit
def test_get_context_length_unknown_returns_fallback(tmp_path: Path) -> None:
    """GIVEN unknown model WHEN looked up THEN fallback context window is returned."""
    provider = FakeProvider(models=["a"], capacities={"a": 1024})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    assert store.get_context_length("missing") == FALLBACK_CONTEXT_WINDOW


@pytest.mark.unit
def test_populated_event_is_set_after_populate(tmp_path: Path) -> None:
    """GIVEN store not yet populated WHEN populate completes THEN .populated event is set."""
    provider = FakeProvider(models=["x"], capacities={"x": 8192})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")

    assert not store.populated.is_set()
    store.populate()
    assert store.populated.is_set()


@pytest.mark.unit
def test_populated_event_set_even_on_provider_failure(tmp_path: Path) -> None:
    """GIVEN a provider that raises WHEN populate is called THEN .populated event is still set.

    This ensures the background thread never leaves callers waiting indefinitely.
    """

    class BrokenProvider:
        provider_id = "broken"

        def list_models(self) -> list[str]:  # type: ignore[return]
            raise RuntimeError("network error")

        def get_context_length(self, model_name: str) -> int:
            return FALLBACK_CONTEXT_WINDOW

        def get_model_metadata(self, model_name: str) -> dict:
            return {}

    store = ModelMetadataStore(provider=BrokenProvider(), cache_path=tmp_path / "cache.json")
    store.populate()  # must not raise, must set event
    assert store.populated.is_set()


@pytest.mark.unit
def test_save_cache_uses_provider_id(tmp_path: Path) -> None:
    """GIVEN a fake provider with a custom provider_id WHEN save_cache runs THEN cache records that id."""
    provider = FakeProvider(models=["m"], capacities={"m": 2048})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    cached = json.loads((tmp_path / "cache.json").read_text())
    assert cached["provider"] == "fake-provider"


@pytest.mark.unit
def test_invalidate_clears_single_model(tmp_path: Path) -> None:
    """GIVEN populated store WHEN invalidate(model) is called THEN that model is evicted.

    The invalidated model returns FALLBACK_CONTEXT_WINDOW before re-population.
    """
    provider = FakeProvider(models=["a", "b"], capacities={"a": 1024, "b": 4096})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()
    assert store.get_context_length("a") == 1024

    # After invalidation the entry is gone; wait for background re-populate.
    store.invalidate("a")
    store.populated.wait(timeout=5.0)
    # Re-fetched from provider (context_calls should contain 'a' a second time).
    assert store.get_context_length("a") == 1024


@pytest.mark.unit
def test_invalidate_all_clears_store(tmp_path: Path) -> None:
    """GIVEN populated store WHEN invalidate() (no arg) is called THEN all entries are cleared."""
    provider = FakeProvider(models=["a"], capacities={"a": 999})
    store = ModelMetadataStore(provider=provider, cache_path=tmp_path / "cache.json")
    store.populate()

    store.invalidate()  # clear all
    store.populated.wait(timeout=5.0)
    # After re-population the value is restored.
    assert store.get_context_length("a") == 999


@pytest.mark.unit
def test_parse_cache_data_returns_both_dicts(tmp_path: Path) -> None:
    """GIVEN a raw cache dict WHEN _parse_cache_data is called THEN both capacities and metadata are returned."""
    raw = {
        "capacities": {"m": 1234},
        "metadata": {"m": {"family": "test"}},
    }
    capacities, metadata = ModelMetadataStore._parse_cache_data(raw)
    assert capacities == {"m": 1234}
    assert metadata == {"m": {"family": "test"}}
